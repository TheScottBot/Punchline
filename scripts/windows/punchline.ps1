<#
.SYNOPSIS
  Install, uninstall and manage start-at-login for Disciplinary Close-Up (Punchline) on Windows.

.DESCRIPTION
  Per-user only; never needs administrator rights. Every action is idempotent.

  Changes this script (and the app) can make to the system, all reversed by -Action Uninstall:
    1. %LOCALAPPDATA%\Programs\Punchline\punchline.exe              (installed program)
    2. HKCU\Software\Microsoft\Windows\CurrentVersion\Run  "Punchline"  (start at login)
    3. HKCU\...\Explorer\StartupApproved\Run  "Punchline"            (written by Windows when
       start-up is toggled in Task Manager)
    4. %LOCALAPPDATA%\Punchline\  settings.json, punchline.log       (kept with -KeepSettings)
  No drivers, services, firewall rules, scheduled tasks or machine-wide settings are touched.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1
  powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1 -Action DisableStartup
  powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1 -Action Uninstall
#>
[CmdletBinding()]
param(
    [ValidateSet('Install', 'Uninstall', 'EnableStartup', 'DisableStartup', 'Status')]
    [string]$Action = 'Install',
    # Install a prebuilt punchline.exe instead of building from source.
    [string]$FromExe,
    # Target architecture when building: amd64 or arm64 (default: this machine's).
    [ValidateSet('', 'amd64', 'arm64')]
    [string]$Arch = '',
    # Install without starting the app now.
    [switch]$NoLaunch,
    # Install without registering start at login.
    [switch]$NoStartup,
    # Uninstall but keep settings and log.
    [switch]$KeepSettings
)

Set-StrictMode -Version 2
$ErrorActionPreference = 'Stop'

$Name        = 'Punchline'
$InstallDir  = Join-Path $env:LOCALAPPDATA 'Programs\Punchline'
$Exe         = Join-Path $InstallDir 'punchline.exe'
$DataDir     = Join-Path $env:LOCALAPPDATA 'Punchline'
$RunKey      = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$ApprovedKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run'
$RepoRoot    = Resolve-Path (Join-Path $PSScriptRoot '..\..')

function Say([string]$m) { Write-Host "  $m" }

function Remove-RegValue([string]$Key, [string]$Value) {
    if ((Test-Path $Key) -and ($null -ne (Get-ItemProperty -Path $Key -Name $Value -ErrorAction SilentlyContinue))) {
        Remove-ItemProperty -Path $Key -Name $Value
        Say "removed $Key\$Value"
    }
}

function Get-Running {
    , @(Get-Process -Name 'punchline' -ErrorAction SilentlyContinue)
}

# Run a native command without PowerShell 5.1 turning its stderr into a
# terminating error. Returns stdout; check $LASTEXITCODE afterwards.
function Invoke-Native([scriptblock]$Block, [switch]$Quiet) {
    $old = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { & $Block 2>&1 | ForEach-Object { if ($_ -is [System.Management.Automation.ErrorRecord]) { if (-not $Quiet) { Write-Host "  $_" } } else { $_ } } }
    finally { $ErrorActionPreference = $old }
}

function Stop-App {
    $procs = Get-Running
    if ($procs.Count -eq 0) { return }
    # Ask politely first so hotkeys are unregistered and the camera released.
    $quitter = if (Test-Path $Exe) { $Exe } else { $procs[0].Path }
    if ($quitter) { Invoke-Native { & $quitter -quit } | Out-Null }
    $deadline = (Get-Date).AddSeconds(5)
    while ((Get-Running).Count -gt 0 -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 200 }
    $left = Get-Running
    foreach ($p in $left) {
        Say "forcing stop of process $($p.Id)"
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    }
    Say 'stopped running instance'
}

function Enable-Startup {
    if (-not (Test-Path $Exe)) { throw "Not installed ($Exe missing). Run -Action Install first." }
    if (-not (Test-Path $RunKey)) { New-Item -Path $RunKey -Force | Out-Null }
    Set-ItemProperty -Path $RunKey -Name $Name -Value "`"$Exe`""
    Remove-RegValue $ApprovedKey $Name   # clear a Task Manager "disabled" override
    Say "start at login enabled -> $Exe"
}

function Disable-Startup {
    Remove-RegValue $RunKey $Name
    Remove-RegValue $ApprovedKey $Name
    Say 'start at login disabled'
}

function Build-Exe([string]$OutDir) {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        throw 'Go is not on PATH. Install Go, or pass -FromExe <path to punchline.exe>.'
    }
    $ver = 'dev'
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $d = Invoke-Native { & git -C $RepoRoot describe --tags --always --dirty } -Quiet | Select-Object -First 1
        if ($LASTEXITCODE -eq 0 -and $d) { $ver = "$d" }
    }
    $out = Join-Path $OutDir 'punchline.exe'
    $oldArch = $env:GOARCH; $oldOS = $env:GOOS; $oldCgo = $env:CGO_ENABLED
    try {
        $env:GOOS = 'windows'; $env:CGO_ENABLED = '0'
        if ($Arch) { $env:GOARCH = $Arch }
        Push-Location $RepoRoot
        Invoke-Native { & go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=$ver" -o $out ./cmd/punchline } | Out-Host
        if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    } finally {
        Pop-Location
        $env:GOARCH = $oldArch; $env:GOOS = $oldOS; $env:CGO_ENABLED = $oldCgo
    }
    Say "built $ver"
    $out
}

function Install-App {
    $staging = Join-Path ([IO.Path]::GetTempPath()) ("punchline-build-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $staging | Out-Null
    try {
        $src = if ($FromExe) { (Resolve-Path $FromExe).Path } else { Build-Exe $staging }
        Stop-App
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        Copy-Item -Path $src -Destination $Exe -Force
        Say "installed $Exe"
    } finally {
        Remove-Item -Recurse -Force $staging -ErrorAction SilentlyContinue
    }
    if ($NoStartup) { Disable-Startup } else { Enable-Startup }
    if (-not $NoLaunch) {
        Start-Process -FilePath $Exe
        Say 'started (look for the magnifier in the notification area)'
    }
}

function Uninstall-App {
    Stop-App
    Disable-Startup
    if (Test-Path $InstallDir) {
        Remove-Item -Recurse -Force $InstallDir
        Say "removed $InstallDir"
    }
    if ($KeepSettings) {
        Say "kept $DataDir"
    } elseif (Test-Path $DataDir) {
        Remove-Item -Recurse -Force $DataDir
        Say "removed $DataDir"
    }
}

function Show-Status {
    $run = Get-ItemProperty -Path $RunKey -Name $Name -ErrorAction SilentlyContinue
    $appr = Get-ItemProperty -Path $ApprovedKey -Name $Name -ErrorAction SilentlyContinue
    $taskMgrDisabled = [bool]$appr -and ((($appr.$Name)[0] % 2) -eq 1)
    Say "installed:        $(Test-Path $Exe)  ($Exe)"
    Say "start at login:   $(if ($run -and -not $taskMgrDisabled) { 'enabled' } elseif ($run) { 'disabled in Task Manager' } else { 'disabled' })"
    Say "running:          $((Get-Running).Count -gt 0)"
    Say "settings and log: $DataDir  (exists: $(Test-Path $DataDir))"
}

Write-Host "Punchline: $Action"
switch ($Action) {
    'Install'        { Install-App }
    'Uninstall'      { Uninstall-App }
    'EnableStartup'  { Enable-Startup }
    'DisableStartup' { Disable-Startup }
    'Status'         { Show-Status }
}
