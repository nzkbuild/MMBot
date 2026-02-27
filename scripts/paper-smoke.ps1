param(
  [string]$BaseUrl = "http://127.0.0.1:18080",
  [string]$AdminUsername = "admin",
  [string]$AdminPassword = "change-me",
  [string]$ConnectCode = "MMBOT-ONE-TIME-CODE",
  [string]$AccountId = "paper-1",
  [string]$DeviceId = "smoke-device-1",
  [bool]$QueueSignal = $false,
  [string]$SignalSymbol = "XAUUSD",
  [double]$SignalSpreadPips = 1.0,
  [string]$TelegramWebhookSecret = "",
  [string]$TelegramChatId = ""
)

$ErrorActionPreference = "Stop"

function Invoke-JsonPost {
  param(
    [string]$Url,
    [hashtable]$Body,
    [string]$Bearer = "",
    [hashtable]$ExtraHeaders = @{}
  )
  $headers = @{ "Content-Type" = "application/json" }
  if ($Bearer -ne "") { $headers["Authorization"] = "Bearer $Bearer" }
  foreach ($k in $ExtraHeaders.Keys) { $headers[$k] = $ExtraHeaders[$k] }
  return Invoke-RestMethod -Method Post -Uri $Url -Headers $headers -Body ($Body | ConvertTo-Json -Depth 20)
}

function Invoke-JsonGet {
  param(
    [string]$Url,
    [string]$Bearer = ""
  )
  $headers = @{}
  if ($Bearer -ne "") { $headers["Authorization"] = "Bearer $Bearer" }
  return Invoke-RestMethod -Method Get -Uri $Url -Headers $headers
}

function Build-UptrendCandles {
  param([int]$Count = 120)
  $candles = @()
  $basePrice = 1.0800
  $start = [DateTimeOffset]::FromUnixTimeSeconds(1700000000).UtcDateTime
  for ($i = 0; $i -lt $Count; $i++) {
    $step = $i * 0.00065
    $close = $basePrice + $step
    if (($i % 9) -eq 0) { $close -= 0.0002 }
    $open = $close - 0.0002
    $high = $close + 0.0008
    $low = $close - 0.0010
    $candles += @{
      time  = $start.AddMinutes(15 * $i).ToString("yyyy-MM-ddTHH:mm:ssZ")
      open  = [double]$open
      high  = [double]$high
      low   = [double]$low
      close = [double]$close
    }
  }
  return $candles
}

Write-Host "[1/9] health check"
$health = Invoke-JsonGet -Url "$BaseUrl/health"
Write-Host "  status=$($health.status)"

Write-Host "[2/9] admin login"
$login = Invoke-JsonPost -Url "$BaseUrl/admin/login" -Body @{
  username = $AdminUsername
  password = $AdminPassword
}
$adminToken = $login.token
if (-not $adminToken) { throw "admin token missing" }
Write-Host "  token received"

Write-Host "[3/9] pause/resume"
$null = Invoke-JsonPost -Url "$BaseUrl/bot/pause" -Body @{} -Bearer $adminToken
$null = Invoke-JsonPost -Url "$BaseUrl/bot/resume" -Body @{} -Bearer $adminToken
Write-Host "  pause/resume OK"

Write-Host "[4/9] EA register"
$ea = Invoke-JsonPost -Url "$BaseUrl/ea/register" -Body @{
  connect_code = $ConnectCode
  account_id   = $AccountId
  device_id    = $DeviceId
}
$eaToken = $ea.token
if (-not $eaToken) { throw "ea token missing" }
Write-Host "  ea token received"

Write-Host "[5/9] heartbeat + sync"
$hb = Invoke-JsonPost -Url "$BaseUrl/ea/heartbeat" -Body @{} -Bearer $eaToken
$sync = Invoke-JsonPost -Url "$BaseUrl/ea/sync" -Bearer $eaToken -Body @{
  account_id         = $AccountId
  device_id          = $DeviceId
  equity             = 10000
  balance            = 10000
  day_start_equity   = 10000
  realized_pnl_today = -50
  positions          = @()
}
Write-Host "  paused=$($hb.paused) daily_loss_pct=$($sync.daily_loss_pct)"

Write-Host "[6/9] execute poll (likely NOOP unless command queued)"
$exec = Invoke-JsonPost -Url "$BaseUrl/ea/execute" -Body @{} -Bearer $eaToken
Write-Host "  execute type=$($exec.type)"

if ($QueueSignal) {
  Write-Host "[6b] queue test strategy signal"
  $candles = Build-UptrendCandles -Count 120
  $signalResp = Invoke-JsonPost -Url "$BaseUrl/admin/strategy/evaluate" -Bearer $adminToken -Body @{
    account_id  = $AccountId
    symbol      = $SignalSymbol
    spread_pips = $SignalSpreadPips
    candles     = $candles
  }
  Write-Host "  allowed=$($signalResp.allowed) deny_reason=$($signalResp.deny_reason)"
  if ($signalResp.command) {
    Write-Host "  queued command_id=$($signalResp.command.command_id) type=$($signalResp.command.type)"
  }
}

Write-Host "[7/9] /today via telegram webhook (optional)"
if ($TelegramChatId -ne "") {
  $extra = @{}
  if ($TelegramWebhookSecret -ne "") {
    $extra["X-Telegram-Bot-Api-Secret-Token"] = $TelegramWebhookSecret
  }
  $tg = Invoke-JsonPost -Url "$BaseUrl/telegram/webhook" -ExtraHeaders $extra -Body @{
    update_id = 1
    message = @{
      message_id = 1
      text = "/today $AccountId"
      chat = @{ id = [int64]$TelegramChatId }
    }
  }
  Write-Host "  telegram webhook accepted ok=$($tg.ok)"
} else {
  Write-Host "  skipped (TelegramChatId not provided)"
}

Write-Host "[8/9] dashboard summary"
$summary = Invoke-JsonGet -Url "$BaseUrl/dashboard/summary?account_id=$AccountId" -Bearer $adminToken
Write-Host "  open_positions=$($summary.open_positions) daily_loss_pct=$($summary.daily_loss_pct) paused=$($summary.paused)"

Write-Host "[9/9] events"
$events = Invoke-JsonGet -Url "$BaseUrl/events?limit=10" -Bearer $adminToken
Write-Host "  count=$($events.count)"

Write-Host "Paper smoke check completed."
