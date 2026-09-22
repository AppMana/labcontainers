$ErrorActionPreference = 'Stop'
Set-NetConnectionProfile -NetworkCategory Private -ErrorAction SilentlyContinue
Enable-PSRemoting -Force -SkipNetworkProfileCheck
winrm set winrm/config/service '@{AllowUnencrypted="true"}' | Out-Null
winrm set winrm/config/service/auth '@{Basic="true"}' | Out-Null
winrm set winrm/config/service/auth '@{Negotiate="true"}' | Out-Null
Set-Service WinRM -StartupType Automatic
New-NetFirewallRule -DisplayName 'Packer WinRM' -Direction Inbound -Protocol TCP -LocalPort 5985 -Action Allow -ErrorAction SilentlyContinue | Out-Null
