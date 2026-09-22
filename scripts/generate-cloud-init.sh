#!/usr/bin/env bash
# Generate upstream cloud-init network v2 bindings; no Labcontainers schema.
set -euo pipefail
cd "$(dirname "$0")/.."
revision=e682bef5be7f18d6c803fbc3ad35e277fdf03e39 # cloud-init 25.2
upstream="https://raw.githubusercontent.com/canonical/cloud-init/$revision"
mkdir -p schemas/cloud-init pkg/cloudinit/networkconfig python/labcontainers
curl -fsSL "$upstream/cloudinit/config/schemas/schema-network-config-v2.json" \
  -o schemas/cloud-init/schema-network-config-v2.json
echo '0ad5a7b76e130909f4f6482c010c1765cc7bb8990fbc2b72186a3f2380a1788d  schemas/cloud-init/schema-network-config-v2.json' | sha256sum --check
curl -fsSL "$upstream/LICENSE" -o schemas/cloud-init/LICENSE
install -m 644 schemas/cloud-init/LICENSE python/labcontainers/CLOUD-INIT-LICENSE
# JSON Schema defaults additionalProperties to true. Make that existing
# semantic explicit for the Go generator, which otherwise drops open fields.
generation_dir=$(mktemp -d)
trap 'rm -rf -- "$generation_dir"' EXIT
jq 'walk(if type == "object" and has("properties") and (has("additionalProperties") | not) then . + {additionalProperties: true} else . end)' \
  schemas/cloud-init/schema-network-config-v2.json > "$generation_dir/schema-network-config-v2.json"
go run github.com/atombender/go-jsonschema@v0.24.1 \
  --package networkconfig --tags json \
  --output pkg/cloudinit/networkconfig/zz_generated.go \
  "$generation_dir/schema-network-config-v2.json"
python3 scripts/cloudinit-open-fields.py
gofmt -w pkg/cloudinit/networkconfig/zz_generated.go pkg/cloudinit/networkconfig/zz_open_generated.go
uvx --python 3.12 --from datamodel-code-generator==0.28.5 datamodel-codegen \
  --input schemas/cloud-init/schema-network-config-v2.json --input-file-type jsonschema \
  --output python/labcontainers/cloudinit_generated.py \
  --output-model-type pydantic_v2.BaseModel --target-python-version 3.10 \
  --base-class labcontainers._schema.SchemaObject \
  --use-standard-collections --use-union-operator \
  --allow-population-by-field-name --enum-field-as-literal all \
  --strict-types str int float bool --disable-timestamp --use-double-quotes \
  --custom-file-header "# Generated from canonical/cloud-init 25.2 ($revision).
# DO NOT EDIT. Regenerate with: make generate-cloud-init
# Upstream license: GPL-3.0 or Apache-2.0; see CLOUD-INIT-LICENSE."
