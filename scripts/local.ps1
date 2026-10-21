param(
    [ValidateSet('setup', 'api', 'test', 'smoke', 'web', 'customer', 'restaurant', 'delivery')]
    [string]$Task = 'setup',
    [ValidateSet('development', 'test')]
    [string]$Environment = 'development',
    [string]$ApiHost = ''
)

$ErrorActionPreference = 'Stop'
$coreRoot = Split-Path $PSScriptRoot -Parent
$workspaceRoot = Split-Path $coreRoot -Parent
$savedVariables = @{}

function Set-LocalVariable([string]$Name, [string]$Value) {
    if (-not $savedVariables.ContainsKey($Name)) {
        $savedVariables[$Name] = [System.Environment]::GetEnvironmentVariable($Name, 'Process')
    }
    [System.Environment]::SetEnvironmentVariable($Name, $Value, 'Process')
}

function Import-LocalEnv([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { throw "Missing local environment file: $Path" }
    foreach ($line in [IO.File]::ReadAllLines($Path)) {
        if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$') {
            $name = $Matches[1]
            $value = $Matches[2].Trim()
            if ($value.Length -ge 2 -and (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'")))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            Set-LocalVariable $name $value
        }
    }
}

function Invoke-LocalGo([string[]]$GoArguments) {
    & go @GoArguments
    if ($LASTEXITCODE -ne 0) { throw "Go command failed with exit code $LASTEXITCODE" }
}

$envName = if ($Environment -eq 'test') { '.env.test' } else { '.env' }
Push-Location $coreRoot
try {
    if ($Task -in @('setup', 'api', 'test', 'smoke')) {
        Import-LocalEnv (Join-Path $coreRoot $envName)
    }
    switch ($Task) {
        'setup' {
            $databaseUri = [Uri]$env:POSTGRES_DSN
            $expectedDatabase = if ($Environment -eq 'test') { '/yember_local_test' } else { '/yember_local' }
            if (-not $databaseUri.IsLoopback -or $databaseUri.AbsolutePath -ne $expectedDatabase -or $env:APP_ENV -ne $Environment) {
                throw 'Setup requires the selected local environment and its dedicated loopback database.'
            }
            Invoke-LocalGo @('run', './cmd/create_db')
            Invoke-LocalGo @('run', './cmd/migrate')
            Invoke-LocalGo @('run', './cmd/seed_local')
        }
        'api' { Invoke-LocalGo @('run', './cmd/api') }
        'smoke' {
            $databaseUri = [Uri]$env:POSTGRES_DSN
            if (-not $databaseUri.IsLoopback -or $databaseUri.AbsolutePath -notin @('/yember_local', '/yember_local_test')) {
                throw 'Smoke checks require a dedicated local database.'
            }
            Invoke-LocalGo @('build', '-o', '.test-cache/local-smoke.exe', './cmd/api')
            & (Join-Path $PSScriptRoot 'smoke-local.ps1')
        }
        'test' {
            Set-LocalVariable 'YEMBER_INTEGRATION' '1'
            Invoke-LocalGo @('test', './...', '-count=1')
        }
        'web' {
            Set-Location (Join-Path $workspaceRoot 'yember-web')
            Import-LocalEnv (Join-Path (Get-Location) $envName)
            if ($Environment -eq 'test') { & npm.cmd run dev -- --mode test }
            else { & npm.cmd run dev }
            if ($LASTEXITCODE -ne 0) { throw 'Admin web server failed.' }
        }
        default {
            $appPath = Join-Path $workspaceRoot "yember-app/apps/$Task"
            Import-LocalEnv (Join-Path $appPath $envName)
            if ($ApiHost) {
                if ($ApiHost -notmatch '^[A-Za-z0-9.-]+$') { throw 'ApiHost must be a hostname or IPv4 address.' }
                $apiPort = if ($Environment -eq 'test') { 8084 } else { 8080 }
                Set-LocalVariable 'EXPO_PUBLIC_API_URL' "http://${ApiHost}:${apiPort}/api/v1"
            }
            # Load the selected app file explicitly; never change NODE_ENV to select an API.
            Set-Location (Join-Path $workspaceRoot 'yember-app')
            & npm.cmd run $Task
            if ($LASTEXITCODE -ne 0) { throw 'Expo server failed.' }
        }
    }
} finally {
    Pop-Location
    foreach ($name in $savedVariables.Keys) {
        [System.Environment]::SetEnvironmentVariable($name, $savedVariables[$name], 'Process')
    }
}
