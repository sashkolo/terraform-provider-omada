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
  `omada_acl` (v0.7.0, homelab #55),
  `omada_firewall_setting` + `omada_attack_defense_setting` (v0.7.3, homelab #56),
  `omada_switch_port_profile` + `omada_switch_port` (v0.8.0, homelab #153),
  `omada_ip_group` + `omada_ip_port_group` (v0.9.0, homelab #155),
  `omada_gateway_acl_order` (v0.10.0, homelab #156),
  `omada_dhcp_reservation` (v0.11.0, homelab #154),
  `omada_ap_wlan_group` (v0.13.0, homelab #157),
  `omada_port_forwarding` (v0.14.0, homelab #80/#199).
- **Hardening (v0.15.0, homelab #511):** one strict envelope decoder (#235),
  state accuracy (#514), updates that keep live settings (#515), current
  dependencies with govulncheck and OpenTofu in CI (#513). See `CHANGELOG.md`.
- **Decision + consumption model:** documented in the homelab repo at
  `docs/network/OMADA-TERRAFORM.md`.
- **Upstreaming:** changes here that are not homelab-specific should be offered
  back to upstream (MIT).

## Repository settings (operator)

Forks start with Dependabot version updates off, and this fork's dependencies
did not move between June and September 2026 as a result (homelab #513). These
are repository settings, applied by the operator, not by an agent:

- **Dependabot version updates:** Settings → Code security → Dependabot version
  updates → Enable (the config is `.github/dependabot.yml`). Also enable
  Dependabot alerts and security updates.
- **`main` ruleset:** require a pull request, require the `Build`, `govulncheck`
  and acceptance-test checks, and block force pushes and deletion.

Releases are unsigned on purpose: the homelab pins each release's `SHA256SUMS`
digest in `utilities/omada/omadactl`, which is the trust root.

Upstream `README.md` documents the provider itself; this file only records the
fork relationship.
