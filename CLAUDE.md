# terraform-provider-healthchecksio

This repository is a Terraform and OpenTofu provider for the healthchecks.io Management API
v3. The registry address is `jshvn/healthchecksio`. `README.md` is for users. This file is
the design.

## Rules

- Writing: obey ASD-STE100 (Simplified Technical English, Issue 9) in each Markdown file,
  each code comment, each schema description and each example comment. Use the checklist
  in the
  [Writing section of the katoptra CONTRIBUTING](https://github.com/katoptra/.github/blob/main/CONTRIBUTING.md#writing).
  Keep the technical nouns of Terraform and healthchecks.io: provider, resource, data
  source, action, list resource, identity, plan, state, apply, import, registry, release,
  check, ping, grace, schedule, slug, tag, integration, badge, flip, toolbox, engine. Error
  strings and log lines are output strings: the standard is not applicable to them.
- Do not submit, register or publish to an outside party. The owner does each submission:
  the Terraform Registry, the OpenTofu Registry, nixpkgs and any other index. In a plan or
  a checklist, write each submission as one line, as a step for the owner.
- Each tool operates in the toolbox image (`Dockerfile`). A laptop or a CI runner supplies
  only a container engine, go-task and jq (for the menu check). Bare `task` shows the menu.
- Before a tag: `task check`, `task acc` and `task build` must pass, and `docs/` must agree
  with the schema (`task docs`).
- Pin each action to a commit, with the version in a comment. Dependabot updates the pins.
  The versions in the `Dockerfile` ARGs and of go-task are pinned by hand. `task outdated`
  compares them with the latest releases, and the weekly `outdated` workflow fails when
  one is old.

## The files

- `internal/client/`: the API client. It has a function for each endpoint of the API v3.
  It uses only the Go standard library. `client_test.go` tests it with `httptest`.
- `internal/provider/`: the provider (terraform-plugin-framework, protocol 6).
  - `provider.go`: the provider schema, the configuration and the lists of types.
  - `check.go`: `checkFields`, the attributes that the resource, the list resource and the
    data sources share, the conversion to and from the API, and the validators.
  - `check_resource.go`: `healthchecksio_check`. `check_list.go`: its list resource.
  - `check_data_sources.go`, `check_history.go`, `data_sources.go`: the data sources.
  - `actions.go`: `healthchecksio_pause` and `healthchecksio_resume`.
  - `provider_test.go`: the acceptance tests, and a unit test of the UUID mask in the
    log. `task acc` runs the acceptance tests against a local healthchecks server, with
    terraform and then with tofu. The test framework serves all provider blocks of a
    configuration from one provider process, and the last configuration applies to all
    blocks. Thus, a test cannot use two API keys in one configuration.
    `check_resource_test.go` and `data_sources_test.go`: unit tests with `httptest` of the
    uuid guard, of a create with a tz that the server changes, and of the status data
    source on a 5xx.
- `examples/` and `templates/`: the input of tfplugindocs, also the guides in
  `templates/guides/`. `docs/`: its output, which the registries show. Do not edit `docs/`
  by hand.
- `.goreleaser.yml`, `terraform-registry-manifest.json`: the release archives.
- `.golangci.yml`: the linters of `task check`.
- `.github/workflows/test.yml`: the checks. `.github/workflows/release.yml`: the release
  after a tag push, in two jobs. The test job has no write permission and no secret. The
  release job signs, publishes and makes the provenance attestation.
  `.github/workflows/outdated.yml`: the weekly version check.
- `SECURITY.md`: what the provider keeps secret, and how to report a vulnerability.

## Must knows

- **The id is the unique_key, not the UUID.** The UUID is the ping credential, and
  Terraform shows the id in each plan. The unique_key is the SHA-1 of the first 16 hex
  digits of the UUID (`Check.unique_key` in upstream `hc/api/models.py`). The read-only
  API also uses it. `uuid` and `ping_url` are sensitive. The state contains them. Import
  accepts a unique_key or a UUID. Thus, an import block with a unique_key can stay in git.
- **Each update sends each field.** An update in the API changes only the fields in the
  body. If the provider sends only the fields that it knows, the removal of a value from
  the configuration does not clear it on the server, and each plan shows a change.
  `checkFields.spec` sends each field, also an empty value.
- **channels is the one exception.** A null `channels` is not in the body. Thus, the
  provider does not change the integrations of the check. An empty set removes all
  integrations.
- **timeout and schedule are exclusive.** `ModifyPlan` sets the other kind to null, the
  default timeout (86400) when a simple check has none, and the default tz (UTC) when a
  check with a schedule has none. The API keeps the schedule when it gets both.
- **A create uses a new connection.** Some servers (uWSGI http-socket, as in the
  healthchecks image) send HTTP/1.1 with no "Connection: close", and then close the
  connection. Go can send the next request on that closed connection. Go sends the request
  again only if it is idempotent. The client marks each request as idempotent, except a
  create. A create uses a client with no reuse of connections. Thus, a create cannot make
  two checks.
- **Retries.** The client tries again after HTTP 429, and after a 5xx or a network error
  except for a create. It obeys Retry-After. The API accepts a maximum of 100 requests in
  one minute.
- **No UUID in an error or a log.** An `APIError` has no path, and a network error has no
  URL: Terraform shows each error. The provider writes the log of the client with `tflog`
  (`TF_LOG=DEBUG`), and masks each UUID in each field with `***`.
- **A value that the server changes is an error before the plan.** After an apply, the
  state must agree with the plan. Thus, the provider rejects each value that the server
  stores in a different form:
  - A `channels` UUID with uppercase letters. The server writes lowercase.
  - A UUID in the `id` of the `healthchecksio_check` data source. The data source gives the
    unique_key.
- **A legacy tz is an error after the apply.** The server stores the canonical name (for
  example `Europe/Kyiv` for `Europe/Kiev`), and no API gives it before the apply. Thus,
  `tzDiag` compares the tz of the server with the plan after a create or an update. Create
  deletes the new check, Update keeps the state, and the error gives the name to use.
- **Only API v3.** The client does not accept an `api_url` for v1 or v2. These versions
  make the slug from the name.
- **Terraform-only features.** OpenTofu does not support list resources, actions or
  deferred actions. The provider offers them, and OpenTofu ignores them. `task acc` skips
  their tests with tofu.

## Feature support

| Feature | Terraform | OpenTofu | Here |
|---|---|---|---|
| Protocol 6 | 1.0 | 1.6.0 | The provider |
| Resource identity | 1.12 | 1.12.0 | `unique_key` |
| List resources (`terraform query`) | 1.14 | Not supported | `healthchecksio_check` |
| Actions | 1.14 | Not supported | pause, resume |
| Deferred actions | Experiment | Not supported | `Configure` defers when an `api_key` is unknown |
| Ephemeral resources, write-only attributes | 1.10, 1.11 | 1.11.0 | Not used: no API value is temporary |
| Provider-defined functions | 1.8 | 1.7.0 | Not used |

## Why Go and the plugin framework

The plugin framework is the HashiCorp recommendation for a new provider. SDKv2 gets almost
no new features. The framework is the only library with resource identity, list resources
and actions. The protocol is gRPC, and a provider in a different language is possible. But
no other library has the same support for the protocol, the tests and the docs. The code
generators (tfplugingen-openapi, tfplugingen-framework) are a tech preview with no release
since 2024, and the healthchecks.io API has no OpenAPI document. Thus, the schema is
written by hand.

## Platforms

`.goreleaser.yml` builds 18 archives with no cgo, from one toolbox image:

- darwin: amd64, arm64
- linux: 386, amd64, arm (v6), arm64, s390x
- windows: 386, amd64, arm64
- freebsd: 386, amd64, arm, arm64
- openbsd: 386, amd64, arm, arm64

This list contains each platform that the Terraform Registry recommends (darwin amd64 and
arm64, linux amd64, arm64, armv6 and s390x, windows amd64), and its lower-priority
platforms. HCP Terraform needs linux amd64. The OpenTofu Registry reads darwin, freebsd,
linux, openbsd, solaris and windows, on 386, amd64, arm and arm64.

## The registries

Both registries read the same GitHub release. The release must contain:

- `terraform-provider-healthchecksio_{VERSION}_{OS}_{ARCH}.zip`, with the binary
  `terraform-provider-healthchecksio_v{VERSION}`
- `..._{VERSION}_SHA256SUMS`, and `..._SHA256SUMS.sig`: a binary (not armored) detached
  signature
- `..._{VERSION}_manifest.json` with protocol 6.0. Without it, the OpenTofu Registry uses
  protocol 5.0.

Constraints:

- The repository is public, with a lowercase name. The tag is `vX.Y.Z`. No branch has the
  name of a tag. A release is not a draft.
- The signing key is RSA. The Terraform Registry does not accept an ECC key. The OpenTofu
  Registry accepts RSA. A key in the Terraform Registry cannot be removed.
- The OpenTofu Registry shows the docs only for a license on its allowlist. MIT is on it.
- Each registry shows the docs of a tag. A correction of the docs needs a new release.
- The Terraform Registry reads `docs/` (index, resources, data-sources, actions,
  list-resources). A document over 500 KB is cut.

The release workflow makes the GitHub release. No API can register a provider with the
Terraform Registry. The OpenTofu Registry closes a submission from `gh` or from the API.
Thus, the registration is the owner's step, one time for each registry. After that,
each registry gets each new release: the Terraform Registry from a webhook, the OpenTofu
Registry from a check every 15 minutes.

Other places: nixpkgs `terraform-providers` adds a provider from the Terraform Registry
(the owner's PR). OpenTofu 1.10 and later can also install from an OCI mirror. This
provider does not need one.

## Maintenance

- Dependabot opens weekly PRs for the Go modules, the actions and the base image.
- `task outdated` shows the versions in the `Dockerfile` ARGs, the healthchecks image and
  go-task. Change a version and the hash of its SHA256SUMS file together.
- When healthchecks.io changes the API, examine `templates/docs/api.md` and
  `hc/api/views.py` (`Spec`, `_update`) upstream. `client.Check`, `client.CheckSpec` and
  `checkFields` follow them.
- Tag a release with a semver version. A new attribute is a minor version. A removed
  attribute or a change of type is a major version, with a state upgrade
  (`ResourceWithUpgradeState`).
- The signing key: an RSA key in 1Password (a vault addressed by UUID, item
  `terraform-provider-healthchecksio`, section `gpg`: `private_key_base64`, `passphrase`,
  `public_key`, `fingerprint`). The private key is base64 on one line, because a password
  field can lose the line breaks of an armored key. The release workflow reads it with the
  `OP_SERVICE_ACCOUNT_TOKEN` secret.
