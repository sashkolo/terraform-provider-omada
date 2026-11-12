# Notes for coding agents

This is a public fork of `Tohaker/terraform-provider-omada`.
[CONTRIBUTING.md](CONTRIBUTING.md) describes the code structure and the test
requirements, and they apply here. One difference: pull requests for this
fork are opened against this repository's `main`, not against upstream.

## This repository is public

Everything here can be read by anyone and stays readable: code, tests,
examples, docs, commit messages, pull request titles and descriptions,
review comments. Deleting something later does not take it back.

- Never write a real controller address, site or device ID, MAC address, IP
  address or subnet, hostname, SSID, pre-shared key or token. Use values that
  are plainly invented: the documentation ranges `192.0.2.0/24`,
  `198.51.100.0/24` and `203.0.113.0/24`, MAC addresses from
  `00-00-5E-00-53-00` to `00-00-5E-00-53-FF`, IDs such as `site-id-example`.
- Never refer to a private repository, its issues or its pull requests, and
  never paste output from a live controller. Describe the behaviour instead.
- Test data comes from the in-process API stand-in in `internal/acctest`,
  not from a recording of a real controller.

## Working here

- Branch names start with `claude/` or `codex/`, and every commit carries the
  matching `Co-Authored-By` trailer.
- Before pushing: `make fmt`, `make lint`, `make testacc`, and
  `make generate` when a schema or an example changed.
- Pull requests are squash-merged. Releases are tags cut by the maintainer;
  an agent does not tag or publish.

## Review guidelines

Flag as **P0**:

- any real identifier, address or secret of the kinds listed above, in any
  file or in the pull request itself;
- a reference to a private repository.

Flag as **P1**:

- a resource whose `Read` does not reflect what the controller holds, so
  that a later plan shows no change where there is one;
- an `Update` or `Delete` that can change or remove something outside the
  resource's own object. This provider manages networks, and a wrong call
  can cut one off;
- a schema change that forces existing resources to be replaced, or a
  breaking change without a `CHANGELOG.md` entry;
- a change without the test CONTRIBUTING.md asks for: an acceptance test for
  a new resource or attribute, a regression test for a bug fix.

Don't flag formatting that `gofmt` and the linters own.
