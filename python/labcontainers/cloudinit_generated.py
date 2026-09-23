# Generated from canonical/cloud-init 25.2 (e682bef5be7f18d6c803fbc3ad35e277fdf03e39).
# DO NOT EDIT. Regenerate with: make generate-cloud-init
# Upstream license: GPL-3.0 or Apache-2.0; see CLOUD-INIT-LICENSE.

from __future__ import annotations

from typing import Literal

from pydantic import ConfigDict, Field, RootModel, StrictBool, StrictInt, StrictStr

from labcontainers._schema import SchemaObject


class Renderer(RootModel[Literal["networkd", "NetworkManager"]]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: Literal["networkd", "NetworkManager"] = Field(
        ...,
        description="Use the given networking backend for this definition.  Default is networkd.",
    )


class DhcpOverrides(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    hostname: StrictStr | None = Field(
        None,
        description="Unsupported  for dhcp6-overrides when used with the networkd renderer.",
    )
    route_metric: StrictInt | None = Field(
        None,
        alias="route-metric",
        description="Unsupported  for dhcp6-overrides when used with the networkd renderer.",
    )
    send_hostname: StrictBool | None = Field(
        None,
        alias="send-hostname",
        description="Unsupported  for dhcp6-overrides when used with the networkd renderer.",
    )
    use_dns: StrictBool | None = Field(None, alias="use-dns")
    use_domains: StrictStr | None = Field(None, alias="use-domains")
    use_hostname: StrictBool | None = Field(None, alias="use-hostname")
    use_mtu: StrictBool | None = Field(
        None,
        alias="use-mtu",
        description="Unsupported  for dhcp6-overrides when used with the networkd renderer.",
    )
    use_ntp: StrictBool | None = Field(None, alias="use-ntp")
    use_routes: StrictBool | None = Field(
        None,
        alias="use-routes",
        description="Unsupported  for dhcp6-overrides when used with the networkd renderer.",
    )


class Gateway(RootModel[StrictStr]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: StrictStr = Field(
        ...,
        description="Deprecated, see Netplan#default-routes. Set default gateway for IPv4/6, for manual address configuration. This requires setting addresses too. Gateway IPs must be in a form recognised by inet_pton(3).",
    )


class Nameservers(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    search: list[StrictStr] | None = None
    addresses: list[StrictStr] | None = None


class Route(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    to: StrictStr
    via: StrictStr | None = None
    metric: StrictInt | None = None


class Mapping(SchemaObject):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    renderer: Renderer | None = None
    dhcp4: Literal["yes", "no", True, False] | None = Field(
        None, description="Enable DHCP for IPv4. Off by default."
    )
    dhcp6: Literal["yes", "no", True, False] | None = Field(
        None, description="Enable DHCP for IPv6. Off by default."
    )
    dhcp4_overrides: DhcpOverrides | None = Field(None, alias="dhcp4-overrides")
    dhcp6_overrides: DhcpOverrides | None = Field(None, alias="dhcp6-overrides")
    addresses: list[StrictStr] | None = Field(
        None,
        description="Add static addresses to the interface in addition to the ones received through DHCP or RA. Each sequence entry is in CIDR notation, i.e., of the form addr/prefixlen. addr is an IPv4 or IPv6 address as recognised by inet_pton(3) and prefixlen the number of bits of the subnet.",
    )
    gateway4: Gateway | None = None
    gateway6: Gateway | None = None
    optional: StrictBool | None = Field(
        None, description="Not required for online, false by default"
    )
    mtu: StrictInt | None = Field(
        None,
        description="The MTU key represents a device’s Maximum Transmission Unit, the largest size packet or frame, specified in octets (eight-bit bytes), that can be sent in a packet- or frame-based network. Specifying mtu is optional.",
    )
    nameservers: Nameservers | None = Field(
        None,
        description="Set DNS servers and search domains, for manual address configuration. There are two supported fields: addresses: is a list of IPv4 or IPv6 addresses similar to gateway*, and search: is a list of search domains.",
    )
    routes: list[Route] | None = Field(None, description="")


class Match(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    name: StrictStr | None = Field(
        None,
        description="Current interface name. Globs are supported, and the primary use case for matching on names, as selecting one fixed name can be more easily achieved with having no match: at all and just using the ID (see above). Note that currently only networkd supports globbing, NetworkManager does not.",
    )
    macaddress: StrictStr | None = Field(
        None,
        description="Device’s MAC address in the form xx:xx:xx:xx:xx:xx. Globs are not allowed. Letters must be lowercase.",
    )
    driver: StrictStr | None = Field(
        None,
        description="Kernel driver name, corresponding to the DRIVER udev property. Globs are supported. Matching on driver is only supported with networkd.",
    )


class MappingPhysical(Mapping):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    match: Match | None = Field(
        None,
        description="This selects a subset of available physical devices by various hardware properties. The following configuration will then apply to all matching devices, as soon as they appear. All specified properties must match.",
    )
    set_name: StrictStr | None = Field(
        None,
        alias="set-name",
        description="When matching on unique properties such as path or MAC, or with additional assumptions such as ''there will only ever be one wifi device'', match rules can be written so that they only match one device. Then this property can be used to give that device a more specific/desirable/nicer name than the default from udev’s ifnames. Any additional device that satisfies the match rules will then fail to get renamed and keep the original kernel name (and dmesg will show an error).",
    )
    wakeonlan: StrictBool | None = Field(
        None, description="Enable wake on LAN. Off by default."
    )


class Parameters(SchemaObject):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    mode: (
        Literal[
            "balance-rr",
            "active-backup",
            "balance-xor",
            "broadcast",
            "802.3ad",
            "balance-tlb",
            "balance-alb",
        ]
        | None
    ) = Field(
        None,
        description="Set the bonding mode used for the interfaces. The default is balance-rr (round robin).",
    )
    lacp_rate: Literal["fast", "slow"] | None = Field(
        None,
        alias="lacp-rate",
        description="Set the rate at which LACPDUs are transmitted. This is only useful in 802.3ad mode. Possible values are slow (30 seconds, default), and fast (every second).",
    )
    mii_monitor_interval: StrictStr | None = Field(
        None,
        alias="mii-monitor-interval",
        description="Specifies the interval for MII monitoring (verifying if an interface of the bond has carrier). The default is 0; which disables MII monitoring.",
    )
    min_links: StrictInt | None = Field(
        None,
        alias="min-links",
        description="The minimum number of links up in a bond to consider the bond interface to be up.",
    )
    transmit_hash_policy: (
        Literal["layer2", "layer3+4", "layer2+3", "encap2+3", "encap3+4"] | None
    ) = Field(
        None,
        alias="transmit-hash-policy",
        description="Specifies the transmit hash policy for the selection of slaves. This is only useful in balance-xor, 802.3ad and balance-tlb modes.",
    )
    ad_select: Literal["stable", "bandwidth", "count"] | None = Field(
        None,
        alias="ad-select",
        description="Set the aggregation selection mode. This option is only used in 802.3ad mode.",
    )
    all_slaves_active: StrictBool | None = Field(
        None,
        alias="all-slaves-active",
        description="If the bond should drop duplicate frames received on inactive ports, set this option to false. If they should be delivered, set this option to true. The default value is false, and is the desirable behaviour in most situations.",
    )
    arp_interval: StrictInt | None = Field(
        None,
        alias="arp-interval",
        description="Set the interval value for how frequently ARP link monitoring should happen. The default value is 0, which disables ARP monitoring.",
    )
    arp_ip_targets: list[StrictStr] | None = Field(
        None,
        alias="arp-ip-targets",
        description="IPs of other hosts on the link which should be sent ARP requests in order to validate that a slave is up. This option is only used when arp-interval is set to a value other than 0. At least one IP address must be given for ARP link monitoring to function. Only IPv4 addresses are supported. You can specify up to 16 IP addresses. The default value is an empty list.",
    )
    arp_validate: Literal["none", "active", "backup", "all"] | None = Field(
        None,
        alias="arp-validate",
        description="Configure how ARP replies are to be validated when using ARP link monitoring.",
    )
    arp_all_targets: Literal["any", "all"] | None = Field(
        None,
        alias="arp-all-targets",
        description="Specify whether to use any ARP IP target being up as sufficient for a slave to be considered up; or if all the targets must be up. This is only used for active-backup mode when arp-validate is enabled.",
    )
    up_delay: StrictInt | None = Field(
        None,
        alias="up-delay",
        description="Specify the delay before enabling a link once the link is physically up. The default value is 0.",
    )
    down_delay: StrictInt | None = Field(
        None,
        alias="down-delay",
        description="Specify the delay before enabling a link once the link has been lost. The default value is 0.",
    )
    fail_over_mac_policy: Literal["none", "active", "follow"] | None = Field(
        None,
        alias="fail-over-mac-policy",
        description="Set whether to set all slaves to the same MAC address when adding them to the bond, or how else the system should handle MAC addresses.",
    )
    gratuitous_arp: StrictInt | None = Field(
        None,
        alias="gratuitous-arp",
        description="Specify how many ARP packets to send after failover. Once a link is up on a new slave, a notification is sent and possibly repeated if this value is set to a number greater than 1. The default value is 1 and valid values are between 1 and 255. This only affects active-backup mode.",
    )
    packets_per_slave: StrictInt | None = Field(
        None,
        alias="packets-per-slave",
        description="In balance-rr mode, specifies the number of packets to transmit on a slave before switching to the next. When this value is set to 0, slaves are chosen at random. Allowable values are between 0 and 65535. The default value is 1. This setting is only used in balance-rr mode.",
    )
    primary_reselect_policy: Literal["always", "better", "failure"] | None = Field(
        None,
        alias="primary-reselect-policy",
        description="Set the reselection policy for the primary slave. On failure of the active slave, the system will use this policy to decide how the new active slave will be chosen and how recovery will be handled.",
    )
    learn_packet_interval: StrictStr | None = Field(
        None,
        alias="learn-packet-interval",
        description="Specify the interval between sending Learning packets to each slave. The value range is between 1 and 0x7fffffff. The default value is 1. This option only affects balance-tlb and balance-alb modes.",
    )


class MappingBond(Mapping):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    interfaces: list[StrictStr] | None = Field(
        None, description="All devices matching this ID list will be added to the bond."
    )
    parameters: Parameters | None = Field(
        None,
        description="Customisation parameters for special bonding options. Time values are specified in seconds unless otherwise specified.",
    )


class Parameters1(SchemaObject):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    ageing_time: StrictInt | None = Field(
        None,
        alias="ageing-time",
        description="Set the period of time to keep a MAC address in the forwarding database after a packet is received.",
    )
    priority: StrictInt | None = Field(
        None,
        description="Set the priority value for the bridge. This value should be a number between 0 and 65535. Lower values mean higher priority. The bridge with the higher priority will be elected as the root bridge.",
    )
    forward_delay: StrictInt | None = Field(
        None,
        alias="forward-delay",
        description="Specify the period of time the bridge will remain in Listening and Learning states before getting to the Forwarding state. This value should be set in seconds for the systemd backend, and in milliseconds for the NetworkManager backend.",
    )
    hello_time: StrictInt | None = Field(
        None,
        alias="hello-time",
        description="Specify the interval between two hello packets being sent out from the root and designated bridges. Hello packets communicate information about the network topology.",
    )
    max_age: StrictInt | None = Field(
        None,
        alias="max-age",
        description="Set the maximum age of a hello packet. If the last hello packet is older than that value, the bridge will attempt to become the root bridge.",
    )
    path_cost: StrictInt | None = Field(
        None,
        alias="path-cost",
        description="Set the cost of a path on the bridge. Faster interfaces should have a lower cost. This allows a finer control on the network topology so that the fastest paths are available whenever possible.",
    )
    stp: StrictBool | None = Field(
        None,
        description="Define whether the bridge should use Spanning Tree Protocol. The default value is “true”, which means that Spanning Tree should be used.",
    )


class MappingBridge(Mapping):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    interfaces: list[StrictStr] | None = Field(
        None,
        description="All devices matching this ID list will be added to the bridge.",
    )
    parameters: Parameters1 | None = Field(
        None,
        description="Customisation parameters for special bridging options. Time values are specified in seconds unless otherwise stated.",
    )


class MappingVlan(Mapping):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    id: StrictInt | None = Field(
        None, description="VLAN ID, a number between 0 and 4094."
    )
    link: StrictStr | None = Field(
        None,
        description="ID of the underlying device definition on which this VLAN gets created.",
    )


class NetworkConfigVersion2(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    version: Literal[2]
    renderer: Renderer | None = None
    ethernets: dict[str, MappingPhysical] | None = None
    bonds: dict[str, MappingBond] | None = None
    bridges: dict[str, MappingBridge] | None = None
    vlans: dict[str, MappingVlan] | None = None


class Model1(SchemaObject):
    model_config = ConfigDict(
        extra="forbid",
        populate_by_name=True,
    )
    network: NetworkConfigVersion2


class Model(RootModel[NetworkConfigVersion2 | Model1]):
    model_config = ConfigDict(
        populate_by_name=True,
    )
    root: NetworkConfigVersion2 | Model1
