# Homelab fork

This is the [sashkolo/homelab](https://github.com/sashkolo/homelab) fork of
[`Tohaker/terraform-provider-omada`](https://github.com/Tohaker/terraform-provider-omada).

It is the home for Omada Open API resources authored for the homelab
(`omada_lan_network`, `omada_ssid`/`omada_wlan_group`, `omada_acl`, …) under epic
[homelab#49](https://github.com/sashkolo/homelab/issues/49). The provider is
consumed by the homelab's OpenTofu stack (`terraform/omada`) from a pinned,
checksum-verified filesystem mirror.

- **Fork base:** upstream `v0.2.0`.
- **Homelab resources:** `omada_lan_network` (v0.3.0, homelab #53),
  `omada_ssid` + `omada_wlan_group` (v0.4.0, homelab #54),
  `omada_acl` (v0.7.0, homelab #55).
- **Decision + consumption model:** documented in the homelab repo at
  `docs/network/OMADA-TERRAFORM.md`.
- **Upstreaming:** changes here that are not homelab-specific should be offered
  back to upstream (MIT).

Upstream `README.md` documents the provider itself; this file only records the
fork relationship.
