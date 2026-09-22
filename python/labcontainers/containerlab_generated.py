# Generated from srl-labs/containerlab v0.79.0/schemas/clab.schema.json.
# DO NOT EDIT. Regenerate with: make generate-containerlab
# Upstream schema: BSD-3-Clause; see CONTAINERLAB-LICENSE.

from __future__ import annotations

from typing import Any, Literal

from pydantic import (
    ConfigDict,
    Field,
    RootModel,
    StrictBool,
    StrictFloat,
    StrictInt,
    StrictStr,
    confloat,
    conint,
    constr,
)

from labcontainers._schema import SchemaObject


class Env(
    RootModel[
        dict[constr(pattern=r".+", strict=True), StrictStr | StrictFloat | StrictBool]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: dict[
        constr(pattern=r".+", strict=True), StrictStr | StrictFloat | StrictBool
    ] = Field(..., description="environment variables")


class Port(
    RootModel[
        constr(
            pattern=r"^(([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])(%[\p{N}\p{L}]+)?:([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$|^(([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])(%[\p{N}\p{L}]+)?:([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])+(/tcp|/udp|/sctp)$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])+(/tcp|/udp|/sctp)$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])-([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])+(/tcp|/udp|/sctp)?$",
            strict=True,
        )
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: constr(
        pattern=r"^(([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])(%[\p{N}\p{L}]+)?:([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$|^(([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])(%[\p{N}\p{L}]+)?:([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])+(/tcp|/udp|/sctp)$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])+(/tcp|/udp|/sctp)$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$|^([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])-([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]):([0-9]{1,4}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])+(/tcp|/udp|/sctp)?$",
        strict=True,
    )


class Credentials(SchemaObject):
    """
    username and password for SSH/NETCONF/GNMI/etc. (overrides kind default)
    """

    model_config = ConfigDict(
        populate_by_name=True,
    )
    username: StrictStr | None = Field(
        None,
        description="username to use when accessing the node over SSH/NETCONF/GNMI/etc.",
    )
    password: StrictStr | None = Field(
        None,
        description="password to use when accessing the node over SSH/NETCONF/GNMI/etc.",
    )
    identity_file: StrictStr | None = Field(
        None,
        alias="identity-file",
        description="path to the SSH private key used to authenticate against the node; rendered as an IdentityFile directive in the generated ssh_config.",
    )


class Ipv4Item(
    RootModel[
        constr(
            pattern=r"^(?:$|(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])/(?:[0-9]|[12][0-9]|3[0-2]))$",
            strict=True,
        )
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: constr(
        pattern=r"^(?:$|(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])/(?:[0-9]|[12][0-9]|3[0-2]))$",
        strict=True,
    )


class Ipv6Item(
    RootModel[
        constr(
            pattern=r"^(?:$|(?:(?:[A-Fa-f0-9]{1,4}:){7}[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,7}:|(?:[A-Fa-f0-9]{1,4}:){1,6}:[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,5}(?::[A-Fa-f0-9]{1,4}){1,2}|(?:[A-Fa-f0-9]{1,4}:){1,4}(?::[A-Fa-f0-9]{1,4}){1,3}|(?:[A-Fa-f0-9]{1,4}:){1,3}(?::[A-Fa-f0-9]{1,4}){1,4}|(?:[A-Fa-f0-9]{1,4}:){1,2}(?::[A-Fa-f0-9]{1,4}){1,5}|[A-Fa-f0-9]{1,4}:(?::[A-Fa-f0-9]{1,4}){1,6}|:(?:(?::[A-Fa-f0-9]{1,4}){1,7}|:))/(?:[0-9]|[1-9][0-9]|1[01][0-9]|12[0-8]))$",
            strict=True,
        )
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: constr(
        pattern=r"^(?:$|(?:(?:[A-Fa-f0-9]{1,4}:){7}[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,7}:|(?:[A-Fa-f0-9]{1,4}:){1,6}:[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,5}(?::[A-Fa-f0-9]{1,4}){1,2}|(?:[A-Fa-f0-9]{1,4}:){1,4}(?::[A-Fa-f0-9]{1,4}){1,3}|(?:[A-Fa-f0-9]{1,4}:){1,3}(?::[A-Fa-f0-9]{1,4}){1,4}|(?:[A-Fa-f0-9]{1,4}:){1,2}(?::[A-Fa-f0-9]{1,4}){1,5}|[A-Fa-f0-9]{1,4}:(?::[A-Fa-f0-9]{1,4}){1,6}|:(?:(?::[A-Fa-f0-9]{1,4}){1,7}|:))/(?:[0-9]|[1-9][0-9]|1[01][0-9]|12[0-8]))$",
        strict=True,
    )


class EndpointVars(SchemaObject):
    """
    per-endpoint variables
    """

    model_config = ConfigDict(
        extra="allow",
        populate_by_name=True,
    )


class LinkVxlanVni(RootModel[conint(ge=1, le=16777215, strict=True)]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: conint(ge=1, le=16777215, strict=True) = Field(..., description="VXLAN VNI")


class LinkVxlanDstport(RootModel[conint(ge=1, le=65535, strict=True)]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: conint(ge=1, le=65535, strict=True) = Field(
        ..., description="Destination UDP port"
    )


class LinkVxlanSrcport(RootModel[conint(ge=1, le=65535, strict=True)]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: conint(ge=1, le=65535, strict=True) = Field(
        ..., description="Source UDP port"
    )


class LinkVars(SchemaObject):
    """
    link-scoped variables used by config engine
    """

    model_config = ConfigDict(
        extra="allow",
        populate_by_name=True,
    )


class Labels(
    RootModel[
        dict[
            constr(pattern=r".+", strict=True),
            constr(min_length=1, strict=True) | confloat(ge=0.0, strict=True),
        ]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: dict[
        constr(pattern=r".+", strict=True),
        constr(min_length=1, strict=True) | confloat(ge=0.0, strict=True),
    ] = Field(..., description="container labels")


class LinkHostInterface(RootModel[StrictStr]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: StrictStr = Field(..., description="Name of the host interface")


class LinkMacvlanMode(
    RootModel[Literal["private", "vepa", "bridge", "passthru", "source"]]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal["private", "vepa", "bridge", "passthru", "source"] = Field(
        ..., description="MACVLAN operating mode"
    )


class ExtrasConfig(SchemaObject):
    """
    node's extra configurations
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    ceos_copy_to_flash: list[StrictStr] | None = Field(
        None,
        alias="ceos-copy-to-flash",
        description="list of cEOS-specific configuration or override files to be copied to the flash directory and evaluated on startup",
        min_length=1,
    )


class ConfigConfig(SchemaObject):
    """
    containerlab config engine parameters
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    vars: dict[str, Any] | None = Field(
        None, description="config variables passed to config engine"
    )


class CertificateConfig(SchemaObject):
    """
    Node's Certificate configuration option
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    issue: Any | None = Field(
        None, description="Set to `true` to generate a TLS certificate for the node"
    )
    sans: list[StrictStr] | None = Field(
        None, description="list of subject alternative names (SAN) to use for this node"
    )
    key_size: StrictInt | None = Field(
        None, alias="key-size", description="size of the to be generated key"
    )
    validity_duration: StrictStr | None = Field(
        None,
        alias="validity-duration",
        description="Duration for how long the certificate issued by the CA will be valid.",
    )


class HealthcheckConfig(SchemaObject):
    """
    Node's Healthcheck configuration option
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    test: list[StrictStr] | None = Field(None, description="test command")
    interval: StrictInt | None = Field(None, description="test execution interval")
    retries: StrictInt | None = Field(None, description="test execution retries")
    timeout: StrictInt | None = Field(
        None, description="test execution timeout in seconds"
    )
    start_period: StrictInt | None = Field(
        None,
        alias="start-period",
        description="time in seconds to wait before starting the healthcheck",
    )


class DnsConfig(SchemaObject):
    """
    Node's DNS configuration option
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    servers: list[StrictStr] | None = Field(None, description="DNS server addresses")
    search: list[StrictStr] | None = Field(None, description="DNS search domains")
    options: list[StrictStr] | None = Field(None, description="DNS options")


class CertificateAuthorityConfig1(SchemaObject):
    """
    Certificate Authority
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    cert: StrictStr = Field(
        ...,
        description="Path to the CA certificate file. If set, it is expected that the CA certificate already exists by that path",
    )
    key: StrictStr = Field(
        ...,
        description="Path to the CA key file. If set, it is expected that the CA certificate already exists by that path",
    )
    key_size: StrictInt | None = Field(
        None,
        alias="key-size",
        description="Key size. Can only be set if the external CA certificate is not provided",
    )
    validity_duration: StrictStr | None = Field(
        None,
        alias="validity-duration",
        description="CA certificate validity duration. Can only be set if the external CA certificate is not provided",
    )


class CertificateAuthorityConfig2(SchemaObject):
    """
    Certificate Authority
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    cert: StrictStr | None = Field(
        None,
        description="Path to the CA certificate file. If set, it is expected that the CA certificate already exists by that path",
    )
    key: StrictStr | None = Field(
        None,
        description="Path to the CA key file. If set, it is expected that the CA certificate already exists by that path",
    )
    key_size: StrictInt = Field(
        ...,
        alias="key-size",
        description="Key size. Can only be set if the external CA certificate is not provided",
    )
    validity_duration: StrictStr | None = Field(
        None,
        alias="validity-duration",
        description="CA certificate validity duration. Can only be set if the external CA certificate is not provided",
    )


class CertificateAuthorityConfig3(SchemaObject):
    """
    Certificate Authority
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    cert: StrictStr | None = Field(
        None,
        description="Path to the CA certificate file. If set, it is expected that the CA certificate already exists by that path",
    )
    key: StrictStr | None = Field(
        None,
        description="Path to the CA key file. If set, it is expected that the CA certificate already exists by that path",
    )
    key_size: StrictInt | None = Field(
        None,
        alias="key-size",
        description="Key size. Can only be set if the external CA certificate is not provided",
    )
    validity_duration: StrictStr = Field(
        ...,
        alias="validity-duration",
        description="CA certificate validity duration. Can only be set if the external CA certificate is not provided",
    )


class CertificateAuthorityConfig(
    RootModel[
        CertificateAuthorityConfig1
        | CertificateAuthorityConfig2
        | CertificateAuthorityConfig3
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: (
        CertificateAuthorityConfig1
        | CertificateAuthorityConfig2
        | CertificateAuthorityConfig3
    ) = Field(..., description="Certificate Authority")


class StagesEnum(
    RootModel[Literal["create", "create-links", "configure", "healthy", "exit"]]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal["create", "create-links", "configure", "healthy", "exit"]


class StageExecItem(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    command: StrictStr = Field(..., description="Shell command to execute")
    target: StrictStr | None = Field(
        "container",
        description="Location to run the command (e.g. 'container', 'host')",
    )
    phase: Literal["on-enter", "on-exit"] = Field(
        ..., description="Phase to execute this command (on-enter or on-exit)"
    )


class StageExecList(RootModel[list[StrictStr]]):
    """
    list of commands to execute
    """

    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: list[StrictStr] = Field(
        ..., description="list of commands to execute", min_length=1
    )


class Mtu(RootModel[confloat(ge=1.0, le=65535.0, strict=True)]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: confloat(ge=1.0, le=65535.0, strict=True) = Field(
        ..., description="MTU for the custom network"
    )


class Ipv4Addr(
    RootModel[
        constr(
            pattern=r"^(([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])(%[\p{N}\p{L}]+)?$",
            strict=True,
        )
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: constr(
        pattern=r"^(([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])(%[\p{N}\p{L}]+)?$",
        strict=True,
    ) = Field(..., description="IPv4 address")


class Ipv6Addr(
    RootModel[
        constr(
            pattern=r"^((:|[0-9a-fA-F]{0,4}):)([0-9a-fA-F]{0,4}:){0,5}((([0-9a-fA-F]{0,4}:)?(:|[0-9a-fA-F]{0,4}))|(((25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])))(%[\p{N}\p{L}]+)?$",
            strict=True,
        )
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: constr(
        pattern=r"^((:|[0-9a-fA-F]{0,4}):)([0-9a-fA-F]{0,4}:){0,5}((([0-9a-fA-F]{0,4}:)?(:|[0-9a-fA-F]{0,4}))|(((25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])))(%[\p{N}\p{L}]+)?$",
        strict=True,
    ) = Field(..., description="IPv6 address")


class Ipv4Prefix(
    RootModel[
        constr(
            pattern=r"^(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])/(?:[0-9]|[12][0-9]|3[0-2])$",
            strict=True,
        )
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: constr(
        pattern=r"^(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])/(?:[0-9]|[12][0-9]|3[0-2])$",
        strict=True,
    ) = Field(..., description="IPv4 address in CIDR notation")


class Ipv6Prefix(
    RootModel[
        constr(
            pattern=r"^(?:(?:[A-Fa-f0-9]{1,4}:){7}[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,7}:|(?:[A-Fa-f0-9]{1,4}:){1,6}:[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,5}(?::[A-Fa-f0-9]{1,4}){1,2}|(?:[A-Fa-f0-9]{1,4}:){1,4}(?::[A-Fa-f0-9]{1,4}){1,3}|(?:[A-Fa-f0-9]{1,4}:){1,3}(?::[A-Fa-f0-9]{1,4}){1,4}|(?:[A-Fa-f0-9]{1,4}:){1,2}(?::[A-Fa-f0-9]{1,4}){1,5}|[A-Fa-f0-9]{1,4}:(?::[A-Fa-f0-9]{1,4}){1,6}|:(?:(?::[A-Fa-f0-9]{1,4}){1,7}|:))/(?:[0-9]|[1-9][0-9]|1[01][0-9]|12[0-8])$",
            strict=True,
        )
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: constr(
        pattern=r"^(?:(?:[A-Fa-f0-9]{1,4}:){7}[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,7}:|(?:[A-Fa-f0-9]{1,4}:){1,6}:[A-Fa-f0-9]{1,4}|(?:[A-Fa-f0-9]{1,4}:){1,5}(?::[A-Fa-f0-9]{1,4}){1,2}|(?:[A-Fa-f0-9]{1,4}:){1,4}(?::[A-Fa-f0-9]{1,4}){1,3}|(?:[A-Fa-f0-9]{1,4}:){1,3}(?::[A-Fa-f0-9]{1,4}){1,4}|(?:[A-Fa-f0-9]{1,4}:){1,2}(?::[A-Fa-f0-9]{1,4}){1,5}|[A-Fa-f0-9]{1,4}:(?::[A-Fa-f0-9]{1,4}){1,6}|:(?:(?::[A-Fa-f0-9]{1,4}){1,7}|:))/(?:[0-9]|[1-9][0-9]|1[01][0-9]|12[0-8])$",
        strict=True,
    ) = Field(..., description="IPv6 address in CIDR notation")


class SrosCardTypes(
    RootModel[
        Literal[
            "cpm-1se/imm36-800g-qsfpdd",
            "cpm-1x/dms24-800g-qsfpdd-1",
            "cpm-1x/i24-800g-qsfpdd-1",
            "cpm-1x/i40-200g-sfpdd+6-800g-qsfpdd-1",
            "cpm-1x/i48-400g-qsfpdd-1",
            "cpm-1x/i48-800g-qsfpdd-1x",
            "cpm-1x/i80-200g-sfpdd+12-400g-qsfpdd-1",
            "cpm-1x/i80-200g-sfpdd+12-800g-qsfpdd-1x",
            "cpm-ixr-e-gnss/imm14-10g-sfp++4-1g-tx",
            "cpm-ixr-e-gnss/imm24-sfp++8-sfp28+2-qsfp28",
            "cpm-ixr-e/imm14-10g-sfp++4-1g-tx",
            "cpm-ixr-e/imm24-sfp++8-sfp28+2-qsfp28",
            "cpm-ixr-e2",
            "cpm-ixr-e2c",
            "cpm-ixr-e2n/imm4-sfp+4-sfp+",
            "cpm-ixr-e3c/imm4-qsfp28+16-sfp28+8-sfp56",
            "cpm-ixr-e3x/imm16-sfp112+15-sfp56+6-qsfpdd",
            "cpm-ixr-ec",
            "cpm-ixr-s/imm48-sfp++6-qsfp28",
            "cpm-ixr-x/imm32-qsfp28+4-qsfpdd",
            "cpm-ixr-x/imm6-qsfpdd+48-sfp56",
            "cpm-sar-hm",
            "cpm-sar-hmc",
            "dms24-800g-qsfpdd-1",
            "i24-800g-qsfpdd-1",
            "i40-200g-sfpdd+6-800g-qsfpdd-1",
            "i48-400g-qsfpdd-1",
            "i48-800g-qsfpdd-1x",
            "i80-200g-sfpdd+12-400g-qsfpdd-1",
            "i80-200g-sfpdd+12-800g-qsfpdd-1x",
            "imm-2pac-fp3",
            "imm12-sfp28+2-qsfp28",
            "imm14-10g-sfp++4-1g-tx",
            "imm2-qsfpdd+2-qsfp28+24-sfp28",
            "imm24-sfp++8-sfp28+2-qsfp28",
            "imm32-qsfp28+4-qsfpdd",
            "imm36-100g-qsfp28",
            "imm36-800g-qsfpdd",
            "imm36-qsfpdd",
            "imm4-100gb-cfp4",
            "imm4-100gb-cxp",
            "imm4-1g-tx+20-1g-sfp+6-10g-sfp+",
            "imm40-10gb-sfp",
            "imm40-10gb-sfp-ptp",
            "imm48-1gb-sfp-c",
            "imm48-sfp++6-qsfp28",
            "imm48-sfp+2-qsfp28",
            "imm6-qsfpdd+48-sfp56",
            "iom-1",
            "iom-a",
            "iom-e",
            "iom-ixr-r4",
            "iom-ixr-r6",
            "iom-ixr-r6d",
            "iom-sar",
            "iom-sar-hm",
            "iom-sar-hmc",
            "iom-v",
            "iom4-e",
            "iom4-e-b",
            "iom4-e-hs",
            "iom5-e",
            "xcm-14s",
            "xcm-14s-b",
            "xcm-1s",
            "xcm-2s",
            "xcm-2se",
            "xcm-7s",
            "xcm-7s-b",
            "xcm-x20",
            "xcm2-14s",
            "xcm2-7s",
            "xcm2-x20",
            "xcmc-2se",
        ]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal[
        "cpm-1se/imm36-800g-qsfpdd",
        "cpm-1x/dms24-800g-qsfpdd-1",
        "cpm-1x/i24-800g-qsfpdd-1",
        "cpm-1x/i40-200g-sfpdd+6-800g-qsfpdd-1",
        "cpm-1x/i48-400g-qsfpdd-1",
        "cpm-1x/i48-800g-qsfpdd-1x",
        "cpm-1x/i80-200g-sfpdd+12-400g-qsfpdd-1",
        "cpm-1x/i80-200g-sfpdd+12-800g-qsfpdd-1x",
        "cpm-ixr-e-gnss/imm14-10g-sfp++4-1g-tx",
        "cpm-ixr-e-gnss/imm24-sfp++8-sfp28+2-qsfp28",
        "cpm-ixr-e/imm14-10g-sfp++4-1g-tx",
        "cpm-ixr-e/imm24-sfp++8-sfp28+2-qsfp28",
        "cpm-ixr-e2",
        "cpm-ixr-e2c",
        "cpm-ixr-e2n/imm4-sfp+4-sfp+",
        "cpm-ixr-e3c/imm4-qsfp28+16-sfp28+8-sfp56",
        "cpm-ixr-e3x/imm16-sfp112+15-sfp56+6-qsfpdd",
        "cpm-ixr-ec",
        "cpm-ixr-s/imm48-sfp++6-qsfp28",
        "cpm-ixr-x/imm32-qsfp28+4-qsfpdd",
        "cpm-ixr-x/imm6-qsfpdd+48-sfp56",
        "cpm-sar-hm",
        "cpm-sar-hmc",
        "dms24-800g-qsfpdd-1",
        "i24-800g-qsfpdd-1",
        "i40-200g-sfpdd+6-800g-qsfpdd-1",
        "i48-400g-qsfpdd-1",
        "i48-800g-qsfpdd-1x",
        "i80-200g-sfpdd+12-400g-qsfpdd-1",
        "i80-200g-sfpdd+12-800g-qsfpdd-1x",
        "imm-2pac-fp3",
        "imm12-sfp28+2-qsfp28",
        "imm14-10g-sfp++4-1g-tx",
        "imm2-qsfpdd+2-qsfp28+24-sfp28",
        "imm24-sfp++8-sfp28+2-qsfp28",
        "imm32-qsfp28+4-qsfpdd",
        "imm36-100g-qsfp28",
        "imm36-800g-qsfpdd",
        "imm36-qsfpdd",
        "imm4-100gb-cfp4",
        "imm4-100gb-cxp",
        "imm4-1g-tx+20-1g-sfp+6-10g-sfp+",
        "imm40-10gb-sfp",
        "imm40-10gb-sfp-ptp",
        "imm48-1gb-sfp-c",
        "imm48-sfp++6-qsfp28",
        "imm48-sfp+2-qsfp28",
        "imm6-qsfpdd+48-sfp56",
        "iom-1",
        "iom-a",
        "iom-e",
        "iom-ixr-r4",
        "iom-ixr-r6",
        "iom-ixr-r6d",
        "iom-sar",
        "iom-sar-hm",
        "iom-sar-hmc",
        "iom-v",
        "iom4-e",
        "iom4-e-b",
        "iom4-e-hs",
        "iom5-e",
        "xcm-14s",
        "xcm-14s-b",
        "xcm-1s",
        "xcm-2s",
        "xcm-2se",
        "xcm-7s",
        "xcm-7s-b",
        "xcm-x20",
        "xcm2-14s",
        "xcm2-7s",
        "xcm2-x20",
        "xcmc-2se",
    ] = Field(..., description="Card types for SR OS")


class SrosCpmTypes(
    RootModel[
        Literal[
            "cpiom-ixr-r6",
            "cpiom-ixr-r6d",
            "cpm-1",
            "cpm-1s",
            "cpm-1se",
            "cpm-1se/imm36-800g-qsfpdd",
            "cpm-1x",
            "cpm-1x/dms24-800g-qsfpdd-1",
            "cpm-1x/i24-800g-qsfpdd-1",
            "cpm-1x/i40-200g-sfpdd+6-800g-qsfpdd-1",
            "cpm-1x/i48-400g-qsfpdd-1",
            "cpm-1x/i48-800g-qsfpdd-1x",
            "cpm-1x/i80-200g-sfpdd+12-400g-qsfpdd-1",
            "cpm-1x/i80-200g-sfpdd+12-800g-qsfpdd-1x",
            "cpm-2s",
            "cpm-2se",
            "cpm-a",
            "cpm-e",
            "cpm-ixr",
            "cpm-ixr-e",
            "cpm-ixr-e-gnss",
            "cpm-ixr-e-gnss/imm14-10g-sfp++4-1g-tx",
            "cpm-ixr-e-gnss/imm24-sfp++8-sfp28+2-qsfp28",
            "cpm-ixr-e/imm14-10g-sfp++4-1g-tx",
            "cpm-ixr-e/imm24-sfp++8-sfp28+2-qsfp28",
            "cpm-ixr-e2",
            "cpm-ixr-e2c",
            "cpm-ixr-e2n/imm4-sfp+4-sfp+",
            "cpm-ixr-e3c/imm4-qsfp28+16-sfp28+8-sfp56",
            "cpm-ixr-e3x/imm16-sfp112+15-sfp56+6-qsfpdd",
            "cpm-ixr-ec",
            "cpm-ixr-r4",
            "cpm-ixr-s",
            "cpm-ixr-s/imm48-sfp++6-qsfp28",
            "cpm-ixr-x",
            "cpm-ixr-x/imm32-qsfp28+4-qsfpdd",
            "cpm-ixr-x/imm6-qsfpdd+48-sfp56",
            "cpm-s",
            "cpm-sar",
            "cpm-sar-hm",
            "cpm-sar-hmc",
            "cpm-v",
            "cpm-v/iom-v",
            "cpm-x20",
            "cpm2-s",
            "cpm2-x20",
            "cpm5",
            "iom-sar",
        ]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal[
        "cpiom-ixr-r6",
        "cpiom-ixr-r6d",
        "cpm-1",
        "cpm-1s",
        "cpm-1se",
        "cpm-1se/imm36-800g-qsfpdd",
        "cpm-1x",
        "cpm-1x/dms24-800g-qsfpdd-1",
        "cpm-1x/i24-800g-qsfpdd-1",
        "cpm-1x/i40-200g-sfpdd+6-800g-qsfpdd-1",
        "cpm-1x/i48-400g-qsfpdd-1",
        "cpm-1x/i48-800g-qsfpdd-1x",
        "cpm-1x/i80-200g-sfpdd+12-400g-qsfpdd-1",
        "cpm-1x/i80-200g-sfpdd+12-800g-qsfpdd-1x",
        "cpm-2s",
        "cpm-2se",
        "cpm-a",
        "cpm-e",
        "cpm-ixr",
        "cpm-ixr-e",
        "cpm-ixr-e-gnss",
        "cpm-ixr-e-gnss/imm14-10g-sfp++4-1g-tx",
        "cpm-ixr-e-gnss/imm24-sfp++8-sfp28+2-qsfp28",
        "cpm-ixr-e/imm14-10g-sfp++4-1g-tx",
        "cpm-ixr-e/imm24-sfp++8-sfp28+2-qsfp28",
        "cpm-ixr-e2",
        "cpm-ixr-e2c",
        "cpm-ixr-e2n/imm4-sfp+4-sfp+",
        "cpm-ixr-e3c/imm4-qsfp28+16-sfp28+8-sfp56",
        "cpm-ixr-e3x/imm16-sfp112+15-sfp56+6-qsfpdd",
        "cpm-ixr-ec",
        "cpm-ixr-r4",
        "cpm-ixr-s",
        "cpm-ixr-s/imm48-sfp++6-qsfp28",
        "cpm-ixr-x",
        "cpm-ixr-x/imm32-qsfp28+4-qsfpdd",
        "cpm-ixr-x/imm6-qsfpdd+48-sfp56",
        "cpm-s",
        "cpm-sar",
        "cpm-sar-hm",
        "cpm-sar-hmc",
        "cpm-v",
        "cpm-v/iom-v",
        "cpm-x20",
        "cpm2-s",
        "cpm2-x20",
        "cpm5",
        "iom-sar",
    ] = Field(..., description="CPM card types for SR OS")


class SrosMdaTypes(
    RootModel[
        Literal[
            "a32-chds1v2",
            "d24-800g-qsfpdd-1",
            "i1-wlan",
            "i2-cellular",
            "i2-sdi",
            "i3-10/100eth-tx",
            "i6-10/100eth-tx",
            "isa-aa-v",
            "isa-bb-v",
            "isa-ms-v",
            "isa-tunnel-v",
            "isa2-aa",
            "isa2-bb",
            "isa2-tunnel",
            "isa2-video",
            "m1-400g-qsfpdd+1-100g-qsfp28",
            "m10-10g-sfp+",
            "m10-1g-sfp+2-10g-sfp+",
            "m10-50g-sfp56",
            "m10-sfp++6-sfp",
            "m12-sfp28+2-qsfp28",
            "m14-10g-sfp++4-1g-tx",
            "m18-25g-sfp28",
            "m2-100g-qsfp28+16-10g-sfp+",
            "m2-cfp2",
            "m2-qsfpdd+2-qsfp28+24-sfp28",
            "m20-10g-sfp+",
            "m20-1g-csfp",
            "m20-v",
            "m24-800g-qsfpdd-1",
            "m24-sfp++8-sfp28+2-qsfp28",
            "m32-1g-csfp",
            "m32-qsfp28+4-qsfpdd",
            "m36-100g-qsfp28",
            "m36-qsfpdd",
            "m4-100g-cfp4",
            "m4-10g-sfp++1-100g-cfp2",
            "m4-1g-tx+20-1g-sfp+6-10g-sfp+",
            "m40-10g-sfp",
            "m40-10g-sfp-ptp",
            "m40-200g-sfpdd+6-800g-qsfpdd-1",
            "m46-10g-sfp+",
            "m48-400g-qsfpdd-1",
            "m48-800g-qsfpdd-1x",
            "m48-sfp++6-qsfp28",
            "m48-sfp+2-qsfp28",
            "m5-100g-qsfp28",
            "m5e2-100g-qsfp28+2-800g-qdd",
            "m5e8-100g-sfp112+2-800g-qdd",
            "m6-10g-sfp++1-100g-qsfp28",
            "m6-10g-sfp++4-25g-sfp28",
            "m6-qsfpdd+48-sfp56",
            "m80-1g-csfp",
            "m80-200g-sfpdd+12-400g-qsfpdd-1",
            "m80-200g-sfpdd+12-800g-qsfpdd-1x",
            "ma2-10gb-sfp+12-1gb-sfp",
            "ma20-1gb-tx",
            "ma4-10gb-sfp+",
            "ma44-1gb-csfp",
            "maxp1-100gb-cfp",
            "maxp1-100gb-cfp2",
            "maxp1-100gb-cfp4",
            "maxp10-10/1gb-msec-sfp+",
            "maxp10-10gb-sfp+",
            "me-isa2-ms",
            "me-isa2-ms-e",
            "me1-100gb-cfp2",
            "me10-10gb-sfp+",
            "me12-10/1gb-sfp+",
            "me12-100gb-qsfp28",
            "me16-25gb-sfp28+2-100gb-qsfp-b",
            "me16-25gb-sfp28+2-100gb-qsfp28",
            "me2-100gb-ms-qsfp28",
            "me2-100gb-qsfp28",
            "me3-200gb-cfp2-dco",
            "me3-400gb-qsfpdd",
            "me40-1gb-csfp",
            "me6-100gb-qsfp28",
            "me6-10gb-sfp+",
            "me6-400gb-qsfpdd",
            "me8-10/25gb-sfp28",
            "ms36-800g-qsfpdd",
            "p-isa2-ms",
            "p-isa2-ms-e",
            "p1-100g-cfp",
            "p10-10g-sfp",
            "p20-1gb-sfp",
            "p6-10g-sfp",
            "s18-100gb-qsfp28",
            "s36-100gb-qsfp28",
            "s36-100gb-qsfp28-3.6t",
            "s36-400gb-qsfpdd",
            "x12-400g-qsfpdd",
            "x2-s36-800g-qsfpdd-12.0t",
            "x2-s36-800g-qsfpdd-18.0t",
            "x24-100g-qsfp28",
            "x4-100g-cfp2",
            "x40-10g-sfp",
            "x40-10g-sfp-ptp",
            "x6-200g-cfp2-dco",
            "x6-400g-cfp8",
        ]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal[
        "a32-chds1v2",
        "d24-800g-qsfpdd-1",
        "i1-wlan",
        "i2-cellular",
        "i2-sdi",
        "i3-10/100eth-tx",
        "i6-10/100eth-tx",
        "isa-aa-v",
        "isa-bb-v",
        "isa-ms-v",
        "isa-tunnel-v",
        "isa2-aa",
        "isa2-bb",
        "isa2-tunnel",
        "isa2-video",
        "m1-400g-qsfpdd+1-100g-qsfp28",
        "m10-10g-sfp+",
        "m10-1g-sfp+2-10g-sfp+",
        "m10-50g-sfp56",
        "m10-sfp++6-sfp",
        "m12-sfp28+2-qsfp28",
        "m14-10g-sfp++4-1g-tx",
        "m18-25g-sfp28",
        "m2-100g-qsfp28+16-10g-sfp+",
        "m2-cfp2",
        "m2-qsfpdd+2-qsfp28+24-sfp28",
        "m20-10g-sfp+",
        "m20-1g-csfp",
        "m20-v",
        "m24-800g-qsfpdd-1",
        "m24-sfp++8-sfp28+2-qsfp28",
        "m32-1g-csfp",
        "m32-qsfp28+4-qsfpdd",
        "m36-100g-qsfp28",
        "m36-qsfpdd",
        "m4-100g-cfp4",
        "m4-10g-sfp++1-100g-cfp2",
        "m4-1g-tx+20-1g-sfp+6-10g-sfp+",
        "m40-10g-sfp",
        "m40-10g-sfp-ptp",
        "m40-200g-sfpdd+6-800g-qsfpdd-1",
        "m46-10g-sfp+",
        "m48-400g-qsfpdd-1",
        "m48-800g-qsfpdd-1x",
        "m48-sfp++6-qsfp28",
        "m48-sfp+2-qsfp28",
        "m5-100g-qsfp28",
        "m5e2-100g-qsfp28+2-800g-qdd",
        "m5e8-100g-sfp112+2-800g-qdd",
        "m6-10g-sfp++1-100g-qsfp28",
        "m6-10g-sfp++4-25g-sfp28",
        "m6-qsfpdd+48-sfp56",
        "m80-1g-csfp",
        "m80-200g-sfpdd+12-400g-qsfpdd-1",
        "m80-200g-sfpdd+12-800g-qsfpdd-1x",
        "ma2-10gb-sfp+12-1gb-sfp",
        "ma20-1gb-tx",
        "ma4-10gb-sfp+",
        "ma44-1gb-csfp",
        "maxp1-100gb-cfp",
        "maxp1-100gb-cfp2",
        "maxp1-100gb-cfp4",
        "maxp10-10/1gb-msec-sfp+",
        "maxp10-10gb-sfp+",
        "me-isa2-ms",
        "me-isa2-ms-e",
        "me1-100gb-cfp2",
        "me10-10gb-sfp+",
        "me12-10/1gb-sfp+",
        "me12-100gb-qsfp28",
        "me16-25gb-sfp28+2-100gb-qsfp-b",
        "me16-25gb-sfp28+2-100gb-qsfp28",
        "me2-100gb-ms-qsfp28",
        "me2-100gb-qsfp28",
        "me3-200gb-cfp2-dco",
        "me3-400gb-qsfpdd",
        "me40-1gb-csfp",
        "me6-100gb-qsfp28",
        "me6-10gb-sfp+",
        "me6-400gb-qsfpdd",
        "me8-10/25gb-sfp28",
        "ms36-800g-qsfpdd",
        "p-isa2-ms",
        "p-isa2-ms-e",
        "p1-100g-cfp",
        "p10-10g-sfp",
        "p20-1gb-sfp",
        "p6-10g-sfp",
        "s18-100gb-qsfp28",
        "s36-100gb-qsfp28",
        "s36-100gb-qsfp28-3.6t",
        "s36-400gb-qsfpdd",
        "x12-400g-qsfpdd",
        "x2-s36-800g-qsfpdd-12.0t",
        "x2-s36-800g-qsfpdd-18.0t",
        "x24-100g-qsfp28",
        "x4-100g-cfp2",
        "x40-10g-sfp",
        "x40-10g-sfp-ptp",
        "x6-200g-cfp2-dco",
        "x6-400g-cfp8",
    ] = Field(..., description="MDA types for SR OS")


class SrosXiomTypes(
    RootModel[
        Literal[
            "iom-s-1.5t",
            "iom-s-3.0t",
            "iom2-s-3.0t",
            "iom2-s-6.0t",
            "iom2-se-3.0t",
            "iom2-se-6.0t",
            "x2-s36-400g-qsfp112-3.0t",
            "x2-s36-800g-qsfpdd-6.0t",
        ]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal[
        "iom-s-1.5t",
        "iom-s-3.0t",
        "iom2-s-3.0t",
        "iom2-s-6.0t",
        "iom2-se-3.0t",
        "iom2-se-6.0t",
        "x2-s36-400g-qsfp112-3.0t",
        "x2-s36-800g-qsfpdd-6.0t",
    ] = Field(..., description="XIOM types for SR OS")


class SrosXiomMdaTypes(
    RootModel[
        Literal[
            "ms6-200gb-cfp2-dco",
            "ms3-200gb-cfp2-dco",
            "ms16-100gb-sfpdd+4-100gb-qsfp28",
            "ms18-100gb-qsfp28",
            "ms4-400gb-qsfpdd+4-100gb-qsfp28",
            "ms24-10/100gb-sfpdd",
            "ms2-400gb-qsfpdd+2-100gb-qsfp28",
            "ms8-100gb-sfpdd+2-100gb-qsfp28",
            "ms16-sdd+4-qsfp28-b",
            "ms8-sdd+2-qsfp28-b",
            "mse24-200g-sfpdd",
            "mse6-800g-cfp2-dco",
            "mse14-800g+4-400g",
            "m36-800g-qsfpdd",
            "mse6-800g-qsfpdd",
            "m36-400g-qsfp112",
            "ms2-100g-qsfp28+2-800g-qsfpdd",
            "ms8-100g-sfp112+2-800g-qsfpdd",
            "ms4-400g-qsfpdd+4-100g-qsfp28",
        ]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal[
        "ms6-200gb-cfp2-dco",
        "ms3-200gb-cfp2-dco",
        "ms16-100gb-sfpdd+4-100gb-qsfp28",
        "ms18-100gb-qsfp28",
        "ms4-400gb-qsfpdd+4-100gb-qsfp28",
        "ms24-10/100gb-sfpdd",
        "ms2-400gb-qsfpdd+2-100gb-qsfp28",
        "ms8-100gb-sfpdd+2-100gb-qsfp28",
        "ms16-sdd+4-qsfp28-b",
        "ms8-sdd+2-qsfp28-b",
        "mse24-200g-sfpdd",
        "mse6-800g-cfp2-dco",
        "mse14-800g+4-400g",
        "m36-800g-qsfpdd",
        "mse6-800g-qsfpdd",
        "m36-400g-qsfp112",
        "ms2-100g-qsfp28+2-800g-qsfpdd",
        "ms8-100g-sfp112+2-800g-qsfpdd",
        "ms4-400g-qsfpdd+4-100g-qsfp28",
    ] = Field(..., description="XIOM MDA types for SR OS")


class SrosSfmTypes(
    RootModel[
        Literal[
            "m-sfm5-12",
            "m-sfm5-12e",
            "m-sfm5-7",
            "m-sfm6-12e",
            "m-sfm6-7/12",
            "sfm-2s",
            "sfm-2se",
            "sfm-ixr-10",
            "sfm-ixr-6",
            "sfm-s",
            "sfm-x20",
            "sfm-x20-b",
            "sfm-x20s-b",
            "sfm2-s",
            "sfm2-x20s",
        ]
    ]
):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal[
        "m-sfm5-12",
        "m-sfm5-12e",
        "m-sfm5-7",
        "m-sfm6-12e",
        "m-sfm6-7/12",
        "sfm-2s",
        "sfm-2se",
        "sfm-ixr-10",
        "sfm-ixr-6",
        "sfm-s",
        "sfm-x20",
        "sfm-x20-b",
        "sfm-x20s-b",
        "sfm2-s",
        "sfm2-x20s",
    ] = Field(..., description="SFM types for SR OS")


class Mgmt(SchemaObject):
    """
    configuration container for management network
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    network: StrictStr | None = Field(None, description="management network name")
    bridge: StrictStr | None = Field(
        None,
        description="Set bridge to use for the management network (instead of the default generated bridge).",
    )
    ipv4_subnet: constr(pattern=r"(^.+/[0-9]{1,2}$)|(auto)", strict=True) | None = (
        Field(
            None,
            alias="ipv4-subnet",
            description="IPv4 subnet to use for the custom management network. e.g. 172.100.100.0/24",
        )
    )
    ipv6_subnet: constr(pattern=r"(^.+/[0-9]{1,3}$)|(auto)", strict=True) | None = (
        Field(
            None,
            alias="ipv6-subnet",
            description="IPv6 subnet to use for the custom management network. e.g. 3fff:172:100:100::/64",
        )
    )
    ipv4_gw: Ipv4Addr | None = Field(
        None,
        alias="ipv4-gw",
        description="IPv4 gateway address that will be set on a bridge used for the management network. Will be set to the first available IP address by default",
    )
    ipv6_gw: Ipv6Addr | None = Field(
        None,
        alias="ipv6-gw",
        description="IPv6 gateway address that will be set on a bridge used for the management network. Will be set to the first available IP address by default",
    )
    ipv4_range: constr(pattern=r"^.+/[0-9]{1,2}$", strict=True) | None = Field(
        None,
        alias="ipv4-range",
        description="IPv4 range out of the ipv4-subnet to use for the custom management network. e.g. 172.100.100.128/25",
    )
    ipv6_range: (
        constr(
            pattern=r"^((:|[0-9a-fA-F]{0,4}):)([0-9a-fA-F]{0,4}:){0,5}((([0-9a-fA-F]{0,4}:)?(:|[0-9a-fA-F]{0,4}))|(((25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])))(%[\p{N}\p{L}]+)?$",
            strict=True,
        )
        | None
    ) = Field(
        None,
        alias="ipv6-range",
        description="IPv6 range out of the ipv6-subnet to use for the custom management network. e.g. 3fff:172:100:100:8000::/65",
    )
    mtu: Mtu | None = Field(
        default_factory=lambda: Mtu.model_validate(1500),
        description="MTU for the custom network",
    )
    external_access: StrictBool | None = Field(
        None,
        alias="external-access",
        description="controls whether the management network has external access or not",
    )
    skip_when_unused: StrictBool | None = Field(
        None,
        alias="skip-when-unused",
        description="skip management network creation when every node has network-mode: none",
    )
    driver_opts: (
        dict[constr(pattern=r".+", strict=True), StrictStr | StrictFloat | StrictBool]
        | None
    ) = Field(
        None,
        alias="driver-opts",
        description="overrides for container runtime network driver options",
    )


class Settings(SchemaObject):
    """
    Global containerlab settings
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    certificate_authority: CertificateAuthorityConfig | None = Field(
        None, alias="certificate-authority"
    )


class MdaItem(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    slot: conint(ge=1, strict=True)
    type: SrosXiomMdaTypes


class XiomItem(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    slot: conint(ge=1, strict=True)
    type: SrosXiomTypes
    mda: list[MdaItem] | None = Field(
        None,
        description="Define list of MDAs under this XIOM. Each defined MDA must have values of 'slot' and 'type'",
    )


class MdaItem1(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    slot: conint(ge=1, strict=True)
    type: SrosMdaTypes = Field(..., description="Set MDA type (SR OS specific)")


class Component(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    slot: (
        constr(pattern=r"^[ABab]$", strict=True) | conint(ge=1, strict=True) | None
    ) = Field(
        None, description="Set component physical position on a distributed chassis"
    )
    type: Any | None = Field(None, description="Set component type")
    sfm: SrosSfmTypes | None = Field(None, description="Set SFM type (SR OS specific).")
    xiom: list[XiomItem] | None = Field(
        None,
        description="Define list of XIOMs (SR OS Specific). Each XIOM must have values of 'slot' and 'type'. MDAs must be defined under the XIOM.",
    )
    mda: list[MdaItem1] | None = Field(
        None,
        description="Define list of MDAs (SR OS Specific). Each defined MDA must have values of 'slot' and 'type'",
    )
    env: Env | None = None


class LinkConfigShort(SchemaObject):
    """
    link configuration container
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    endpoints: list = Field(..., description="endpoints list")
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    ipv4: list[Ipv4Item] | None = Field(
        None,
        description="Assign IPv4 per endpoint in brief links. Accepts ordered list of IPv4 prefixes as strings",
        max_length=2,
        min_length=1,
    )
    ipv6: list[Ipv6Item] | None = Field(
        None,
        description="Assign IPv6 per endpoint in brief links. Accepts ordered list of IPv6 prefixes as strings",
        max_length=2,
        min_length=1,
    )
    vars: LinkVars | None = None


class LinkEndpoint(SchemaObject):
    """
    Common link endpoint object for extended link configs
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    node: StrictStr = Field(..., description="")
    interface: StrictStr = Field(..., description="")
    mac: (
        constr(pattern=r"^(?:[0-9A-Fa-f]{2}[:-]){5}(?:[0-9A-Fa-f]{2})", strict=True)
        | None
    ) = Field(None, description="")
    ipv4: Ipv4Prefix | None = Field(
        None,
        description="IPv4 address (in CIDR notation) to configure on this interface",
    )
    ipv6: Ipv6Prefix | None = Field(
        None,
        description="IPv6 address (in CIDR notation) to configure on this interface",
    )
    vars: EndpointVars | None = None


class LinkVxlanRemote(RootModel[Ipv4Addr | Ipv6Addr]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Ipv4Addr | Ipv6Addr


class WaitForConfigItem(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    node: StrictStr | None = Field(None, description="node name to wait for")
    stage: StagesEnum | None = Field(None, description="phase to wait for")


class WaitForConfig(RootModel[list[WaitForConfigItem]]):
    """
    Dependency list for the node
    """

    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: list[WaitForConfigItem] = Field(
        ..., description="Dependency list for the node"
    )


class StageExec1(SchemaObject):
    """
    per-stage exec configuration
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    on_enter: StageExecList | None = Field(None, alias="on-enter")
    on_exit: StageExecList | None = Field(None, alias="on-exit")


class StageExec(RootModel[StageExec1 | list[StageExecItem]]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: StageExec1 | list[StageExecItem] = Field(
        ..., description="per-stage exec configuration"
    )


class LinkTypeVeth(SchemaObject):
    """
    Link definition to support the veth interfaces
    """

    model_config = ConfigDict(
        populate_by_name=True,
    )
    type: Literal["veth"]
    endpoints: list[LinkEndpoint] = Field(
        ..., description="Endpoints for the links", max_length=2, min_length=2
    )
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    ipv4: list[Ipv4Item] | None = Field(
        None,
        description="Assign IPv4 per endpoint. Accepts ordered list of IPv4 prefixes as strings",
        max_length=2,
        min_length=1,
    )
    ipv6: list[Ipv6Item] | None = Field(
        None,
        description="Assign IPv6 per endpoint. Accepts ordered list of IPv6 prefixes as strings",
        max_length=2,
        min_length=1,
    )
    vars: LinkVars | None = None
    labels: Labels | None = None


class LinkTypeVethStitched(SchemaObject):
    """
    Link definition that transparently stitches two veth endpoints through root-namespace veth ends joined by a tc redirect
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: Literal["veth-stitch"]
    endpoints: list[LinkEndpoint] = Field(
        ...,
        description="Endpoints for the link as extended objects",
        max_length=2,
        min_length=2,
    )
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    vars: LinkVars | None = None
    labels: Labels | None = None


class LinkTypeMgmtNet(SchemaObject):
    """
    Link definition for management network interfaces
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: Literal["mgmt-net"]
    endpoint: LinkEndpoint
    host_interface: LinkHostInterface = Field(..., alias="host-interface")
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    vars: LinkVars | None = None
    labels: Labels | None = None


class LinkTypeMacvlan(SchemaObject):
    """
    Link definition describing a macvlan link endpoint configuration
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: Literal["macvlan"]
    endpoint: LinkEndpoint
    host_interface: LinkHostInterface = Field(..., alias="host-interface")
    mode: LinkMacvlanMode | None = None
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    vars: LinkVars | None = None
    labels: Labels | None = None


class LinkTypeHost(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: Literal["host"]
    endpoint: LinkEndpoint
    host_interface: LinkHostInterface = Field(..., alias="host-interface")
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    vars: LinkVars | None = None
    labels: Labels | None = None


class LinkTypeVxlan(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: Literal["vxlan"]
    endpoint: LinkEndpoint
    remote: LinkVxlanRemote
    vni: LinkVxlanVni
    dst_port: LinkVxlanDstport = Field(..., alias="dst-port")
    src_port: LinkVxlanSrcport | None = Field(None, alias="src-port")
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    vars: LinkVars | None = None
    labels: Labels | None = None


class LinkTypeVxlanStitched(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: Literal["vxlan-stitch"]
    endpoint: LinkEndpoint
    remote: LinkVxlanRemote
    vni: LinkVxlanVni
    dst_port: LinkVxlanDstport = Field(..., alias="dst-port")
    src_port: LinkVxlanSrcport | None = Field(None, alias="src-port")
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    vars: LinkVars | None = None
    labels: Labels | None = None


class LinkTypeDummy(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: Literal["dummy"]
    endpoint: LinkEndpoint
    mtu: Mtu | None = Field(default_factory=lambda: Mtu.model_validate(1500))
    vars: LinkVars | None = None
    labels: Labels | None = None


class Create(SchemaObject):
    """
    create stage configuration
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    wait_for: WaitForConfig | None = Field(None, alias="wait-for")
    exec: StageExec | None = None


class CreateLinks(SchemaObject):
    """
    create stage configuration
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    wait_for: WaitForConfig | None = Field(None, alias="wait-for")
    exec: StageExec | None = None


class Configure(SchemaObject):
    """
    create stage configuration
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    wait_for: WaitForConfig | None = Field(None, alias="wait-for")
    exec: StageExec | None = None


class Healthy(SchemaObject):
    """
    create stage configuration
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    wait_for: WaitForConfig | None = Field(None, alias="wait-for")
    exec: StageExec | None = None


class Exit(SchemaObject):
    """
    create stage configuration
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    wait_for: WaitForConfig | None = Field(None, alias="wait-for")
    exec: StageExec | None = None


class StagesConfig(SchemaObject):
    """
    node's stages configurations
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    create: Create | None = Field(None, description="create stage configuration")
    create_links: CreateLinks | None = Field(
        None, alias="create-links", description="create stage configuration"
    )
    configure: Configure | None = Field(None, description="create stage configuration")
    healthy: Healthy | None = Field(None, description="create stage configuration")
    exit: Exit | None = Field(None, description="create stage configuration")


class NodeConfig(SchemaObject):
    """
    topology node configuration container
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    type: StrictStr | None = Field(None, description="type of a node")
    image: StrictStr | None = Field(
        None, description="container image to use for this node"
    )
    image_pull_policy: (
        Literal["always", "Always", "never", "Never", "ifnotpresent", "IfNotPresent"]
        | None
    ) = Field(
        None,
        alias="image-pull-policy",
        description="policy for pulling the referenced container image",
    )
    restart_policy: (
        Literal[
            "no",
            "No",
            "on-failure",
            "On-failure",
            "Always",
            "always",
            "unless-stopped",
            "Unless-stopped",
        ]
        | None
    ) = Field(
        None,
        alias="restart-policy",
        description="restart policy for the referenced container image",
    )
    kind: (
        Literal[
            "nokia_srlinux",
            "arista_ceos",
            "juniper_crpd",
            "juniper_csrx",
            "sonic-vs",
            "sonic-vm",
            "nokia_sros",
            "nokia_srsim",
            "juniper_vmx",
            "juniper_vqfx",
            "juniper_vsrx",
            "juniper_vjunosrouter",
            "juniper_vjunosswitch",
            "juniper_vjunosevolved",
            "cisco_xrv",
            "cisco_xrv9k",
            "arista_veos",
            "cisco_csr1000v",
            "paloalto_panos",
            "mikrotik_ros",
            "6wind_vsr",
            "cisco_n9kv",
            "cisco_ftdv",
            "dell_ftosv",
            "dell_sonic",
            "aruba_aoscx",
            "linux",
            "bridge",
            "ovs-bridge",
            "border0",
            "host",
            "keysight_ixia-c-one",
            "ipinfusion_ocnos",
            "checkpoint_cloudguard",
            "ciena_saos",
            "ext-container",
            "rare",
            "cisco_xrd",
            "cisco_xrd_vrouter",
            "cisco_c8000",
            "cisco_c8000v",
            "cisco_cat9kv",
            "cisco_iol",
            "cisco_asav",
            "cisco_vios",
            "nvidia_cumulusvx",
            "huawei_vrp",
            "openbsd",
            "freebsd",
            "generic_vm",
            "fortinet_fortigate",
            "k8s-kind",
            "fdio_vpp",
            "vyosnetworks_vyos",
            "juniper_cjunosevolved",
            "arrcus_arcos",
            "f5_bigip-ve",
            "cisco_sdwan",
            "openwrt",
            "spirent_stc",
            "veesix_osvbng",
            "ostinato",
            "plvision_sonic",
        ]
        | None
    ) = Field(None, description="kind of this node")
    license: StrictStr | None = Field(None, description="path to a license file")
    group: StrictStr | None = Field(
        None,
        description="grouping parameter of a node. A free form string that is mainly used in sorting elements when graphing",
    )
    startup_config: StrictStr | None = Field(
        None,
        alias="startup-config",
        description="path to a startup config file (if supported by the kind)",
    )
    startup_delay: StrictInt | None = Field(
        None,
        alias="startup-delay",
        description="Optional startup delay (seconds) to apply",
    )
    enforce_startup_config: StrictBool | None = Field(
        None,
        alias="enforce-startup-config",
        description="Set to `true` to make the node to boot with a startup-config even if the config file is present in the lab directory",
    )
    suppress_startup_config: StrictBool | None = Field(
        None,
        alias="suppress-startup-config",
        description="Set to `true` to prevent a startup-config file from being created (in a Zero-Touch Provisioning lab, for example)",
    )
    auto_remove: StrictBool | None = Field(
        None,
        alias="auto-remove",
        description="Set to `true` to remove the node automatically, instead of auto-restarting",
    )
    exec: list[StrictStr] | None = Field(
        None, description="list of commands to execute post deploy", min_length=1
    )
    binds: list[StrictStr] | None = Field(
        None, description="list of file/directory bindings", min_length=1
    )
    volumes: list[StrictStr] | None = Field(
        None, description="list of volumes to attach", min_length=1
    )
    ports: list[Port] | None = Field(
        None, description="list of port mappings", min_length=0
    )
    env: Env | None = None
    credentials: Credentials | None = Field(
        None,
        description="username and password for SSH/NETCONF/GNMI/etc. (overrides kind default)",
    )
    env_files: list[StrictStr] | None = Field(
        None,
        alias="env-files",
        description="list of external files containing environment variables",
        min_length=1,
    )
    user: constr(min_length=1, strict=True) | confloat(ge=0.0, strict=True) | None = (
        Field(None, description="user to use within the container")
    )
    hostname: constr(min_length=1, strict=True) | None = Field(
        None, description="hostname configured inside the container"
    )
    entrypoint: StrictStr | None = Field(None, description="container's entrypoint")
    cmd: StrictStr | None = Field(None, description="command to launch container with")
    labels: Labels | None = None
    runtime: Literal["docker", "podman"] | None = Field(
        None, description="Runtime used to launch the container node"
    )
    mgmt_ipv4: Ipv4Addr | None = Field(
        None,
        alias="mgmt-ipv4",
        description="IPv4 management address of the node (e.g. 172.10.10.11)",
    )
    mgmt_ipv6: Ipv6Addr | None = Field(
        None,
        alias="mgmt-ipv6",
        description="IPv6 management address of the node (e.g. 172.10.10.11)",
    )
    network_mode: (
        constr(pattern=r"^(host)|(container:\S+)|(none)$", strict=True) | None
    ) = Field(
        None,
        alias="network-mode",
        description="node network mode (can only be set host, defaults to bridge)",
    )
    link_apply_mode: Literal["live", "restart", "recreate"] | None = Field(
        None,
        alias="link-apply-mode",
        description="how `containerlab apply` handles dataplane link changes for this node: live (no lifecycle action), restart or recreate; overrides the kind default",
    )
    cpu: confloat(ge=0.0, strict=True) | None = Field(
        None, description="number of vcpu to allocate for this node/container"
    )
    memory: StrictStr | None = Field(
        None, description="memory limit for this node/container"
    )
    cpu_set: StrictStr | None = Field(
        None, alias="cpu-set", description="CPU cores to use by this node/container"
    )
    extras: ExtrasConfig | None = None
    config: ConfigConfig | None = None
    stages: StagesConfig | None = None
    dns: DnsConfig | None = None
    certificate: CertificateConfig | None = None
    healthcheck: HealthcheckConfig | None = None
    components: list[Component] | None = Field(
        None, description="List of node components, used for multicontainer systems"
    )
    aliases: list[StrictStr] | None = Field(
        None, description="list of additional network aliases for the node"
    )
    shm_size: (
        constr(
            pattern=r"^[0-9]+(\.?[0-9]*)?\s*([bB]|[kK][iI]?[bB]|[mM][iI]?[bB]|[gG][iI]?[bB])?$",
            strict=True,
        )
        | None
    ) = Field(
        None,
        alias="shm-size",
        description="shared memory size limit allocated to the container. Supported memory suffixes (case insensitive): b, kib, kb, mib, mb, gib, gb",
    )
    cap_add: list[StrictStr] | None = Field(
        None,
        alias="cap-add",
        description="list of capabilities to add to the container",
    )
    privileged: StrictBool | None = Field(
        None, description="run the container in privileged mode"
    )
    cgroupns_mode: Literal["host", "private"] | None = Field(
        None,
        alias="cgroupns-mode",
        description="cgroup namespace mode for the container",
    )
    cgroup_parent: StrictStr | None = Field(None, alias="cgroup-parent")
    pid_mode: StrictStr | None = Field(
        None, alias="pid-mode", description="PID namespace mode for the container"
    )
    tmpfs: dict[constr(pattern=r"^/.*", strict=True), StrictStr] | None = Field(
        None, description="tmpfs mounts to add to the container"
    )
    security_opts: list[StrictStr] | None = Field(
        None,
        alias="security-opts",
        description="security options to apply to the container runtime",
    )
    sysctls: (
        dict[constr(pattern=r".+", strict=True), StrictStr | StrictFloat] | None
    ) = Field(None, description="sysctl kernel parameters to set in the container")
    devices: list[StrictStr] | None = Field(
        None, description="list of host devices to add to the container"
    )


class Kinds(SchemaObject):
    """
    topology kinds configuration container
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    nokia_srlinux: NodeConfig | None = None
    nokia_srsim: NodeConfig | None = None
    nokia_sros: NodeConfig | None = None
    arista_ceos: NodeConfig | None = None
    vyosnetworks_vyos: NodeConfig | None = None
    juniper_crpd: NodeConfig | None = None
    juniper_csrx: NodeConfig | None = None
    sonic_vs: NodeConfig | None = Field(None, alias="sonic-vs")
    sonic_vm: NodeConfig | None = Field(None, alias="sonic-vm")
    dell_ftosv: NodeConfig | None = None
    dell_sonic: NodeConfig | None = None
    plvision_sonic: NodeConfig | None = None
    juniper_vmx: NodeConfig | None = None
    juniper_vsrx: NodeConfig | None = None
    juniper_vjunosrouter: NodeConfig | None = None
    juniper_vjunosswitch: NodeConfig | None = None
    juniper_vjunosevolved: NodeConfig | None = None
    cjunosevolved: NodeConfig | None = None
    juniper_cjunosevolved: NodeConfig | None = None
    aruba_aoscx: NodeConfig | None = None
    cisco_xrd: NodeConfig | None = None
    cisco_xrd_vrouter: NodeConfig | None = None
    cisco_xrv: NodeConfig | None = None
    cisco_xrv9k: NodeConfig | None = None
    cisco_nxos: NodeConfig | None = None
    cisco_n9kv: NodeConfig | None = None
    cisco_csr: NodeConfig | None = None
    cisco_cat9kv: NodeConfig | None = None
    cisco_ftdv: NodeConfig | None = None
    cisco_iol: NodeConfig | None = None
    cisco_c8000: NodeConfig | None = None
    cisco_c8000v: NodeConfig | None = None
    cisco_sdwan: NodeConfig | None = None
    cisco_asav: NodeConfig | None = None
    cisco_vios: NodeConfig | None = None
    linux: NodeConfig | None = None
    bridge: NodeConfig | None = None
    ovs_bridge: NodeConfig | None = Field(None, alias="ovs-bridge")
    host: NodeConfig | None = None
    ipinfusion_ocnos: NodeConfig | None = None
    keysight_ixia_c_one: NodeConfig | None = Field(None, alias="keysight_ixia-c-one")
    checkpoint_cloudguard: NodeConfig | None = None
    ciena_saos: NodeConfig | None = None
    ext_container: NodeConfig | None = Field(None, alias="ext-container")
    rare: NodeConfig | None = None
    nvidia_cumulusvx: NodeConfig | None = None
    openbsd: NodeConfig | None = None
    freebsd: NodeConfig | None = None
    openwrt: NodeConfig | None = None
    huawei_vrp: NodeConfig | None = None
    generic_vm: NodeConfig | None = None
    fdio_vpp: NodeConfig | None = None
    arrcus_arcos: NodeConfig | None = None
    mikrotik_ros: NodeConfig | None = None
    field_6wind_vsr: NodeConfig | None = Field(None, alias="6wind_vsr")
    spirent_stc: NodeConfig | None = None
    veesix_osvbng: NodeConfig | None = None
    ostinato: NodeConfig | None = None


class Topology(SchemaObject):
    """
    topology configuration container
    """

    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    nodes: dict[
        constr(pattern=r"^[a-zA-Z0-9][a-zA-Z0-9-._|]*$", strict=True), NodeConfig | None
    ] = Field(..., description="topology nodes configuration container")
    groups: dict[constr(pattern=r".*", strict=True), NodeConfig] | None = Field(
        None, description="topology groups configuration container"
    )
    kinds: Kinds | None = Field(
        None, description="topology kinds configuration container"
    )
    defaults: NodeConfig | None = None
    links: (
        list[
            LinkConfigShort
            | LinkTypeVeth
            | LinkTypeVethStitched
            | LinkTypeMgmtNet
            | LinkTypeMacvlan
            | LinkTypeHost
            | LinkTypeVxlan
            | LinkTypeVxlanStitched
            | LinkTypeDummy
        ]
        | None
    ) = Field(None, description="topology links section", min_length=1)


class Config(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    name: constr(pattern=r"^[a-zA-Z0-9][a-zA-Z0-9-._]*$", strict=True) = Field(
        ..., description="topology name"
    )
    prefix: (
        constr(pattern=r"^$|^__lab-name$|^[a-zA-Z0-9][a-zA-Z0-9-._]*$", strict=True)
        | None
    ) = Field(None, description="lab prefix")
    mgmt: Mgmt | None = Field(
        None, description="configuration container for management network"
    )
    topology: Topology = Field(..., description="topology configuration container")
    settings: Settings | None = Field(None, description="Global containerlab settings")
    debug: StrictBool | None = Field(
        None, description="Boolean flag to enable debug mode for the topology"
    )
