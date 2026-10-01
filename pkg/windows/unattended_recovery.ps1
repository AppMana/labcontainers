# Disposable lab-image policy: attempted crash recovery must not wait for a
# human at WinRE. This does not disable signature enforcement or repair disks.
# https://learn.microsoft.com/windows-hardware/drivers/devtest/bcdedit--set
$ErrorActionPreference = 'Stop'
& bcdedit.exe /set '{current}' bootstatuspolicy IgnoreAllFailures
if ($LASTEXITCODE -ne 0) { throw 'Failed to configure unattended lab boot policy' }
# Readback alone can observe the registry cache. The crash test may cut power
# immediately; persist this particular hive before acknowledging provisioning.
# RegCloseKey/ordinary readback does not provide that durability guarantee.
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class LabcontainersBootPolicy {
    [DllImport("advapi32.dll", EntryPoint="RegFlushKey")]
    public static extern int Flush(IntPtr key);
}
'@
$key = [Microsoft.Win32.Registry]::LocalMachine.OpenSubKey('BCD00000000')
if ($null -eq $key) { throw 'Cannot open the active BCD hive for durable provisioning' }
try {
    $status = [LabcontainersBootPolicy]::Flush($key.Handle.DangerousGetHandle())
    if ($status -ne 0) { throw "Cannot flush BCD hive: Windows error $status" }
} finally { $key.Dispose() }
$settings = & bcdedit.exe /enum '{current}'
if ($LASTEXITCODE -ne 0 -or -not (($settings -join "`n") -match '(?im)^bootstatuspolicy\s+IgnoreAllFailures\s*$')) {
    throw 'Unattended lab boot policy did not read back'
}
Write-Output 'UNATTENDED_RECOVERY_CONFIGURED'
