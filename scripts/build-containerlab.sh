#!/usr/bin/env bash
# Build the pinned native CLI with the narrow owned-eth0 correction. This never
# installs over the host Containerlab or edits the Go module cache.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
output=${1:?usage: bash scripts/build-containerlab.sh /absolute/output/containerlab}
[[ "$output" = /* ]] || { echo 'output must be absolute' >&2; exit 2; }
cd "$root"
module=$(go mod download -json github.com/srl-labs/containerlab)
version=$(printf '%s' "$module" | jq -r .Version)
[[ "$version" = v0.79.0 ]] || { echo "rebase the native patch before building $version" >&2; exit 2; }
source_dir=$(printf '%s' "$module" | jq -r .Dir)
build_dir=$(mktemp -d -t labcontainers-native-build.XXXXXXXX)
trap 'rm -rf -- "$build_dir"' EXIT
cp -a "$source_dir/." "$build_dir/"
chmod -R u+w "$build_dir"
cd "$build_dir"
git apply "$root/patches/containerlab-owned-eth0.patch"
GOWORK=off go test ./links ./core
patch_sum=$(sha256sum "$root/patches/containerlab-owned-eth0.patch")
patch_sum=${patch_sum%% *}
GOWORK=off go build -trimpath \
  -ldflags "-X github.com/srl-labs/containerlab/cmd.Version=0.79.0 -X github.com/srl-labs/containerlab/cmd.commit=5ae50094+patch-$patch_sum" \
  -o "$output" .
"$output" version
