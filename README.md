# Terraform Provider Omada (extended fork)

A Terraform and OpenTofu provider for the
[TP-Link Omada Software Controller](https://www.tp-link.com/omada-sdn/), built on
the controller's Open API.

This is a fork of
[`Tohaker/terraform-provider-omada`](https://github.com/Tohaker/terraform-provider-omada),
which manages sites. The fork adds most of what a small network needs: LANs and
VLANs, wireless, gateway ACLs, switch ports, DHCP reservations and port
forwarding.

## Resources

| Resource | Manages |
|---|---|
| `omada_site` | Sites (from upstream) |
| `omada_lan_network` | LAN networks: VLAN, gateway subnet, DHCP |
| `omada_wlan_group` | WLAN groups |
| `omada_ssid` | SSIDs in a WLAN group, including the VLAN they tag and the PSK |
| `omada_ap_wlan_group` | Which WLAN group each access point broadcasts |
| `omada_acl` | Gateway ACL rules |
| `omada_gateway_acl_order` | The order the gateway evaluates its ACLs (allow before deny) |
| `omada_ip_group` | IP groups that ACLs reference |
| `omada_ip_port_group` | IP-port groups that ACLs reference |
| `omada_switch_port_profile` | Switch port profiles: native and tagged networks, PoE, STP, loopback detection |
| `omada_switch_port` | The profile assigned to a switch port |
| `omada_dhcp_reservation` | Fixed-IP DHCP reservations, keyed by MAC |
| `omada_port_forwarding` | Port forwarding rules |
| `omada_firewall_setting` | Site firewall settings (one per site) |
| `omada_attack_defense_setting` | Site attack-defence settings (one per site) |

Data source: `omada_sites`.

Every resource supports import, so you can adopt a controller that is already
configured and begin from a plan with no changes. Each page in [`docs/`](docs/)
gives the import ID format.

## Behaviour worth knowing

- **Updates keep settings you don't manage.** A setting left out of your config
  keeps its live value on update; it is not reset to a default.
- **Switch port writes are guarded.** `omada_switch_port` refuses a port the
  switch doesn't have, a link-aggregation member, or moving a port off a trunk
  profile (where uplinks and AP ports usually sit) unless you allow it
  explicitly. A wrong port number shouldn't be able to cut off a switch.
- **A profile in use isn't deleted.** `omada_switch_port_profile` refuses to
  delete a profile that any switch port still uses. Destroying an
  `omada_switch_port` doesn't change the port, so move the port to another
  profile first, in an earlier apply or with `create_before_destroy` on the
  profile.
- **State follows the controller.** An object deleted outside Terraform is
  dropped from state, and one that is still there is never dropped on a
  transient error.
- **Self-signed controllers can be verified.** Pin the controller's certificate
  with `tls_server_sha256`, or trust a CA with `ca_cert_pem`, instead of turning
  verification off with `tls_skip_verify`.
- **Long runs keep working.** The access token is renewed as it nears expiry,
  and each request has a timeout (`request_timeout`).

## Compatibility

This fork is developed and tested against a self-hosted Omada Software
Controller 6.2.10. Some resources were first built on 5.15.x.
Other controller versions and cloud-hosted controllers may work, but they are
not tested. The acceptance suite runs against both Terraform and OpenTofu.

## Installation

The fork is not published to a registry. Download a release from
[Releases](https://github.com/sashkolo/terraform-provider-omada/releases) and
check the zip against the release's `SHA256SUMS`. Then serve it from a
filesystem mirror under the upstream source address, so your configuration
works with either build:

```hcl
terraform {
  required_providers {
    omada = {
      source  = "registry.terraform.io/Tohaker/omada"
      version = "0.16.0"
    }
  }
}
```

Put the zip in the mirror's packed layout:

```text
<mirror>/registry.terraform.io/tohaker/omada/terraform-provider-omada_0.16.0_linux_amd64.zip
```

Then point the CLI configuration (`~/.terraformrc`, or `~/.tofurc` for OpenTofu)
at the mirror:

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/path/to/mirror"
    include = ["registry.terraform.io/tohaker/omada"]
  }
  direct {
    exclude = ["registry.terraform.io/tohaker/omada"]
  }
}
```

Without the mirror, `init` goes to the Terraform Registry, which only has
upstream's releases.

## Configuration

The provider authenticates with an Open API application (client credentials).
To create one, see the [provider documentation](docs/index.md). In short:

```hcl
provider "omada" {
  host              = "https://omada.example.com:8043"
  controller_id     = var.omada_controller_id # the Omada ID
  client_id         = var.omada_client_id
  client_secret     = var.omada_client_secret # or OMADA_CLIENT_SECRET
  tls_server_sha256 = var.omada_cert_sha256   # for a self-signed certificate
}
```

Most resource pages in [`docs/resources`](docs/resources/) list the Open API
permissions the application needs.

## Documentation

- Provider and resource reference: [`docs/`](docs/)
- Runnable examples: [`examples/`](examples/)
- Changes by release: [`CHANGELOG.md`](CHANGELOG.md)

## Contributing

Issues and pull requests are welcome here. [CONTRIBUTING.md](CONTRIBUTING.md)
explains how to build the provider, run it locally and run the tests
(`make test`, and `make testacc` for the acceptance suite, which runs against a
fake controller). Changes that would help upstream users as well are worth
offering to `Tohaker/terraform-provider-omada` too.

### Maintaining this fork

- Releases are unsigned on purpose. Consumers pin the digest of each release's
  `SHA256SUMS`, and that pin is what they trust. Publishing to a registry would
  require signing, which is disabled in `.goreleaser.yml`.
- Forks start with Dependabot version updates turned off. Turn them on (and
  Dependabot alerts and security updates) in the repository settings; the
  configuration is `.github/dependabot.yml`.
- Protect `main` with a ruleset that requires a pull request and the `Build`,
  `govulncheck` and acceptance-test checks, and blocks force pushes and
  deletion.

## License

[MIT](LICENSE), as upstream.
