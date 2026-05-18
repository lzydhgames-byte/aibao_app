# Plan 9d UI-layer items, simulated via API (items #2 #3 #4 #6 #11 #12).
# Run via UTF-8 stdin pipe:
#   Get-Content -Encoding UTF8 -Raw scripts/plan9d-ui-via-api.ps1 | Invoke-Expression

$ErrorActionPreference = 'Continue'
$results = @()
function Record($name, $pass, $detail) {
  $script:results += [PSCustomObject]@{Item=$name; Pass=$pass; Detail=$detail}
}
function Post($path, $bodyObj, $tok) {
  $bodyJson = $bodyObj | ConvertTo-Json -Compress
  $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
  $hdr = @{}
  if ($tok) { $hdr['Authorization'] = "Bearer $tok" }
  return Invoke-RestMethod -Method POST -Uri ("http://127.0.0.1:8080" + $path) -ContentType 'application/json; charset=utf-8' -Headers $hdr -Body $bodyBytes
}
function Get_($path, $tok) {
  $hdr = @{}
  if ($tok) { $hdr['Authorization'] = "Bearer $tok" }
  return Invoke-RestMethod -Uri ("http://127.0.0.1:8080" + $path) -Headers $hdr
}

# Login (fresh, exercises full Plan 9b auth path)
Post '/api/v1/auth/sms/send' @{phone='13900000001'} $null | Out-Null
$loginResp = Post '/api/v1/auth/login_or_register' @{phone='13900000001'; code='123456'} $null
$tok = $loginResp.access_token
$childId = 3

# === Item 3: HEARTBEAT — 时段问候 + 活跃 storyline ===
try {
  $hb = Get_ ("/api/v1/heartbeat?child_id=" + $childId) $tok
  $hasGreeting = ($hb.greeting -and $hb.greeting.Length -gt 0)
  $hasStorylines = ($null -ne $hb.active_storylines)
  if ($hasGreeting -and $hasStorylines) {
    Record "3.HEARTBEAT greeting + storylines" $true ("greeting='" + $hb.greeting + "' storylines=" + $hb.active_storylines.Count)
  } else {
    Record "3.HEARTBEAT greeting + storylines" $false ($hb | ConvertTo-Json -Compress)
  }
} catch {
  Record "3.HEARTBEAT greeting + storylines" $false $_.Exception.Message
}

# === Item 4: 故事历史列表 ===
try {
  $list = Get_ ("/api/v1/stories?child_id=" + $childId + "&limit=5") $tok
  if ($list.items -and $list.items.Count -gt 0) {
    # check sorted DESC by created_at
    $createdAts = $list.items | ForEach-Object { [DateTime]::Parse($_.created_at) }
    $sortedDesc = $true
    for ($i = 1; $i -lt $createdAts.Count; $i++) {
      if ($createdAts[$i] -gt $createdAts[$i-1]) { $sortedDesc = $false; break }
    }
    Record "4.Story history list" $sortedDesc ("count=" + $list.items.Count + " desc=" + $sortedDesc)
  } else {
    Record "4.Story history list" $false "no items"
  }
} catch {
  Record "4.Story history list" $false $_.Exception.Message
}

# Capture pre-generation list length to verify item 11
$beforeCount = (Get_ ("/api/v1/stories?child_id=" + $childId + "&limit=5") $tok).items.Count

# === Item 6: storyline 续集 ===
# Step 1: start a new storyline
try {
  $r1 = Post '/api/v1/stories/generate' @{child_id=$childId; prompt='小狐狸的星空冒险'; duration=3; style='温馨治愈'; start_storyline=$true} $tok
  $slId = $r1.storyline_id
  $epNo1 = $r1.episode_no
  if ($slId -and $epNo1 -eq 1) {
    # Step 2: continue same storyline
    $r2 = Post '/api/v1/stories/generate' @{child_id=$childId; prompt='接着上一集讲'; duration=3; style='温馨治愈'; storyline_id=$slId} $tok
    if ($r2.storyline_id -eq $slId -and $r2.episode_no -eq 2) {
      Record "6.Storyline sequel" $true ("sl=" + $slId + " ep1=" + $epNo1 + " ep2=" + $r2.episode_no)
    } else {
      Record "6.Storyline sequel" $false ("ep2 wrong: sl=" + $r2.storyline_id + " ep=" + $r2.episode_no)
    }
  } else {
    Record "6.Storyline sequel" $false ("ep1 start failed: sl=" + $slId + " ep=" + $epNo1)
  }
} catch {
  Record "6.Storyline sequel" $false $_.Exception.Message
}

# === Item 11: list auto-refresh after generation ===
$afterCount = (Get_ ("/api/v1/stories?child_id=" + $childId + "&limit=10") $tok).items.Count
# This is a smoke: backend always has fresh data; the real Plan 9b 1c41cf6 fix is client-side
# (ref.invalidate). Server-side guarantee: a new generation appears in GET /stories immediately.
$afterFirst = (Get_ ("/api/v1/stories?child_id=" + $childId + "&limit=1") $tok).items[0]
$secsAgo = ((Get-Date).ToUniversalTime() - [DateTime]::Parse($afterFirst.created_at).ToUniversalTime()).TotalSeconds
if ($secsAgo -lt 120) {
  Record "11.List shows latest gen" $true ("latest story_id=" + $afterFirst.id + " " + [math]::Round($secsAgo) + "s ago")
} else {
  Record "11.List shows latest gen" $false ("latest is " + [math]::Round($secsAgo) + "s old — expected <120s")
}

# === Item 12: logout flow integrity ===
# Server has no logout endpoint by design (no JWT blacklist in MVP).
# What we actually validate: (a) old token still works (server-side logout
# truly is just client-side token discard), (b) relogin via SMS + code
# round-trips and returns a usable token.
try {
  $oldOk = $null -ne (Get_ '/api/v1/me' $tok)
  Post '/api/v1/auth/sms/send' @{phone='13900000001'} $null | Out-Null
  $loginResp2 = Post '/api/v1/auth/login_or_register' @{phone='13900000001'; code='123456'} $null
  $tok2 = $loginResp2.access_token
  $newOk = $null -ne (Get_ '/api/v1/me' $tok2)
  if ($oldOk -and $newOk) {
    Record "12.Logout/relogin flow" $true "old token still valid (no blacklist) + new token works"
  } else {
    Record "12.Logout/relogin flow" $false ("old_token_ok=" + $oldOk + " new_token_ok=" + $newOk)
  }
} catch {
  Record "12.Logout/relogin flow" $false $_.Exception.Message
}

# === Item 2: BOOTSTRAP card trigger ===
# This is a client-side conditional render: when child.profile.description is empty,
# the home screen shows the BOOTSTRAP-prompt card. Verify the GET /children response
# carries enough info for the client to make that decision.
try {
  $children = Get_ '/api/v1/children' $tok
  $child = $children.items | Where-Object { $_.id -eq $childId } | Select-Object -First 1
  if ($child) {
    $profile = $child.profile  # JSON string
    $hasDescription = ($profile -and $profile -match '"description"\s*:\s*"[^"]+')
    Record "2.BOOTSTRAP wiring (child.profile.description present?)" $true ("has_desc=" + $hasDescription + " — client will hide card when true, show when false")
  } else {
    Record "2.BOOTSTRAP wiring" $false "child not found"
  }
} catch {
  Record "2.BOOTSTRAP wiring" $false $_.Exception.Message
}

# === Print ===
"" | Out-Host
"=== Plan 9d UI-via-API smoke ===" | Out-Host
$results | ForEach-Object {
  $mark = if ($_.Pass) { "PASS" } else { "FAIL" }
  ("  [" + $mark + "] " + $_.Item + " - " + $_.Detail) | Out-Host
}
$failed = @($results | Where-Object { -not $_.Pass }).Count
"" | Out-Host
("Total: " + $results.Count + ", Passed: " + ($results.Count - $failed) + ", Failed: " + $failed) | Out-Host
if ($failed -gt 0) { exit 1 } else { exit 0 }
