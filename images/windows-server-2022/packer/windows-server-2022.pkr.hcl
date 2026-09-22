packer {
  required_version = ">= 1.15.0"
  required_plugins {
    qemu = {
      source  = "github.com/hashicorp/qemu"
      version = ">= 1.1.6"
    }
    windows-update = {
      source  = "github.com/rgl/windows-update"
      version = ">= 0.16.0"
    }
  }
}

variable "iso_url" {
  type        = string
  description = "Licensed Windows Server 2022 ISO path or URL"
}

variable "iso_checksum" {
  type        = string
  description = "Windows ISO checksum, including the sha256: prefix"
}

variable "virtio_win_directory" {
  type        = string
  description = "Absolute path to the extracted contents of a pinned virtio-win ISO"
}

variable "windows_password" {
  type      = string
  sensitive = true
  validation {
    condition     = length(var.windows_password) >= 14
    error_message = "The Windows password must contain at least 14 characters."
  }
}

variable "output_directory" {
  type    = string
  default = "output/windows-server-2022"
}

variable "provisioning_scripts" {
  type        = list(string)
  default     = []
  description = "Optional project-owned PowerShell image layers, run after the shared VM preparation and before cleanup"
}

source "qemu" "windows_server_2022" {
  accelerator      = "kvm"
  machine_type     = "pc"
  cpu_model        = "host"
  cpus             = 4
  memory           = 8192
  headless         = true
  boot_wait        = "2s"
  boot_command     = ["<enter>"]
  format           = "qcow2"
  disk_interface   = "ide"
  disk_size        = "80G"
  disk_compression = true
  net_device       = "e1000"
  iso_url          = var.iso_url
  iso_checksum     = var.iso_checksum
  output_directory = var.output_directory
  vm_name          = "windows-server-2022-labcontainers.qcow2"

  communicator   = "winrm"
  winrm_username = "Administrator"
  winrm_password = var.windows_password
  winrm_timeout  = "90m"
  winrm_use_ntlm = true
  winrm_no_proxy = true

  cd_label = "LABCONTAINERS"
  cd_content = {
    "Autounattend.xml" = templatefile("${path.root}/autounattend.xml.pkrtpl", {
      password = var.windows_password
    })
    "enable-winrm.ps1" = file("${path.root}/scripts/enable-winrm.ps1")
    "sysprep-unattend.xml" = templatefile("${path.root}/sysprep-unattend.xml.pkrtpl", {
      password = var.windows_password
    })
  }
  cd_files = ["${var.virtio_win_directory}/*"]

  shutdown_command = "C:\\Windows\\System32\\Sysprep\\Sysprep.exe /generalize /oobe /shutdown /mode:vm /unattend:C:\\Labcontainers\\sysprep-unattend.xml"
  shutdown_timeout = "30m"
}

build {
  name    = "labcontainers-windows-server-2022"
  sources = ["source.qemu.windows_server_2022"]

  # Updating at image-build time makes every test overlay both fast and
  # reproducible, and avoids the old first-L2Bridge crash on RTM media.
  provisioner "windows-update" {
    search_criteria = "IsInstalled=0"
    filters = [
      "exclude:$_.Title -like '*Preview*'",
      "include:$true",
    ]
    update_limit = 50
  }

  provisioner "powershell" {
    scripts = concat(
      ["${path.root}/scripts/prepare-image.ps1"],
      var.provisioning_scripts,
      ["${path.root}/scripts/cleanup-image.ps1"],
    )
  }
}
