# Terraform Provider for UpSnap

[![Tests](https://github.com/WindKube/terraform-provider-upsnap/actions/workflows/test.yml/badge.svg)](https://github.com/WindKube/terraform-provider-upsnap/actions/workflows/test.yml)

Manage [UpSnap](https://github.com/seriousm4x/UpSnap), the wake-on-LAN dashboard, with Terraform or OpenTofu. Built with the
[Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and tested against UpSnap 5.7.3.

```terraform
provider "upsnap" {
  endpoint = "http://upsnap.lan:8090"
  username = "admin@example.com"
  password = var.upsnap_password
}

resource "upsnap_group" "office" {
  name = "Office"
}

resource "upsnap_device" "workstation" {
  name      = "Workstation"
  ip        = "192.168.1.30"
  mac       = "aa:bb:cc:dd:ee:30"
  group_ids = [upsnap_group.office.id]
}

resource "upsnap_port" "rdp" {
  device_id = upsnap_device.workstation.id
  name      = "RDP"
  number    = 3389
}

action "upsnap_device_wake" "workstation" {
  config {
    device_id = upsnap_device.workstation.id
  }
}
```

## What it covers

| Terraform                                                                        | UpSnap / PocketBase                              |
| -------------------------------------------------------------------------------- | ------------------------------------------------ |
| `upsnap_device` resource, `upsnap_device` / `upsnap_devices` data sources         | `devices` collection                             |
| `upsnap_port` resource                                                           | `ports` collection, linked by `devices.ports`    |
| `upsnap_group` resource, `upsnap_group` / `upsnap_groups` data sources            | `device_groups` collection, `devices.groups`     |
| `upsnap_user` resource                                                           | `users` auth collection                          |
| `upsnap_permission` resource                                                     | `permissions` collection                         |
| `upsnap_settings` resource (singleton)                                           | `settings_private` and `settings_public`         |
| `upsnap_device_wake` / `_shutdown` / `_sleep` / `_reboot` actions                 | `GET /api/upsnap/{wake,shutdown,sleep,reboot}/{id}` |
| `upsnap_group_wake` / `_shutdown` actions                                        | `GET /api/upsnap/{wakegroup,shutdowngroup}/{id}` |

Not covered: the network scan (`/api/upsnap/scan` returns a one-off list that does not map onto a resource or a stable data source),
the settings favicon (a file upload), and PocketBase superusers themselves.

Documentation for every resource, data source and action, with import examples, is in [`docs/`](docs/) and on the Terraform Registry.

## Development

Requirements: Go (see `go.mod`), Terraform 1.14+ (actions) and Docker.

```shell
make build              # compile
make test               # unit tests (client, validators, schema)
make testacc-up         # start UpSnap 5.7.3 from docker-compose.yml and create the test superuser
make testacc            # acceptance tests against it (TF_ACC=1)
make testacc-down       # remove the container
make generate           # format examples and regenerate docs/
make lint               # golangci-lint
```

`UPSNAP_PORT` changes the local port (default 8090). UpSnap ignores CLI subcommands while `UPSNAP_HTTP_LISTEN` is set, so
`make testacc-up` clears it when running `upsnap superuser upsert`.

To try a local build with Terraform, add a [development override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers)
for `windkube/upsnap` pointing at `$(go env GOPATH)/bin` and run `make install`.

CI runs lint, docs drift and `tfplugindocs validate`, unit tests, and the acceptance suite against a real UpSnap container on
Terraform 1.13, 1.14 and latest, and OpenTofu latest (which skips resource identity and actions, as OpenTofu does not implement them).

## Releasing

Push a `v*` tag. The release workflow builds with GoReleaser, signs the checksums with the GPG key in the `GPG_PRIVATE_KEY` and
`PASSPHRASE` secrets, and publishes a GitHub release that the Terraform Registry picks up.

## License

[Apache 2.0](LICENSE)
