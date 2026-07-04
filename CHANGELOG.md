## 0.9.0

FEATURES:
- Added `omada_ip_group` resource: a reusable, named set of IP hosts/subnets
  (Open API group profile, type 0) that gateway ACL rules reference by group id
  via `source_type`/`destination_type = 1` instead of hard-coding whole-VLAN
  networks. Full CRUD + import against the Open API v1 `profiles/groups` surface
  (homelab #155).
- Added `omada_ip_port_group` resource: a named set of IP hosts/subnets scoped
  to TCP/UDP ports (group profile type 1), referenced by ACLs via
  `source_type`/`destination_type = 2`. Supports both port-list (`port_type` 0)
  and port-mask (`port_type` 1) modes. Full CRUD + import (homelab #155).

## 0.8.0

FEATURES:
- Added `omada_switch_port_profile` resource: a reusable switch (OSW) LAN/port
  profile defining a port's VLAN membership (native/untagged plus tagged =
  access vs trunk). Full CRUD + import against the Open API v1 `lan-profiles`
  surface (homelab #153).
- Added `omada_switch_port` resource: the per-port assignment on a managed
  switch (which profile a port uses, plus name, PoE mode, and admin state).
  Modeled as a singleton keyed by (site, switch MAC, port) — Create/Update apply
  via the per-port Modify endpoint, Read selects from the switch overview
  portList, Delete is a no-op (a physical port cannot be removed) (homelab #153).

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
