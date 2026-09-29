## 0.16.1

FIXES:
- `omada_lan_network`: creating a network without `dhcp_settings` no longer
  fails with "inconsistent result after apply". The controller reports a
  disabled DHCP block for such a network; an unset `dhcp_settings` now stays
  unset while DHCP is off, and DHCP turned on outside Terraform still shows as
  drift. Found by the homelab #515 write proof.

## Unreleased

DOCS:
- Examples, docs and test fixtures use documentation values (placeholder IDs,
  `00-00-5E-00-53-xx` MACs, generic names) instead of values from a live
  network.
- The README describes the fork: its resources, behaviour, compatibility and
  how to install it from a release.

## 0.16.0

FIXES (switch-port guardrails):
- `omada_switch_port` checks the switch before writing and refuses:
  - a port the switch doesn't have (a typo in `port`);
  - a link-aggregation member, whose write would take it out of its LAG,
    unless `allow_lag_member = true`;
  - moving a port off a trunk profile (the "All" type, or any profile with
    tagged networks) unless `allow_trunk_reassign = true`. Uplinks and AP
    trunks sit on such profiles, and the Open API reports no uplink port, so
    the guard judges by what the port carries now.

## 0.15.0

FEATURES (client):
- `tls_server_sha256` (`OMADA_TLS_SERVER_SHA256`) pins the controller's
  certificate by its SHA-256 fingerprint, so a self-signed controller no longer
  needs `tls_skip_verify`. `ca_cert_pem` (`OMADA_CA_CERT_PEM`) trusts a given CA
  with normal verification.
- `request_timeout` (`OMADA_REQUEST_TIMEOUT`, default 60 s) bounds each
  request; a controller that stopped answering used to hang the run and hold
  the state lock.

FIXES (client):
- The access token is renewed before it expires, and once more when the
  controller rejects it (-44112/-44113 or HTTP 401), with the request retried.
  It used to be fetched once, so a run longer than its lifetime failed.
- A refused token request reports the controller's message, not only the HTTP
  status.


FIXES (update semantics):
- `omada_ssid`: settings left unset in config keep their live values on
  update. They were unknown on update and sent as hard-coded defaults, so a
  rename or PSK rotation switched 802.11r, MLO, hide-password and group-key
  rekey off. The update also sends the live `autoWanAccess` back.
  `device_type`, which the update endpoint can't change, now forces
  replacement instead of failing after apply; `vlan_id` is Optional+Computed.
- `omada_lan_network`: `igmp_snoop_enable` keeps its live value when unset (it
  was sent as false). Updates are read-modify-write: L2 relay, isolation, MLD
  snooping, all-LAN, application and DHCP next-server/options 60/66/138 are
  sent with their live values, and the update is refused, before anything is
  written, when DHCP guard, DHCPv6 guard, IPv6 or custom DHCP options are on.
  `vlan_id` changes in place instead of destroying and recreating the network.
  An unset `interface_ids` is no longer sent as `[]` (unbinding every port); an
  update that would drop a network's ports is refused.
- `omada_attack_defense_setting`: the optional limits and toggles keep their
  live value when unset, instead of failing the apply or planning a change to
  null.
- `omada_switch_port_profile`: creating a profile requires
  `spanning_tree_enable`, `loopback_detect_enable`, `port_isolation_enable` and
  `lldp_med_enable`; the SDK sends them as plain booleans, so unset ones created
  the profile with those protections off.


FIXES (state accuracy):
- Objects deleted outside Terraform are now dropped from state, so the plan
  shows them. `omada_acl`, `omada_lan_network`, `omada_ssid` and
  `omada_wlan_group` kept the prior state instead, so a deleted deny rule,
  VLAN or SSID planned as "No changes". Every Read re-checks a miss three times
  first, because the controller's lists are eventually consistent, and a
  failing list read is never taken as absence.
- Deletes are idempotent and never trust a bare error code: a controller error
  counts as "already gone" only when a confirmed re-list shows the object
  absent. `omada_ssid` and `omada_wlan_group` used to treat the generic -1001
  as success, dropping a live object from state when a delete was rejected.
- An object whose create succeeded but whose read-back failed is kept in state
  as tainted, so the next apply replaces it instead of orphaning it or failing
  on a duplicate. This applies to every resource with a create.
- Created ids are never lost: `omada_lan_network`, `omada_ssid`,
  `omada_wlan_group` and `omada_switch_port_profile` tested only `IsNull` on a
  Computed id (Unknown at create), so the name lookup never ran; the IP groups
  and the port profile saved a null id when the read-back missed the row.
- `omada_acl` rejects a null or unknown id in `source_ids`/`destination_ids`
  instead of silently dropping it and sending a narrower or empty list.
- Importing an id that doesn't exist now fails.
- Retry loops keep only the last attempt's diagnostics, so a transient error
  followed by success no longer fails the apply.
- `omada_switch_port` drops the port when its switch is gone (-39050) instead of
  failing every refresh; `omada_switch_port_profile` uses the documented
  not-found code -33507 (was -33517); `omada_port_forwarding` paging stops after
  100 pages.

FIXES:
- One shared envelope decoder (`internal/envelope`) replaces the 14 per-resource
  copies. A response without an `errorCode`, whatever its HTTP status, and an
  HTTP error status with `errorCode` 0 are now errors. Before, a failed DELETE
  answered with a JSON error page (no `errorCode`) counted as success in
  `acl`, `lannetwork`, `ssid`, `wlangroup`, `firewallsetting`,
  `attackdefensesetting`, `switchport`, `switchportprofile` and `apwlangroup`,
  so destroy dropped a live object from state.
- `omada_sites`: a controller error (for example an expired token) crashed the
  provider with a nil dereference; it is now reported as an error.

SECURITY:
- Bump every module to its current release, including `omada-go-sdk` 0.5.0 →
  0.6.0, and the `go` directive to 1.26.8.
  `govulncheck` on 0.14.0 reported 12 reachable vulnerabilities (grpc, x/net,
  x/text, and the Go 1.25.8 standard library the release was built with); it
  now reports none. grpc is held at 1.83.2 because 1.84.0 is affected by
  GO-2026-6443 and has no patched release.

CI:
- New `govulncheck` job (pinned), a pinned golangci-lint, and the acceptance
  suite also runs against OpenTofu as well as Terraform.

## 0.14.0

FEATURES:
- Added `omada_port_forwarding`, a full CRUD/import resource for explicit
  gateway TCP/UDP port and port-range mappings. It supports physical/virtual WAN
  and WAN-IP selectors plus optional source-address restrictions. DMZ is
  deliberately excluded so an all-ports exposure cannot be represented as an
  ordinary port-forwarding rule.

## 0.13.2

FIXES:
- `omada_ap_wlan_group`: add `wlanId` to the overview group-id key scan. Live
  capture on 6.2.10.18 showed `GET /aps/{apMac}` keys the bound group under
  `wlanId` (the same field the WLAN-group create returns), not any of the
  spellings v0.13.1 tried, so imported bindings still read back an empty
  `wlan_group_id`. With `wlanId` first in the candidate list a refresh now
  self-heals the state and the baseline plans to a no-op.

## 0.13.1

FIXES:
- `omada_ap_wlan_group`: read the current WLAN group id robustly. The SDK model
  `ApOverviewInfo` declares the group under the spaced key `"wlan group id"`, but
  live firmware (6.2.10.18) keys it differently, so `GET /aps/{apMac}` was read
  back with an empty `wlan_group_id` and an imported binding never planned to a
  no-op. The overview decode now scans candidate key spellings
  (`wlanGroupId`, `wlan group id`, …) and accepts only an id-shaped value (24-hex),
  so a group *name* is never mistaken for its id.

## 0.13.0

FEATURES:
- Added `omada_ap_wlan_group` resource: manages which single WLAN group a managed
  access point (EAP) broadcasts. The Open API models the AP↔WLAN-group
  relationship as 1:1 (every AP belongs to exactly one group) and exposes only a
  "switch this AP to a different group" write (`PATCH /aps/{apMac}/wlan-group`),
  never an unbind. The resource is therefore a singleton keyed by
  `(site_id, ap_mac)`: Create/Update switch the AP to the target group — skipping
  the call when the AP is already on it, which the controller rejects ("cannot be
  the current wlan group") — Read fetches the current group from the AP overview
  (`GET /aps/{apMac}`, decoding the literal `"wlan group id"` key), and Delete is
  a no-op (an AP cannot be unbound; moving it "back" is a `wlan_group_id` change).
  Import id is `<site_id>/<ap_mac>`. This lets Git assert and drift-detect AP↔group
  bindings while keeping staged WLAN groups unbroadcast until an AP is explicitly
  pointed at them.

## 0.12.0

FIXES:
- `omada_attack_defense_setting`: enable managed writes. The write path is now
  built by hand (a local modify body) and PATCHed through the SDK's configured
  transport, instead of the generated SDK model, so the nested IP-security-option
  toggle is sent under the controller's key `specifiedOption.securityEnable`. The
  SDK model hardcodes the codegen name `securityOptionEnable`, which the
  controller silently ignores — leaving `security_option_enable` in perpetual
  drift (planned, never applied, read back unchanged). The read side already
  compensated for the same key drift (v0.7.4); the write side now matches. On
  controller 5.15.x the write path was additionally blocked because GET omitted
  13 PATCH-required fields; on 6.2.10.18 GET and PATCH share the same schema, so
  a full-object write round-trips.

## 0.11.0

FEATURES:
- Added `omada_dhcp_reservation` resource: a MAC-keyed DHCP fixed-address
  reservation on a LAN network, with MAC normalization so out-of-band and
  imported entries compare cleanly. Full CRUD + import.

## 0.10.0

FEATURES:
- Added `omada_gateway_acl_order` resource: a per-site singleton that owns the
  deterministic, site-global evaluation order of gateway (OSG) ACL rules via
  `ModifyAclIndex` (type `gateway`). `ordered_acl_ids` must be exhaustive — every
  gateway ACL id, highest priority first — so an out-of-band or unlisted rule is
  rejected rather than silently reordered. Read reflects the live order (drift
  detection); Delete is a no-op (ordering is intrinsic); import is by `site_id`.
  This is the building block for allow-before-deny policy.

## 0.9.1

FIXES:
- `omada_ip_group` / `omada_ip_port_group`: preserve the configured top-level
  `description` on read. The controller stores it on create/modify but does not
  echo it in the per-type group list read, so the resource was refreshing it to
  null — tripping "Provider produced inconsistent result after apply" on create
  and drifting on every refresh. The description is now adopted from the API only
  when the read returns one, and is documented as not recoverable on bare import.
  Found by a live apply against a 6.2 controller.

## 0.9.0

FEATURES:
- Added `omada_ip_group` resource: a reusable, named set of IP hosts/subnets
  (Open API group profile, type 0) that gateway ACL rules reference by group id
  via `source_type`/`destination_type = 1` instead of hard-coding whole-VLAN
  networks. Full CRUD + import against the Open API v1 `profiles/groups` surface
 .
- Added `omada_ip_port_group` resource: a named set of IP hosts/subnets scoped
  to TCP/UDP ports (group profile type 1), referenced by ACLs via
  `source_type`/`destination_type = 2`. Supports both port-list (`port_type` 0)
  and port-mask (`port_type` 1) modes. Full CRUD + import.

## 0.8.0

FEATURES:
- Added `omada_switch_port_profile` resource: a reusable switch (OSW) LAN/port
  profile defining a port's VLAN membership (native/untagged plus tagged =
  access vs trunk). Full CRUD + import against the Open API v1 `lan-profiles`
  surface.
- Added `omada_switch_port` resource: the per-port assignment on a managed
  switch (which profile a port uses, plus name, PoE mode, and admin state).
  Modeled as a singleton keyed by (site, switch MAC, port) — Create/Update apply
  via the per-port Modify endpoint, Read selects from the switch overview
  portList, Delete is a no-op (a physical port cannot be removed).

## 0.2.0

FEATURES:
- Added `tls_skip_verify` option to provider config.

## 0.1.2

FEATURES:
- Updating documentation. No functional changes.

## 0.1.1

FIXES:
- Running only 1 task in parallel during release 

## 0.1.0

FEATURES:
- Added `site` resource
- Added `sites` data source
