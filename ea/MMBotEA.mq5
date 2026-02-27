//+------------------------------------------------------------------+
//|                                                    MMBotEA.mq5   |
//| Real EA polling/execution loop for MMBot backend                |
//+------------------------------------------------------------------+
#property strict

#include <Trade/Trade.mqh>

input string ApiBaseUrl           = "http://127.0.0.1:18080";
input string AccountId            = "paper-1";
input string DeviceId             = "mt5-device-1";
input string ConnectCode          = "MMBOT-ONE-TIME-CODE";
input int    PollIntervalSeconds  = 5;
input int    SyncEveryLoops       = 10;     // every N timer loops
input int    RequestTimeoutMs     = 5000;
input bool   VerboseLogs          = true;
input bool   CloseBySymbolOnly    = true;   // CLOSE command scope guard

CTrade g_trade;

string g_token = "";
string g_tokenExpiresAt = "";
bool   g_remotePaused = false;
bool   g_isBusy = false;
int    g_loopCounter = 0;
string g_lastCommandId = "";
string g_pendingCommandId = "";
string g_pendingResultPayload = "";
string g_stateFileName = "";

//+------------------------------------------------------------------+
int OnInit()
{
   g_stateFileName = StringFormat("MMBotEA_%I64u_state.txt", (ulong)AccountInfoInteger(ACCOUNT_LOGIN));
   LoadState();
   EventSetTimer(MathMax(PollIntervalSeconds, 1));
   PrintInfo("MMBotEA initialized.");
   return(INIT_SUCCEEDED);
}

//+------------------------------------------------------------------+
void OnDeinit(const int reason)
{
   EventKillTimer();
   SaveState();
}

//+------------------------------------------------------------------+
void OnTimer()
{
   if(g_isBusy)
      return;

   g_isBusy = true;

   if(g_pendingCommandId != "")
   {
      if(!FlushPendingResult())
      {
         g_isBusy = false;
         return;
      }
   }

   if(g_token == "")
   {
      RegisterEA();
      g_isBusy = false;
      return;
   }

   if(!SendHeartbeat())
   {
      g_isBusy = false;
      return;
   }

   if(g_loopCounter % MathMax(SyncEveryLoops, 1) == 0)
   {
      SendSync();
   }

   PollAndExecute();
   g_loopCounter++;
   g_isBusy = false;
}

//+------------------------------------------------------------------+
void RegisterEA()
{
   string body = StringFormat(
      "{\"connect_code\":\"%s\",\"account_id\":\"%s\",\"device_id\":\"%s\"}",
      JsonEscape(ConnectCode),
      JsonEscape(AccountId),
      JsonEscape(DeviceId)
   );

   int status = 0;
   string resp = "";
   if(!HttpRequest("POST", "/ea/register", body, false, status, resp))
      return;

   if(status != 200)
   {
      PrintWarn(StringFormat("EA register failed: HTTP %d, body=%s", status, resp));
      return;
   }

   string token = JsonGetString(resp, "token");
   string expiresAt = JsonGetString(resp, "expires_at");
   if(token == "")
   {
      PrintWarn("EA register response missing token.");
      return;
   }

   g_token = token;
   g_tokenExpiresAt = expiresAt;
   SaveState();
   PrintInfo("EA registered and token acquired.");
}

//+------------------------------------------------------------------+
bool SendHeartbeat()
{
   int status = 0;
   string resp = "";
   if(!HttpRequest("POST", "/ea/heartbeat", "{}", true, status, resp))
      return false;

   if(status == 401)
   {
      PrintWarn("EA token rejected on heartbeat; clearing token.");
      ClearToken();
      return false;
   }
   if(status != 200)
   {
      PrintWarn(StringFormat("Heartbeat failed: HTTP %d body=%s", status, resp));
      return false;
   }

   bool paused = JsonGetBool(resp, "paused", false);
   g_remotePaused = paused;
   return true;
}

//+------------------------------------------------------------------+
bool SendSync()
{
   string body = BuildSyncPayload();
   int status = 0;
   string resp = "";
   if(!HttpRequest("POST", "/ea/sync", body, true, status, resp))
      return false;

   if(status == 401)
   {
      PrintWarn("EA token rejected on sync; clearing token.");
      ClearToken();
      return false;
   }
   if(status != 200)
   {
      PrintWarn(StringFormat("Sync failed: HTTP %d body=%s", status, resp));
      return false;
   }
   return true;
}

//+------------------------------------------------------------------+
void PollAndExecute()
{
   int status = 0;
   string resp = "";
   if(!HttpRequest("POST", "/ea/execute", "{}", true, status, resp))
      return;

   if(status == 401)
   {
      PrintWarn("EA token rejected on execute poll; clearing token.");
      ClearToken();
      return;
   }
   if(status != 200)
   {
      PrintWarn(StringFormat("Execute poll failed: HTTP %d body=%s", status, resp));
      return;
   }

   string cmdId = JsonGetString(resp, "command_id");
   string cmdType = ToUpper(JsonGetString(resp, "type"));
   if(cmdId == "" || cmdType == "" || cmdType == "NOOP")
      return;

   if(cmdId == g_lastCommandId || cmdId == g_pendingCommandId)
   {
      PrintInfo(StringFormat("Skipping duplicate command_id=%s", cmdId));
      return;
   }

   bool ok = false;
   string ticket = "";
   string errCode = "";
   string errMsg = "";

   if(cmdType == "OPEN")
      ok = ExecuteOpen(resp, ticket, errCode, errMsg);
   else if(cmdType == "CLOSE")
      ok = ExecuteClose(resp, ticket, errCode, errMsg);
   else if(cmdType == "MOVE_SL")
      ok = ExecuteMoveSL(resp, ticket, errCode, errMsg);
   else if(cmdType == "SET_TP")
      ok = ExecuteSetTP(resp, ticket, errCode, errMsg);
   else if(cmdType == "PAUSE")
      ok = ExecutePause(ticket, errCode, errMsg);
   else if(cmdType == "RESUME")
      ok = ExecuteResume(ticket, errCode, errMsg);
   else
   {
      ok = false;
      errCode = "UNSUPPORTED_COMMAND";
      errMsg = "unsupported command type: " + cmdType;
   }

   string statusStr = (ok ? "SUCCESS" : "FAIL");
   string executedAt = TimeToISO8601(TimeCurrent());
   string payload = StringFormat(
      "{\"command_id\":\"%s\",\"status\":\"%s\",\"broker_ticket\":\"%s\",\"error_code\":\"%s\",\"error_message\":\"%s\",\"executed_at\":\"%s\"}",
      JsonEscape(cmdId),
      statusStr,
      JsonEscape(ticket),
      JsonEscape(errCode),
      JsonEscape(errMsg),
      JsonEscape(executedAt)
   );

   g_pendingCommandId = cmdId;
   g_pendingResultPayload = payload;
   SaveState();
   FlushPendingResult();
}

//+------------------------------------------------------------------+
bool FlushPendingResult()
{
   if(g_pendingCommandId == "" || g_pendingResultPayload == "")
      return true;

   int status = 0;
   string resp = "";
   if(!HttpRequest("POST", "/ea/result", g_pendingResultPayload, true, status, resp))
      return false;

   if(status == 401)
   {
      PrintWarn("EA token rejected on result report; clearing token.");
      ClearToken();
      return false;
   }
   if(status != 200)
   {
      PrintWarn(StringFormat("Result report failed: HTTP %d body=%s", status, resp));
      return false;
   }

   g_lastCommandId = g_pendingCommandId;
   g_pendingCommandId = "";
   g_pendingResultPayload = "";
   SaveState();
   return true;
}

//+------------------------------------------------------------------+
bool ExecuteOpen(const string cmdJson, string &ticket, string &errCode, string &errMsg)
{
   if(g_remotePaused)
   {
      errCode = "REMOTE_PAUSED";
      errMsg = "backend pause active, refusing OPEN";
      return false;
   }

   string symbol = JsonGetString(cmdJson, "symbol");
   string side = ToUpper(JsonGetString(cmdJson, "side"));
   double volume = JsonGetDouble(cmdJson, "volume", 0.0);
   double slPips = JsonGetDouble(cmdJson, "sl", 0.0);
   double tpPips = JsonGetDouble(cmdJson, "tp", 0.0);

   if(symbol == "")
      symbol = _Symbol;
   if(side != "BUY" && side != "SELL")
   {
      errCode = "INVALID_SIDE";
      errMsg = "side must be BUY or SELL";
      return false;
   }
   if(volume <= 0.0)
      volume = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);

   if(!SymbolSelect(symbol, true))
   {
      errCode = "SYMBOL_SELECT_FAILED";
      errMsg = "could not select symbol " + symbol;
      return false;
   }

   double ask = SymbolInfoDouble(symbol, SYMBOL_ASK);
   double bid = SymbolInfoDouble(symbol, SYMBOL_BID);
   int digits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);
   double point = SymbolInfoDouble(symbol, SYMBOL_POINT);
   double pip = ((digits == 3 || digits == 5) ? point * 10.0 : point);

   double slPrice = 0.0;
   double tpPrice = 0.0;
   if(side == "BUY")
   {
      if(slPips > 0.0)
         slPrice = NormalizeDouble(ask - slPips * pip, digits);
      if(tpPips > 0.0)
         tpPrice = NormalizeDouble(ask + tpPips * pip, digits);
   }
   else
   {
      if(slPips > 0.0)
         slPrice = NormalizeDouble(bid + slPips * pip, digits);
      if(tpPips > 0.0)
         tpPrice = NormalizeDouble(bid - tpPips * pip, digits);
   }

   volume = NormalizeVolume(symbol, volume);
   bool sent = false;
   if(side == "BUY")
      sent = g_trade.Buy(volume, symbol, 0.0, slPrice, tpPrice, "MMBot OPEN");
   else
      sent = g_trade.Sell(volume, symbol, 0.0, slPrice, tpPrice, "MMBot OPEN");

   long retcode = g_trade.ResultRetcode();
   ticket = IntegerToString((int)g_trade.ResultOrder());
   if(!sent || !IsTradeRetcodeSuccess(retcode))
   {
      errCode = IntegerToString((int)retcode);
      errMsg = g_trade.ResultRetcodeDescription();
      return false;
   }
   return true;
}

//+------------------------------------------------------------------+
bool ExecuteClose(const string cmdJson, string &ticket, string &errCode, string &errMsg)
{
   string symbol = JsonGetString(cmdJson, "symbol");
   int closed = 0;
   int failed = 0;

   for(int i = PositionsTotal() - 1; i >= 0; i--)
   {
      ulong posTicket = PositionGetTicket(i);
      if(posTicket == 0)
         continue;
      if(!PositionSelectByTicket(posTicket))
         continue;

      string posSymbol = PositionGetString(POSITION_SYMBOL);
      if(CloseBySymbolOnly && symbol != "" && posSymbol != symbol)
         continue;

      if(g_trade.PositionClose(posTicket))
      {
         long ret = g_trade.ResultRetcode();
         if(IsTradeRetcodeSuccess(ret))
            closed++;
         else
            failed++;
      }
      else
      {
         failed++;
      }
   }

   ticket = IntegerToString(closed);
   if(closed <= 0)
   {
      errCode = "CLOSE_NONE";
      errMsg = (failed > 0 ? "no positions closed; trade errors occurred" : "no matching positions to close");
      return false;
   }
   if(failed > 0)
   {
      errCode = "CLOSE_PARTIAL";
      errMsg = "some positions failed to close";
   }
   return true;
}

//+------------------------------------------------------------------+
bool ExecuteMoveSL(const string cmdJson, string &ticket, string &errCode, string &errMsg)
{
   string symbol = JsonGetString(cmdJson, "symbol");
   double newSL = JsonGetDouble(cmdJson, "sl", 0.0); // absolute price expected
   if(newSL <= 0.0)
   {
      errCode = "INVALID_SL";
      errMsg = "MOVE_SL requires positive absolute sl price";
      return false;
   }

   int modified = 0;
   int failed = 0;
   for(int i = PositionsTotal() - 1; i >= 0; i--)
   {
      ulong posTicket = PositionGetTicket(i);
      if(posTicket == 0 || !PositionSelectByTicket(posTicket))
         continue;
      string posSymbol = PositionGetString(POSITION_SYMBOL);
      if(symbol != "" && posSymbol != symbol)
         continue;
      double currentTP = PositionGetDouble(POSITION_TP);
      int digits = (int)SymbolInfoInteger(posSymbol, SYMBOL_DIGITS);
      bool ok = g_trade.PositionModify(posTicket, NormalizeDouble(newSL, digits), currentTP);
      if(ok && IsTradeRetcodeSuccess(g_trade.ResultRetcode()))
         modified++;
      else
         failed++;
   }

   ticket = IntegerToString(modified);
   if(modified <= 0)
   {
      errCode = "MOVE_SL_NONE";
      errMsg = "no positions modified";
      return false;
   }
   if(failed > 0)
   {
      errCode = "MOVE_SL_PARTIAL";
      errMsg = "some positions failed to modify";
   }
   return true;
}

//+------------------------------------------------------------------+
bool ExecuteSetTP(const string cmdJson, string &ticket, string &errCode, string &errMsg)
{
   string symbol = JsonGetString(cmdJson, "symbol");
   double newTP = JsonGetDouble(cmdJson, "tp", 0.0); // absolute price expected
   if(newTP <= 0.0)
   {
      errCode = "INVALID_TP";
      errMsg = "SET_TP requires positive absolute tp price";
      return false;
   }

   int modified = 0;
   int failed = 0;
   for(int i = PositionsTotal() - 1; i >= 0; i--)
   {
      ulong posTicket = PositionGetTicket(i);
      if(posTicket == 0 || !PositionSelectByTicket(posTicket))
         continue;
      string posSymbol = PositionGetString(POSITION_SYMBOL);
      if(symbol != "" && posSymbol != symbol)
         continue;
      double currentSL = PositionGetDouble(POSITION_SL);
      int digits = (int)SymbolInfoInteger(posSymbol, SYMBOL_DIGITS);
      bool ok = g_trade.PositionModify(posTicket, currentSL, NormalizeDouble(newTP, digits));
      if(ok && IsTradeRetcodeSuccess(g_trade.ResultRetcode()))
         modified++;
      else
         failed++;
   }

   ticket = IntegerToString(modified);
   if(modified <= 0)
   {
      errCode = "SET_TP_NONE";
      errMsg = "no positions modified";
      return false;
   }
   if(failed > 0)
   {
      errCode = "SET_TP_PARTIAL";
      errMsg = "some positions failed to modify";
   }
   return true;
}

//+------------------------------------------------------------------+
bool ExecutePause(string &ticket, string &errCode, string &errMsg)
{
   g_remotePaused = true;
   ticket = "0";
   return true;
}

//+------------------------------------------------------------------+
bool ExecuteResume(string &ticket, string &errCode, string &errMsg)
{
   g_remotePaused = false;
   ticket = "0";
   return true;
}

//+------------------------------------------------------------------+
string BuildSyncPayload()
{
   double balance = AccountInfoDouble(ACCOUNT_BALANCE);
   double equity  = AccountInfoDouble(ACCOUNT_EQUITY);
   double realizedToday = ComputeRealizedPnLToday();

   string positions = "[";
   int count = 0;
   for(int i = 0; i < PositionsTotal(); i++)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0 || !PositionSelectByTicket(ticket))
         continue;

      if(count > 0)
         positions += ",";

      string symbol = PositionGetString(POSITION_SYMBOL);
      string side = ((ENUM_POSITION_TYPE)PositionGetInteger(POSITION_TYPE) == POSITION_TYPE_BUY ? "BUY" : "SELL");
      double volume = PositionGetDouble(POSITION_VOLUME);
      double priceOpen = PositionGetDouble(POSITION_PRICE_OPEN);
      double priceCurrent = PositionGetDouble(POSITION_PRICE_CURRENT);
      double sl = PositionGetDouble(POSITION_SL);
      double tp = PositionGetDouble(POSITION_TP);
      double profit = PositionGetDouble(POSITION_PROFIT);
      double swap = PositionGetDouble(POSITION_SWAP);
      double commission = 0.0;
      #ifdef POSITION_COMMISSION
         commission = PositionGetDouble(POSITION_COMMISSION);
      #endif

      positions += StringFormat(
         "{\"ticket\":%I64u,\"symbol\":\"%s\",\"side\":\"%s\",\"volume\":%s,\"price_open\":%s,\"price_current\":%s,\"sl\":%s,\"tp\":%s,\"profit\":%s,\"swap\":%s,\"commission\":%s}",
         ticket,
         JsonEscape(symbol),
         side,
         D(volume),
         D(priceOpen),
         D(priceCurrent),
         D(sl),
         D(tp),
         D(profit),
         D(swap),
         D(commission)
      );
      count++;
   }
   positions += "]";

   string payload = StringFormat(
      "{\"account_id\":\"%s\",\"device_id\":\"%s\",\"equity\":%s,\"balance\":%s,\"day_start_equity\":%s,\"realized_pnl_today\":%s,\"open_positions_count\":%d,\"positions\":%s}",
      JsonEscape(AccountId),
      JsonEscape(DeviceId),
      D(equity),
      D(balance),
      D(balance),
      D(realizedToday),
      count,
      positions
   );
   return payload;
}

//+------------------------------------------------------------------+
double ComputeRealizedPnLToday()
{
   datetime now = TimeCurrent();
   MqlDateTime dt;
   TimeToStruct(now, dt);
   dt.hour = 0;
   dt.min = 0;
   dt.sec = 0;
   datetime dayStart = StructToTime(dt);

   if(!HistorySelect(dayStart, now))
      return 0.0;

   int totalDeals = (int)HistoryDealsTotal();
   double realized = 0.0;
   for(int i = 0; i < totalDeals; i++)
   {
      ulong deal = HistoryDealGetTicket(i);
      if(deal == 0)
         continue;
      long entry = HistoryDealGetInteger(deal, DEAL_ENTRY);
      if(entry != DEAL_ENTRY_OUT && entry != DEAL_ENTRY_OUT_BY)
         continue;
      double profit = HistoryDealGetDouble(deal, DEAL_PROFIT);
      double swap = HistoryDealGetDouble(deal, DEAL_SWAP);
      double commission = HistoryDealGetDouble(deal, DEAL_COMMISSION);
      realized += profit + swap + commission;
   }
   return realized;
}

//+------------------------------------------------------------------+
bool HttpRequest(
   const string method,
   const string path,
   const string body,
   const bool withAuth,
   int &status,
   string &responseBody
)
{
   string base = ApiBaseUrl;
   if(StringLen(base) > 0 && StringSubstr(base, StringLen(base) - 1, 1) == "/")
      base = StringSubstr(base, 0, StringLen(base) - 1);
   string url = base + path;

   string headers = "Content-Type: application/json\r\nAccept: application/json\r\n";
   if(withAuth && g_token != "")
      headers += "Authorization: Bearer " + g_token + "\r\n";

   char data[];
   if(StringLen(body) > 0)
   {
      StringToCharArray(body, data, 0, WHOLE_ARRAY, CP_UTF8);
      int len = ArraySize(data);
      if(len > 0 && data[len - 1] == 0)
         ArrayResize(data, len - 1);
   }
   else
   {
      ArrayResize(data, 0);
   }

   char result[];
   string resultHeaders = "";
   ResetLastError();
   status = (int)WebRequest(method, url, headers, RequestTimeoutMs, data, result, resultHeaders);
   if(status == -1)
   {
      int err = GetLastError();
      PrintWarn(StringFormat("WebRequest failed (path=%s, err=%d). Ensure URL is in MT5 WebRequest allowlist.", path, err));
      return false;
   }
   responseBody = CharArrayToString(result, 0, WHOLE_ARRAY, CP_UTF8);
   return true;
}

//+------------------------------------------------------------------+
bool IsTradeRetcodeSuccess(const long retcode)
{
   return (retcode == TRADE_RETCODE_DONE ||
           retcode == TRADE_RETCODE_DONE_PARTIAL ||
           retcode == TRADE_RETCODE_PLACED);
}

//+------------------------------------------------------------------+
double NormalizeVolume(const string symbol, double volume)
{
   double minLot = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);
   double maxLot = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MAX);
   double step = SymbolInfoDouble(symbol, SYMBOL_VOLUME_STEP);
   if(step <= 0.0)
      step = 0.01;
   if(volume < minLot)
      volume = minLot;
   if(volume > maxLot)
      volume = maxLot;
   double steps = MathFloor((volume - minLot) / step + 0.5);
   return minLot + steps * step;
}

//+------------------------------------------------------------------+
string ToUpper(const string s)
{
   string out = s;
   StringToUpper(out);
   return out;
}

//+------------------------------------------------------------------+
string JsonGetString(const string json, const string key)
{
   string marker = "\"" + key + "\":";
   int p = StringFind(json, marker);
   if(p < 0)
      return "";
   p += StringLen(marker);
   p = SkipWs(json, p);
   if(p >= StringLen(json))
      return "";
   if(StringGetCharacter(json, p) != '\"')
      return "";
   p++;
   string out = "";
   for(int i = p; i < StringLen(json); i++)
   {
      ushort ch = StringGetCharacter(json, i);
      if(ch == '\"')
         return out;
      if(ch == '\\')
      {
         i++;
         if(i < StringLen(json))
            out += StringSubstr(json, i, 1);
         continue;
      }
      out += StringSubstr(json, i, 1);
   }
   return "";
}

//+------------------------------------------------------------------+
double JsonGetDouble(const string json, const string key, const double fallback)
{
   string marker = "\"" + key + "\":";
   int p = StringFind(json, marker);
   if(p < 0)
      return fallback;
   p += StringLen(marker);
   p = SkipWs(json, p);
   if(p >= StringLen(json))
      return fallback;

   int end = p;
   while(end < StringLen(json))
   {
      ushort ch = StringGetCharacter(json, end);
      if((ch >= '0' && ch <= '9') || ch == '-' || ch == '+' || ch == '.' || ch == 'e' || ch == 'E')
      {
         end++;
         continue;
      }
      break;
   }
   if(end <= p)
      return fallback;
   string raw = StringSubstr(json, p, end - p);
   return StringToDouble(raw);
}

//+------------------------------------------------------------------+
bool JsonGetBool(const string json, const string key, const bool fallback)
{
   string marker = "\"" + key + "\":";
   int p = StringFind(json, marker);
   if(p < 0)
      return fallback;
   p += StringLen(marker);
   p = SkipWs(json, p);
   if(p >= StringLen(json))
      return fallback;
   string tail = StringSubstr(json, p, 5);
   if(StringFind(tail, "true") == 0)
      return true;
   if(StringFind(tail, "false") == 0)
      return false;
   return fallback;
}

//+------------------------------------------------------------------+
int SkipWs(const string s, int p)
{
   while(p < StringLen(s))
   {
      ushort ch = StringGetCharacter(s, p);
      if(ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t')
         p++;
      else
         break;
   }
   return p;
}

//+------------------------------------------------------------------+
string JsonEscape(const string s)
{
   string out = "";
   for(int i = 0; i < StringLen(s); i++)
   {
      ushort ch = StringGetCharacter(s, i);
      if(ch == '\\')
         out += "\\\\";
      else if(ch == '\"')
         out += "\\\"";
      else if(ch == '\n')
         out += "\\n";
      else if(ch == '\r')
         out += "\\r";
      else if(ch == '\t')
         out += "\\t";
      else
         out += StringSubstr(s, i, 1);
   }
   return out;
}

//+------------------------------------------------------------------+
string TimeToISO8601(datetime t)
{
   MqlDateTime dt;
   TimeToStruct(t, dt);
   return StringFormat("%04d-%02d-%02dT%02d:%02d:%02dZ", dt.year, dt.mon, dt.day, dt.hour, dt.min, dt.sec);
}

//+------------------------------------------------------------------+
void LoadState()
{
   int h = FileOpen(g_stateFileName, FILE_COMMON | FILE_READ | FILE_TXT | FILE_ANSI);
   if(h == INVALID_HANDLE)
      return;

   while(!FileIsEnding(h))
   {
      string line = FileReadString(h);
      int eq = StringFind(line, "=");
      if(eq <= 0)
         continue;
      string key = StringSubstr(line, 0, eq);
      string val = StringSubstr(line, eq + 1);
      if(key == "token")
         g_token = val;
      else if(key == "token_expires_at")
         g_tokenExpiresAt = val;
      else if(key == "last_command_id")
         g_lastCommandId = val;
      else if(key == "pending_command_id")
         g_pendingCommandId = val;
      else if(key == "pending_result")
         g_pendingResultPayload = val;
      else if(key == "remote_paused")
         g_remotePaused = (val == "1");
   }
   FileClose(h);
}

//+------------------------------------------------------------------+
void SaveState()
{
   int h = FileOpen(g_stateFileName, FILE_COMMON | FILE_WRITE | FILE_TXT | FILE_ANSI);
   if(h == INVALID_HANDLE)
      return;

   FileWriteString(h, "token=" + g_token + "\n");
   FileWriteString(h, "token_expires_at=" + g_tokenExpiresAt + "\n");
   FileWriteString(h, "last_command_id=" + g_lastCommandId + "\n");
   FileWriteString(h, "pending_command_id=" + g_pendingCommandId + "\n");
   FileWriteString(h, "pending_result=" + g_pendingResultPayload + "\n");
   FileWriteString(h, "remote_paused=" + (g_remotePaused ? "1" : "0") + "\n");
   FileClose(h);
}

//+------------------------------------------------------------------+
void ClearToken()
{
   g_token = "";
   g_tokenExpiresAt = "";
   SaveState();
}

//+------------------------------------------------------------------+
void PrintInfo(const string msg)
{
   if(VerboseLogs)
      Print("[MMBotEA] ", msg);
}

//+------------------------------------------------------------------+
void PrintWarn(const string msg)
{
   Print("[MMBotEA][WARN] ", msg);
}

//+------------------------------------------------------------------+
string D(double value)
{
   return DoubleToString(value, 8);
}
