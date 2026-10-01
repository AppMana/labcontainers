# Disposable lab-image policy: attempted crash recovery must not wait for a
# human at WinRE. This does not disable signature enforcement or repair disks.
# https://learn.microsoft.com/windows-hardware/drivers/devtest/bcdedit--set
$ErrorActionPreference = 'Stop'
& bcdedit.exe /set '{current}' bootstatuspolicy IgnoreAllFailures
if ($LASTEXITCODE -ne 0) { throw 'Failed to configure unattended lab boot policy' }
$settings = & bcdedit.exe /enum '{current}'
if ($LASTEXITCODE -ne 0 -or -not (($settings -join "`n") -match '(?im)^bootstatuspolicy\s+IgnoreAllFailures\s*$')) {
    throw 'Unattended lab boot policy did not read back'
}
Write-Output 'UNATTENDED_RECOVERY_CONFIGURED'
