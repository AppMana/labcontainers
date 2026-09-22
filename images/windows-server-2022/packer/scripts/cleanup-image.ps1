$ErrorActionPreference = 'Stop'
# Current cumulative updates can register Edge for the build Administrator
# without provisioning the Appx package for all users. Sysprep rejects that
# inconsistent state with 0x80073cf2, so remove only the per-user Edge package.
Get-AppxPackage -AllUsers -Name Microsoft.MicrosoftEdge.Stable -ErrorAction SilentlyContinue |
  Remove-AppxPackage -AllUsers -ErrorAction SilentlyContinue
Stop-Service wuauserv -Force -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force C:\Windows\SoftwareDistribution\Download\* -ErrorAction SilentlyContinue
Clear-RecycleBin -Force -ErrorAction SilentlyContinue
# Packer sources its generated environment script again while invoking the
# shutdown command, so preserve its files until the communicator is gone.
@('C:\Windows\Temp', $env:TEMP) | Select-Object -Unique | ForEach-Object {
  Get-ChildItem $_ -Force -ErrorAction SilentlyContinue |
    Where-Object Name -NotLike 'packer-*' |
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
}
Optimize-Volume -DriveLetter C -ReTrim -Verbose
Remove-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon' -Name DefaultPassword -ErrorAction SilentlyContinue
Set-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon' -Name AutoAdminLogon -Value '0'
