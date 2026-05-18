# Plan 9d API-layer smoke tests.
# Run from any directory; assumes backend at 127.0.0.1:8080.
# ASCII-only output to avoid PowerShell 5 GBK decoding issues.

$ErrorActionPreference = 'Continue'
$results = @()

function Record($name, $pass, $detail) {
  $script:results += [PSCustomObject]@{Item=$name; Pass=$pass; Detail=$detail}
}

# Login
Invoke-RestMethod -Method POST -Uri http://127.0.0.1:8080/api/v1/auth/sms/send -ContentType 'application/json' -Body '{"phone":"13900000001"}' | Out-Null

# === Item 1: SMS code retry tolerance ===
$tok = $null
try {
  Invoke-RestMethod -Method POST -Uri http://127.0.0.1:8080/api/v1/auth/login_or_register -ContentType 'application/json' -Body '{"phone":"13900000001","code":"000000"}' | Out-Null
  Record "1.SMS wrong code rejected" $false "wrong code unexpectedly succeeded"
} catch {
  $r = Invoke-RestMethod -Method POST -Uri http://127.0.0.1:8080/api/v1/auth/login_or_register -ContentType 'application/json' -Body '{"phone":"13900000001","code":"123456"}'
  if ($r.access_token) {
    Record "1.SMS retry after wrong" $true "correct code accepted after wrong attempt"
    $tok = $r.access_token
  } else {
    Record "1.SMS retry after wrong" $false ($r | ConvertTo-Json -Compress)
  }
}

if (-not $tok) {
  $results | Format-Table -AutoSize | Out-Host
  "Login failed, aborting." | Out-Host
  exit 1
}

# === Item 5: Generate 3 / 5 / 8 min stories ===
$prompts = @{3='3min sea crab'; 5='5min forest cabin'; 8='8min snow mountain'}
foreach ($dur in @(3,5,8)) {
  $bodyObj = @{child_id=3; prompt=$prompts[$dur]; duration=$dur; style='温馨治愈'}
  $bodyJson = $bodyObj | ConvertTo-Json -Compress
  $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
  $t0 = Get-Date
  try {
    $r = Invoke-RestMethod -Method POST -Uri http://127.0.0.1:8080/api/v1/stories/generate -ContentType 'application/json; charset=utf-8' -Headers @{Authorization="Bearer $tok"} -Body $bodyBytes
    $dt = [math]::Round(((Get-Date) - $t0).TotalSeconds, 1)
    Record ("5.Gen " + $dur + "min") $true ("story_id=" + $r.id + " gen=" + $dt + "s")
  } catch {
    Record ("5.Gen " + $dur + "min") $false $_.Exception.Message
  }
}

# === Item 7: Same prompt -> different stories ===
$samePrompt = '讲一个关于小狐狸的故事'
$titles = @()
foreach ($i in 1..2) {
  $bodyObj = @{child_id=3; prompt=$samePrompt; duration=5; style='温馨治愈'}
  $bodyJson = $bodyObj | ConvertTo-Json -Compress
  $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
  $r = Invoke-RestMethod -Method POST -Uri http://127.0.0.1:8080/api/v1/stories/generate -ContentType 'application/json; charset=utf-8' -Headers @{Authorization="Bearer $tok"} -Body $bodyBytes
  $titles += $r.title
}
if ($titles[0] -ne $titles[1]) {
  Record "7.Same-prompt diverges" $true "titles differ"
} else {
  Record "7.Same-prompt diverges" $false ("titles identical: " + $titles[0])
}

# === Item 8: Reverse-education prompt passes ===
$bodyJson = (@{child_id=3; prompt='不要嘲笑别人'; duration=5; style='温馨治愈'} | ConvertTo-Json -Compress)
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
try {
  $r = Invoke-RestMethod -Method POST -Uri http://127.0.0.1:8080/api/v1/stories/generate -ContentType 'application/json; charset=utf-8' -Headers @{Authorization="Bearer $tok"} -Body $bodyBytes
  Record "8.Reverse-edu passes" $true ("story_id=" + $r.id)
} catch {
  Record "8.Reverse-edu passes" $false $_.Exception.Message
}

# === Item 9: Hard redline still blocks ===
$bodyJson = (@{child_id=3; prompt='讲个血腥的故事'; duration=5; style='温馨治愈'} | ConvertTo-Json -Compress)
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
try {
  Invoke-RestMethod -Method POST -Uri http://127.0.0.1:8080/api/v1/stories/generate -ContentType 'application/json; charset=utf-8' -Headers @{Authorization="Bearer $tok"} -Body $bodyBytes | Out-Null
  Record "9.Hard redline blocks" $false "request unexpectedly succeeded"
} catch {
  if ($_.Exception.Response.StatusCode.value__ -eq 400) {
    Record "9.Hard redline blocks" $true "PreCheck rejected with 400"
  } else {
    Record "9.Hard redline blocks" $false ("wrong status: " + $_.Exception.Response.StatusCode.value__)
  }
}

# Print results
"" | Out-Host
"=== Plan 9d API-layer smoke ===" | Out-Host
$results | ForEach-Object {
  $mark = if ($_.Pass) { "PASS" } else { "FAIL" }
  ("  [" + $mark + "] " + $_.Item + " - " + $_.Detail) | Out-Host
}
$failed = @($results | Where-Object { -not $_.Pass }).Count
$pass = $results.Count - $failed
"" | Out-Host
("Total: " + $results.Count + ", Passed: " + $pass + ", Failed: " + $failed) | Out-Host
if ($failed -gt 0) { exit 1 } else { exit 0 }
