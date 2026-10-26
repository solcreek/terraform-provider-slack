---
page_title: "Publishing to the OpenTofu Registry"
subcategory: ""
description: |-
  Release and registration instructions for solcreek/slack.
---

# Publishing to the OpenTofu Registry

## Provider identity

| Setting | Value |
| --- | --- |
| Repository | `solcreek/terraform-provider-slack` |
| Registry address | `registry.opentofu.org/solcreek/slack` |
| Provider protocol | `6.0` |
| Initial release | `v0.1.0` |
| Signing fingerprint | `2DF6EA201129324B19FFCCD6CF721E16E07047D5` |

The signing fingerprint is already registered for the `solcreek` namespace. The release workflow checks this identity before signing. Do not replace the registered key when setting up the repository.

## Release setup

The repository must be public. Configure the `GPG_PRIVATE_KEY` Actions secret with the registered private signing key, and `PASSPHRASE` if the key is encrypted. Never commit private keys or passphrases. The release job receives its repository-scoped `GITHUB_TOKEN` from GitHub Actions.

Run `scripts/repo-settings.sh --apply` to enable immutable releases and prevent updates or deletion of `v*` tags. Running the script without arguments verifies those settings.

Before tagging, run:

```sh
make generate fmt-check build vet test test-terraform test-tofu license-check vulncheck
terraform fmt -check -recursive examples
go mod verify
goreleaser check
git diff --exit-code -- docs
```

Confirm the Tests workflow passes for the exact commit to tag. Ensure `CHANGELOG.md` contains the release's exact version heading, for example `## 0.1.0`, with release notes below it.

Create and push a new version tag only when ready to publish. The release workflow runs the tests again, builds the platform archives, includes dependency licenses, signs the checksums, and publishes the release. Published versions and their tags are immutable; fixes require a new version.

## Required release assets

For `v0.1.0`, expect:

- `terraform-provider-slack_0.1.0_<os>_<arch>.zip` for Linux, macOS, Windows, and FreeBSD on amd64 and arm64.
- A provider executable named `terraform-provider-slack_v0.1.0` inside each archive (`.exe` on Windows).
- `terraform-provider-slack_0.1.0_manifest.json`, declaring protocol 6.0.
- `terraform-provider-slack_0.1.0_SHA256SUMS` covering every platform archive and the manifest.
- `terraform-provider-slack_0.1.0_SHA256SUMS.sig`, a detached GPG signature of that checksum file.

Download the published assets to an empty directory and verify the signature with the registered public key before registration:

```sh
gpg --verify terraform-provider-slack_0.1.0_SHA256SUMS.sig terraform-provider-slack_0.1.0_SHA256SUMS
shasum -a 256 -c terraform-provider-slack_0.1.0_SHA256SUMS
```

## Initial registration

After the signed public release exists, open the [OpenTofu provider submission form](https://github.com/opentofu/registry/issues/new?template=provider.yml&title=Provider%3A%20solcreek%2Fslack&repository=solcreek%2Fterraform-provider-slack).

Use these fields:

- Title: `Provider: solcreek/slack`
- Provider Repository: `solcreek/terraform-provider-slack`

Submit through the GitHub issue form UI. The [registry instructions](https://github.com/opentofu/registry#adding-providers-modules-or-gpg-keys-to-the-opentofu-registry) explicitly prohibit creating registration issues with `gh`, the GitHub API, or other tooling because processing depends on the structured issue form. No additional signing-key submission is needed while using the registered fingerprint above.

Registration and indexing are not complete merely because a GitHub release exists. Wait for the registry submission to be accepted, then verify `https://registry.opentofu.org/v1/providers/solcreek/slack/versions` lists the release and that a clean `tofu init` downloads it without a development override. Update README publication status only after that succeeds.

## Verification boundary

Local and CI tests use API fixtures and do not prove live Slack API compatibility. Live verification requires explicit authorization for a dedicated development workspace. App installation and OAuth authorization remain outside this provider's scope.
