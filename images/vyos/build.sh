#!/usr/bin/env bash
# Build the daemon, helper and VyOS wrapper from one clean revision. Installed
# VyOS and Debian QGA media are explicit offline inputs, never fetched by a VM.
set -euo pipefail
if [[ $# != 6 ]]; then
  echo "usage: $0 DISK DISK_SHA256 QGA_ISO QGA_SHA256 OUTPUT_DIR IMAGE_TAG" >&2
  exit 2
fi
disk=$(realpath "$1")
disk_sha=$2
media=$(realpath "$3")
media_sha=$4
output=$(realpath -m "$5")
tag=$6
repo=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
[[ "$disk_sha" =~ ^[0-9a-f]{64}$ && "$media_sha" =~ ^[0-9a-f]{64}$ ]]
[[ "$output" != / && ! -e "$output/labd" && ! -e "$output/labcontainers-guest" ]]
test -z "$(git -C "$repo" status --porcelain)"
revision=$(git -C "$repo" rev-parse HEAD)
printf '%s  %s\n%s  %s\n' "$disk_sha" "$disk" "$media_sha" "$media" | sha256sum -c -
mkdir -p "$output"
cd "$repo"
go build -trimpath -o "$output/labd" ./cmd/labd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$output/labcontainers-guest" ./cmd/labcontainers-guest
helper_sha=$(sha256sum "$output/labcontainers-guest" | cut -d' ' -f1)
docker build -f images/vyos/Dockerfile \
  --build-context "labcontainers-control=$output" \
  --build-context "labcontainers-disk=$(dirname "$disk")" \
  --build-context "labcontainers-media=$(dirname "$media")" \
  --build-context "labcontainers-vyos=$repo/images/vyos" \
  --build-context "labcontainers-windows=$repo/images/windows-server-2022" \
  --build-context "labcontainers-common=$repo/images/common" \
  --build-arg "SOURCE_REVISION=$revision" --build-arg "HELPER_SHA256=$helper_sha" \
  --build-arg "VYOS_SHA256=$disk_sha" --build-arg "QGA_MEDIA_SHA256=$media_sha" \
  --build-arg "IMAGE=$(basename "$disk")" --build-arg "QGA_MEDIA=$(basename "$media")" \
  -t "$tag" images/vyos
docker image inspect "$tag" --format '{{.Id}} {{json .Config.Labels}}'
