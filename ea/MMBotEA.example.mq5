// MMBotEA.example.mq5
// Legacy placeholder skeleton.
// Use ea/MMBotEA.mq5 for the real implementation.

#property strict

input string ApiBaseUrl = "http://127.0.0.1:8080";
input string AccountId = "paper-1";
input string DeviceId = "mt5-device-1";
input string ConnectCode = "MMBOT-ONE-TIME-CODE";

string g_token = "";

int OnInit() {
   EventSetTimer(2);
   return(INIT_SUCCEEDED);
}

void OnDeinit(const int reason) {
   EventKillTimer();
}

void OnTimer() {
   if (g_token == "") {
      RegisterEA();
      return;
   }
   SendHeartbeat();
   PollExecute();
}

void RegisterEA() {
   // TODO:
   // POST /ea/register with {connect_code, account_id, device_id}
   // Store returned token into g_token.
}

void SendHeartbeat() {
   // TODO:
   // POST /ea/heartbeat with Bearer token.
}

void PollExecute() {
   // TODO:
   // POST /ea/execute with Bearer token.
   // If command type is OPEN/CLOSE/MOVE_SL/SET_TP, execute order operations via MQL5 trade API.
   // Then POST /ea/result with SUCCESS/FAIL payload.
}
