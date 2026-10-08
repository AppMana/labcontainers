#!/bin/sh
# Derive the lean test base disk from the Ubuntu QGA disk.
#
# usage: lean-base.sh INPUT.qcow2 INPUT_SHA256 OUTPUT.qcow2
#
# The input is the jammy-qga.qcow2 inside vrnetlab/canonical_ubuntu:jammy-qga
# (Ubuntu cloud image plus qemu-guest-agent). The output boots the same kernel
# and userland without snapd seeding, cloud-platform agents, or background
# timers. Nothing is downloaded: the guest appliance has no network.
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: $0 INPUT.qcow2 INPUT_SHA256 OUTPUT.qcow2" >&2
  exit 2
fi
input=$1 expected=$2 output=$3
printf '%s  %s\n' "$expected" "$input" | sha256sum -c -
test ! -e "$output" || { echo "$output exists" >&2; exit 2; }

work=$(mktemp -d "$(dirname -- "$output")/lean-base.XXXXXXXX")
trap 'rm -rf -- "$work"' EXIT HUP INT TERM
qemu-img convert -O qcow2 "$input" "$work/disk.qcow2"

# Packages that only serve other platforms or interactive hosts. snapd's first
# boot seeding alone took 37 s of a 52 s boot on one vCPU. purge without
# --auto-remove so no kernel or library leaves with a meta package.
purge='snapd lxd-agent-loader multipath-tools open-iscsi open-vm-tools pollinate
ubuntu-pro-client ubuntu-advantage-tools landscape-common unattended-upgrades
apport irqbalance motd-news-config'
# Timers that would start package, scrub, or reporting work mid-test.
timers='apt-daily.timer apt-daily-upgrade.timer dpkg-db-backup.timer
e2scrub_all.timer fstrim.timer man-db.timer mdcheck_continue.timer
mdcheck_start.timer mdmonitor-oneshot.timer update-notifier-download.timer
update-notifier-motd.timer'

virt-customize --no-network -a "$work/disk.qcow2" \
  --run-command "DEBIAN_FRONTEND=noninteractive apt-get -y purge $(echo $purge)" \
  --run-command "systemctl disable $(echo $timers)" \
  --run-command 'rm -rf /var/lib/snapd /snap /var/cache/snapd /var/lib/apt/lists/* /var/cache/apt/*.bin' \
  --write '/etc/cloud/cloud.cfg.d/90-labcontainers-datasource.cfg:datasource_list: [ NoCloud, None ]
' \
  --run-command 'dpkg-query -W qemu-guest-agent linux-image-$(ls /lib/modules)'

# Discard freed blocks so the shipped disk holds only live data.
virt-sparsify --compress "$work/disk.qcow2" "$output"
qemu-img check "$output"
sha256sum "$output"
