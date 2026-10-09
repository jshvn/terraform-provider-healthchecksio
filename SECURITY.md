# Security

## The guarantees of the provider

- The UUID of a check is its ping credential. The provider does not show it:
  - The `id` of a check is its unique_key, not its UUID. Terraform shows the `id` in each
    plan.
  - The `uuid` and `ping_url` attributes are sensitive.
  - An error from the API does not contain the request URL.
  - The debug log (`TF_LOG=DEBUG`) shows `***` in place of each UUID.
- The state contains the UUID and the ping URL. Keep the state in a private backend.
- The provider sends the API key only in the `X-Api-Key` header. It does not write the key
  to the state or to the log. If `api_url` uses http, the provider shows a warning.
- Each release has these files:
  - The archives and their SHA256SUMS file.
  - A detached signature of the SHA256SUMS file, made with an RSA key. Terraform and
    OpenTofu examine this signature before they install the provider.
  - A build provenance attestation for each archive. To examine it, run
    `gh attestation verify <archive> --repo jshvn/terraform-provider-healthchecksio`.
- The repository holds no credential. The release workflow gets the signing key from
  1Password, by name, and holds it only during the release job. The checks of a pull
  request get no secret.

## Reporting

Send a private vulnerability report for each of these problems:

- A method to get the UUID, the ping URL or the API key from the provider, other than
  from the state
- A method to change a release, or to make a release with no correct signature
- A vulnerability in the provider or in its workflows.

To send the report:

1. Open the Security tab of
   [jshvn/terraform-provider-healthchecksio](https://github.com/jshvn/terraform-provider-healthchecksio).
2. Select "Report a vulnerability".

Do not open an issue for the problem. All persons can read an issue. A problem in the
healthchecks.io service goes to [healthchecks.io](https://healthchecks.io/docs/).

## Supported versions

The supported version is the latest release of the latest major version.
