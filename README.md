# terraform-provider-healthchecksio

A Terraform and OpenTofu provider for the [healthchecks.io](https://healthchecks.io)
Management API v3. It manages checks, and it reads pings, flips, integrations, badges and
the server status. It operates with healthchecks.io and with a self-hosted server.

The documentation of each resource, data source, action and list resource is in
[`docs/`](docs/index.md), and on the registry page of the provider.

## Use the provider

```terraform
terraform {
  required_providers {
    healthchecksio = {
      source  = "jshvn/healthchecksio"
      version = "~> 1.0"
    }
  }
}

# The provider reads the API key from HEALTHCHECKSIO_API_KEY.
provider "healthchecksio" {}

data "healthchecksio_channels" "email" {
  kind = "email"
}

resource "healthchecksio_check" "backup" {
  name     = "Nightly backup"
  slug     = "nightly-backup"
  schedule = "0 3 * * *"
  tz       = "UTC"
  grace    = 3600
  channels = data.healthchecksio_channels.email.ids
}
```

The `id` of a check is its unique_key, not its UUID. The UUID is the ping credential. The
`uuid` and `ping_url` attributes are sensitive, but the state contains them. Thus, keep the
state in a private backend.

## Adopt the checks that exist

- With Terraform 1.14 or later, use the list resource:
  `terraform query -generate-config-out=checks.tf` writes an import block and a resource
  block for each check.
- With an older Terraform or with OpenTofu, write an import block with the unique_key of
  the check. Get the unique_key from the `healthchecksio_checks` data source.

## Move from kristofferahl/healthchecksio

I previously used an existing provider, `kristofferahl/healthchecksio`, before building
this. However, at the time it used API v1, exposed the UUID ping credential
in plans and logs, and had no data source for checks, pings, or flips, no pause and resume
actions, and no identity or keyword filters. I made this provider to fix some of those
gaps for myself, and later open sourced and published it in case anyone else might
benefit from it.

The resource has the same type name, but the schema and the id are different. Do not move
the state. The guide [Move from kristofferahl/healthchecksio](docs/guides/migrate-from-kristofferahl.md)
gives the procedure.

## Develop

Install a container engine (Apple `container` or Docker) and
[go-task](https://taskfile.dev). Each other tool is in the toolbox image. Run `task` to see
the menu.

| Task | What it does |
|---|---|
| `task check` | golangci-lint, the unit tests, terraform fmt, actionlint, and the docs check |
| `task acc` | The acceptance tests against a local healthchecks server, with terraform and with tofu |
| `task build` | The release archives for each platform, with no signature |
| `task docs` | Makes `docs/` from the schema, `templates/` and `examples/` |
| `task outdated` | Each pinned tool version and the latest release. Fails when one is old |

`CLAUDE.md` gives the design. `SECURITY.md` tells how to report a vulnerability.

## Release

1. Make sure that `task check`, `task acc` and `task build` pass.
2. Add the changes to `CHANGELOG.md`.
3. Push a tag `vX.Y.Z`. The release workflow signs the archives and makes the GitHub
   release.

The owner does these steps one time. No workflow does them:

- Make an RSA signing key, and put it in the 1Password item
  `terraform-provider-healthchecksio`, section `gpg`. Replace each placeholder value:
  1. `gpg --quick-gen-key "<name> <email>" rsa4096 sign never` makes the key. Use a
     passphrase.
  2. `fingerprint`: the fingerprint that `gpg --list-secret-keys` shows.
  3. `private_key_base64`: the output of
     `gpg --armor --export-secret-keys <fingerprint> | base64 | tr -d '\n'`.
  4. `passphrase`: the passphrase of the key.
  5. `public_key`: the output of `gpg --armor --export <fingerprint>`. Both registries
     need it.
- Add the `OP_SERVICE_ACCOUNT_TOKEN` secret to the repository.
- Turn on private vulnerability reporting in the settings of the repository
  (`SECURITY.md`).
- Terraform Registry: add the public key in **User Settings > Signing Keys**. Then use
  **Publish > Provider**.
- OpenTofu Registry: submit the "Submit new Provider" issue form and the "Submit new
  Provider Signing Key" issue form, by hand, in
  [opentofu/registry](https://github.com/opentofu/registry).

## License

MIT. See [LICENSE](LICENSE).
