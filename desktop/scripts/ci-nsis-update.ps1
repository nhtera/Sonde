# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# CI stand-in for a Windows machine (windows-2022, admin, UAC off): installs
# installer A (0.0.1) into a custom folder, then checks the silent update
# mode of installer B (0.0.2), /S /UPDATE /WAITPID, as the app runs it:
#   - it waits for the app's process to exit, and updates the custom folder
#     (never Program Files), recording InstallLocation and the version;
#   - a 0.1.0 install (no InstallLocation) is found from DisplayIcon;
#   - with Sonde.exe still running from the folder, it changes nothing.
# The payloads are small programs (PayloadA/B) that sleep for their first
# argument's seconds, so "running" can be staged.
param(
  [Parameter(Mandatory)] [string] $InstallerA,
  [Parameter(Mandatory)] [string] $InstallerB,
  [Parameter(Mandatory)] [string] $PayloadA,
  [Parameter(Mandatory)] [string] $PayloadB,
  [Parameter(Mandatory)] [string] $Dir
)
$ErrorActionPreference = 'Stop'
$key = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\The Sonde AuthorsSonde'
$exe = Join-Path $Dir 'Sonde.exe'

function Fail([string] $msg) { Write-Error "FAIL: $msg"; exit 1 }
function Hash([string] $p) { (Get-FileHash -Algorithm SHA256 $p).Hash }
function Value([string] $name) { (Get-ItemProperty -Path $key -Name $name -ErrorAction SilentlyContinue).$name }
# Settle waits until the update's relaunch of Sonde.exe (a payload that
# quits at once without an argument) has come and gone.
function Settle {
  Start-Sleep 3
  for ($i = 0; $i -lt 20; $i++) {
    if (-not (Get-Process | Where-Object { $_.Path -eq $exe })) { return }
    Start-Sleep 1
  }
  Fail 'Sonde.exe is still running after the relaunch'
}

# Run waits for the installer alone, not for what it starts (the relaunch
# through Explorer), and never for more than 2 minutes.
function Run([string] $file, [string[]] $argv) {
  $p = Start-Process -FilePath $file -ArgumentList $argv -PassThru
  $null = $p.Handle # keeps the exit code readable
  if (-not $p.WaitForExit(120000)) { Fail "$file did not finish" }
  return $p.ExitCode
}

# A fresh install into the custom folder (/D last, unquoted).
if ((Run $InstallerA @('/S', "/D=$Dir")) -ne 0) { Fail 'installer A' }
if ((Hash $exe) -ne (Hash $PayloadA)) { Fail 'installer A did not install its payload' }
if ((Value 'InstallLocation') -ne $Dir) { Fail "InstallLocation is '$(Value 'InstallLocation')', want $Dir" }

# The update waits for the app (a process that quits in 8 s).
$app = Start-Process -FilePath powershell -ArgumentList @('-NoProfile', '-Command', 'Start-Sleep 8') -PassThru
$t0 = Get-Date
$code = Run $InstallerB @('/S', '/UPDATE', "/WAITPID=$($app.Id)")
if ($code -ne 0) { Fail "installer B /UPDATE: exit $code" }
Settle
if (((Get-Date) - $t0).TotalSeconds -lt 6) { Fail 'the update did not wait for the app to quit' }
if ((Hash $exe) -ne (Hash $PayloadB)) { Fail 'the update did not replace Sonde.exe in the custom folder' }
if (Test-Path "$env:ProgramFiles\The Sonde Authors\Sonde") { Fail 'the update installed under Program Files' }
if ((Value 'InstallLocation') -ne $Dir) { Fail 'InstallLocation changed' }
if ((Value 'DisplayVersion') -ne '0.0.2') { Fail "DisplayVersion is '$(Value 'DisplayVersion')', want 0.0.2" }

# The folder comes from the registry, never from the command line.
$elsewhere = Join-Path $env:RUNNER_TEMP 'elsewhere'
$code = Run $InstallerB @('/S', '/UPDATE', "/D=$elsewhere")
if ($code -ne 0) { Fail "installer B /UPDATE /D=: exit $code" }
Settle
if (Test-Path $elsewhere) { Fail '/UPDATE installed into the folder /D= named' }
if ((Hash $exe) -ne (Hash $PayloadB)) { Fail 'the update with /D= did not update the recorded folder' }

# A 0.1.0 install recorded no InstallLocation: DisplayIcon finds the folder.
Remove-ItemProperty -Path $key -Name InstallLocation
$code = Run $InstallerA @('/S', '/UPDATE')
if ($code -ne 0) { Fail "the update of a 0.1.0 install: exit $code" }
Settle
if ((Hash $exe) -ne (Hash $PayloadA)) { Fail 'the DisplayIcon fallback did not find the folder' }
if ((Value 'InstallLocation') -ne $Dir) { Fail 'InstallLocation was not written by the update' }

# Another Sonde window still runs: nothing changes.
$running = Start-Process -FilePath $exe -ArgumentList @('60') -PassThru
Start-Sleep 1
$code = Run $InstallerB @('/S', '/UPDATE')
Stop-Process -Id $running.Id -Force
if ($code -eq 0) { Fail 'the update ran over a running Sonde.exe' }
if ((Hash $exe) -ne (Hash $PayloadA)) { Fail 'the update changed a running install' }
if ((Value 'DisplayVersion') -ne '0.0.1') { Fail 'the refused update changed DisplayVersion' }

Write-Output 'nsis update: ok'
