# Invoked by local.ps1 with the selected profile in the process environment.
$ErrorActionPreference = 'Stop'
$apiPort = [int]$env:PORT
if (Get-NetTCPConnection -LocalPort $apiPort -State Listen -ErrorAction SilentlyContinue) {
    throw "Port $apiPort is already in use; smoke checks will not control an existing server."
}
$coreRoot = Split-Path $PSScriptRoot -Parent
$binaryPath = Join-Path $coreRoot '.test-cache/local-smoke.exe'
$server = Start-Process -FilePath $binaryPath -WorkingDirectory $coreRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $coreRoot '.test-cache/smoke-stdout.log') -RedirectStandardError (Join-Path $coreRoot '.test-cache/smoke-stderr.log')
$base = "http://127.0.0.1:${apiPort}/api/v1"
try {
    $ready = $false
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        if ($server.HasExited) { throw 'The local API exited during startup.' }
        try { $null = Invoke-RestMethod "http://127.0.0.1:${apiPort}/readyz"; $ready = $true; break } catch { Start-Sleep -Milliseconds 250 }
    }
    if (-not $ready) { throw 'The local API did not become ready.' }
    $adminHeaders = @{ 'X-Yember-Admin' = '1'; Origin = "http://127.0.0.1:5173" }
    $adminBody = @{email='admin@yember.test';password=$env:LOCAL_SEED_PASSWORD} | ConvertTo-Json
    $admin = Invoke-RestMethod "$base/auth/admin/login" -Method Post -Headers $adminHeaders -ContentType 'application/json' -Body $adminBody -SessionVariable adminCookies
    if ($admin.user.role -ne 'ADMIN') { throw 'Administrator login returned the wrong role.' }
    $headers = @{ Authorization = "Bearer $($admin.access_token)" }
    $stats = Invoke-RestMethod "$base/admin/dashboard/stats" -Headers $headers
    $overview = Invoke-RestMethod "$base/admin/console/overview" -Headers $headers
    if ($stats.total_orders -lt 10 -or $overview.customers -lt 1 -or $overview.daily_orders.Count -ne 7) { throw 'Dashboard fixture totals are incomplete.' }
    foreach ($resource in @('customers','restaurants','partners','orders','payments','reviews','coupons','users','audit','emails','support')) {
        $null = Invoke-RestMethod "$base/admin/manage/$resource" -Headers $headers
    }
    $null = Invoke-RestMethod "$base/auth/admin/session" -Method Post -Headers $adminHeaders -WebSession $adminCookies
    $null = Invoke-RestMethod "$base/auth/admin/logout" -Method Post -Headers $adminHeaders -WebSession $adminCookies
    Write-Output 'ADMIN: login, dashboard, all management lists, cookie restore and logout passed.'
    foreach ($account in @(
        @{email='customer@yember.test';role='CUSTOMER';path='/customer/addresses'},
        @{email='restaurant@yember.test';role='MERCHANT_OWNER';path='/restaurant/profile'},
        @{email='delivery@yember.test';role='DELIVERY_PARTNER';path='/rider/profile'}
    )) {
        $body = @{email=$account.email;password=$env:LOCAL_SEED_PASSWORD} | ConvertTo-Json
        $tokens = Invoke-RestMethod "$base/auth/login" -Method Post -ContentType 'application/json' -Body $body
        $accountHeaders = @{Authorization="Bearer $($tokens.access_token)"}
        $identity = Invoke-RestMethod "$base/auth/me" -Headers $accountHeaders
        if ($identity.role -ne $account.role) { throw "Role check failed for $($account.role)" }
        $null = Invoke-RestMethod "$base$($account.path)" -Headers $accountHeaders
        foreach ($path in @('/partner/account','/partner/overview?days=7','/partner/orders?scope=history','/partner/support')) {
            if ($account.role -ne 'CUSTOMER') { $null = Invoke-RestMethod "$base$path" -Headers $accountHeaders }
        }
        if ($account.role -eq 'DELIVERY_PARTNER') { $null = Invoke-RestMethod "$base/partner/earnings?days=7" -Headers $accountHeaders }
        if ($account.role -eq 'MERCHANT_OWNER') { $null = Invoke-RestMethod "$base/partner/reviews" -Headers $accountHeaders }
        $null = Invoke-RestMethod "$base/auth/logout" -Method Post -ContentType 'application/json' -Body (@{refresh_token=$tokens.refresh_token} | ConvertTo-Json)
        Write-Output "$($account.role): password login, identity and profile passed."
    }
    Write-Output "Local HTTP smoke checks passed on port $apiPort."
} finally {
    if (-not $server.HasExited) { Stop-Process -Id $server.Id }
}
