# Upstream cloud-init network schema

Unmodified source: canonical/cloud-init release `25.2`, commit
`e682bef5be7f18d6c803fbc3ad35e277fdf03e39`,
`cloudinit/config/schemas/schema-network-config-v2.json`.
The generator checks SHA-256
`0ad5a7b76e130909f4f6482c010c1765cc7bb8990fbc2b72186a3f2380a1788d`.
The original upstream license notice is included as `LICENSE`.

Run `make generate-cloud-init`. Go models use go-jsonschema `v0.24.1`;
Python models use datamodel-code-generator `0.28.5`. No field inventory or
Labcontainers network schema is maintained. Go generation makes the JSON
Schema default `additionalProperties: true` explicit in a temporary projection;
the checked-in upstream schema is unchanged. A narrowly generated JSON codec
correction preserves those open fields on encoding and decoding, including
large numeric values. DHCP's boolean/string union uses the generator's native
wrapper types. Do not use its models-only mode: that omits scalar codecs.

Use the generated `NetworkConfigVersion2` definition for standalone NoCloud
network input. File helpers serialize JSON, which cloud-init reads as YAML,
and do not insert DHCP, DNS, gateways, routes, addresses, or interfaces.
These bindings are not a replacement for cloud-init/Netplan runtime validation
and are not a model of every option supported by every Netplan release.
