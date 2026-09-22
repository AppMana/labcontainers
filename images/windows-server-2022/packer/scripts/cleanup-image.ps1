$ErrorActionPreference = 'Stop'
# Current cumulative updates can register Edge for the build Administrator
# without provisioning the Appx package for all users. Sysprep rejects that
# inconsistent state with 0x80073cf2, so remove only the per-user Edge package.
Get-AppxPackage -AllUsers -Name Microsoft.MicrosoftEdge.Stable -ErrorAction SilentlyContinue |
  Remove-AppxPackage -AllUsers -ErrorAction SilentlyContinue
Stop-Service wuauserv -Force -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force C:\Windows\SoftwareDistribution\Download\* -ErrorAction SilentlyContinue
Clear-RecycleBin -Force -ErrorAction SilentlyContinue
Get-ChildItem C:\Windows\Temp -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
Get-ChildItem $env:TEMP -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
Optimize-Volume -DriveLetter C -ReTrim -Verbose
Remove-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon' -Name DefaultPassword -ErrorAction SilentlyContinue
Set-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon' -Name AutoAdminLogon -Value '0'
