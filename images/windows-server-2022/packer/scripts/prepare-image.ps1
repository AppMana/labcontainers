$ErrorActionPreference = 'Stop'

# Install all signed Server 2022 virtio drivers before the image is detached
# from Packer's emulated IDE/e1000 hardware.
$virtio = Get-Volume -FileSystemLabel LABCONTAINERS -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $virtio) { throw 'Packer auxiliary ISO is not attached' }
$virtioRoot = "$($virtio.DriveLetter):\"
$driverRoots = @('NetKVM', 'vioserial', 'viostor', 'vioscsi', 'Balloon')
foreach ($driver in $driverRoots) {
  $path = Join-Path $virtioRoot "$driver\2k22\amd64"
  if (Test-Path $path) { pnputil.exe /add-driver "$path\*.inf" /subdirs /install | Out-Null }
}

$qga = Get-ChildItem -Path $virtioRoot -Filter 'qemu-ga-x86_64.msi' -Recurse | Select-Object -First 1
if (-not $qga) { throw 'qemu-ga-x86_64.msi is missing from virtio-win ISO' }
$qgaProcess = Start-Process msiexec.exe -ArgumentList @('/i', $qga.FullName, '/qn', '/norestart') -Wait -PassThru
if ($qgaProcess.ExitCode -notin @(0, 3010)) { throw "QEMU Guest Agent install failed: $($qgaProcess.ExitCode)" }
$qgaExecutable = Join-Path $env:ProgramFiles 'qemu-ga\qemu-ga.exe'
if (-not (Get-Service qemu-ga -ErrorAction SilentlyContinue)) {
  & $qgaExecutable -s install
}
Set-Service qemu-ga -StartupType Automatic
Start-Service qemu-ga

Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' -Name LongPathsEnabled -Value 1 -Type DWord

$sysprepAnswer = Join-Path $virtioRoot 'sysprep-unattend.xml'
if (-not (Test-Path $sysprepAnswer)) { throw 'Sysprep answer file is missing from the Packer auxiliary ISO' }
New-Item -ItemType Directory -Force C:\Labcontainers | Out-Null
Copy-Item -Force $sysprepAnswer C:\Labcontainers\sysprep-unattend.xml
Set-Content C:\Labcontainers\image.json ('{"schema":1,"os":"windows-server-2022","qga":true,"generalized":true}') -Encoding Ascii
