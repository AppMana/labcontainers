#!/usr/bin/env bash
# Generate bindings from the exact schema shipped by our native Go dependency.
set -euo pipefail
cd "$(dirname "$0")/.."
clab_module=$(go mod download -json github.com/srl-labs/containerlab)
clab_dir=$(printf '%s' "$clab_module" | jq -r .Dir)
clab_version=$(printf '%s' "$clab_module" | jq -r .Version)
mkdir -p schemas/containerlab python/labcontainers
install -m 644 "$clab_dir/schemas/clab.schema.json" schemas/containerlab/clab.schema.json
install -m 644 "$clab_dir/LICENSE" schemas/containerlab/LICENSE
install -m 644 "$clab_dir/LICENSE" python/labcontainers/CONTAINERLAB-LICENSE
uvx --python 3.12 --from datamodel-code-generator==0.28.5 datamodel-codegen \
  --input schemas/containerlab/clab.schema.json --input-file-type jsonschema \
  --output python/labcontainers/containerlab_generated.py \
  --output-model-type pydantic_v2.BaseModel --target-python-version 3.10 \
  --base-class labcontainers._schema.SchemaObject \
  --class-name Config --use-standard-collections --use-union-operator \
  --use-schema-description --allow-population-by-field-name \
  --enum-field-as-literal all --strict-types str int float bool \
  --disable-timestamp --use-double-quotes \
  --custom-file-header "# Generated from srl-labs/containerlab ${clab_version}/schemas/clab.schema.json.
# DO NOT EDIT. Regenerate with: make generate-containerlab
# Upstream schema: BSD-3-Clause; see CONTAINERLAB-LICENSE."
