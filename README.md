# Slack Provider

[![Tests](https://github.com/solcreek/terraform-provider-slack/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/solcreek/terraform-provider-slack/actions/workflows/test.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/solcreek/terraform-provider-slack)](go.mod)
[![License: MPL-2.0](https://img.shields.io/badge/License-MPL--2.0-blue.svg)](LICENSE)
[![OpenTofu compatible](https://img.shields.io/badge/OpenTofu-compatible-FFDA18)](https://opentofu.org/)

Manage Slack Apps as Code with Terraform and OpenTofu.

The provider manages app configuration through Slack's public Manifest API. It supports app creation, full-manifest updates, deletion, import, and manifest export. It uses Terraform Plugin Framework and provider protocol 6.

## Quick start

The intended OpenTofu Registry address is `registry.opentofu.org/solcreek/slack`. Registry publication is pending; until a signed release is indexed, use the local development instructions below. See [release and registry setup](docs/guides/publishing.md) for publication status and maintainer instructions.

```hcl
terraform {
  required_providers {
    slack = {
      source = "registry.opentofu.org/solcreek/slack"
    }
  }
}

provider "slack" {}

resource "slack_app" "notifications" {
  manifest = jsonencode({
    display_information = {
      name = "Notifications"
    }
    features = {
      bot_user = {
        display_name = "Notifications"
      }
    }
    oauth_config = {
      scopes = {
        bot = ["chat:write"]
      }
    }
  })
}
```

Set `SLACK_APP_CONFIGURATION_TOKEN` to an app configuration access token generated in Slack's app settings. Bot tokens, app-level Socket Mode tokens, and ordinary OAuth user tokens are not interchangeable with configuration tokens. Configuration tokens normally expire after 12 hours. Rotate them outside Terraform and persist **both** the new access token and refresh token before the next run. This provider does not rotate tokens or consume one-time refresh tokens during planning.

Run `tofu plan` and `tofu apply`. Terraform users can use the same explicit OpenTofu Registry source with `terraform plan` and `terraform apply`.

## Manifest ownership and drift

`manifest` is the complete desired configuration, not a patch. Slack updates replace the previous configuration, including omitted fields. Maintain one owner for an app's manifest. New Slack manifest fields can be passed through without a provider release; Slack validates the manifest before each create or update.

Use `jsonencode(...)` for HCL, `file(...)` for JSON, or `jsonencode(yamldecode(file("manifest.yaml")))` for YAML. Whitespace and JSON object key ordering do not affect semantic equality. Array ordering remains significant.

The provider stores Slack's post-write export in `exported_manifest`. When later exports match that baseline, it preserves the configured manifest, so Slack-added defaults do not cause repeated updates. Changes anywhere in the exported manifest, including added fields, appear in the next plan. Slack API changes to exported defaults may also produce a plan; review it before applying. A failed post-write export records the known app ID and credentials before returning an error.

## Import and inspect

```sh
terraform import slack_app.notifications A0123456789
```

Import loads the full exported manifest. Copy it into your configuration, then review the plan before applying. A minimal manifest after import can remove existing app settings.

```hcl
data "slack_app" "existing" {
  id = "A0123456789"
}

output "app_manifest" {
  value = jsondecode(data.slack_app.existing.manifest)
}
```

Only an explicit `app_not_found` response removes a resource from state. Authentication and permission errors preserve it. A missing data source returns an error.

## Credentials and API boundaries

App creation returns `client_id`, `client_secret`, `signing_secret`, and an initial `oauth_authorize_url`. Secrets are marked sensitive but remain in Terraform state: use an encrypted backend with restricted access. These credentials cannot be recovered through manifest export, so imported apps have null credential attributes. External credential rotation is not detectable through this API. The initial authorization URL is not refreshed after scope changes.

The provider does not install apps, exchange OAuth authorization codes, create Socket Mode tokens, rotate secrets, upload app icons, or manage channels and workspace memberships. Some scope changes require reinstalling the app in affected workspaces; the provider emits a warning when Slack reports updated permissions. Enabling Socket Mode in the manifest does not create its app-level token.

Destroy permanently deletes the app. For production apps, consider Terraform's `lifecycle { prevent_destroy = true }`.

HTTP requests have a 30-second timeout. Each API call has a five-minute deadline and at most three retries for HTTP 429, respecting `Retry-After`. Network errors and HTTP 5xx are not automatically replayed: after an uncertain create outcome, inspect Slack and import any created app before retrying. Response bodies and tokens are omitted from diagnostics. Production requests use the fixed `https://slack.com/api/` endpoint and reject redirects.

## Development

Requires Go 1.26 or newer. The test suite uses local API fixtures; it does not contact a Slack workspace.

```sh
make build
make test vet
make test-terraform
make test-tofu
make generate
make license-check licenses
```

For local use, put the absolute path to `bin` in your CLI configuration:

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/solcreek/slack" = "/absolute/path/terraform-provider-slack/bin"
    "registry.opentofu.org/solcreek/slack"  = "/absolute/path/terraform-provider-slack/bin"
  }
  direct {}
}
```

Set `TF_CLI_CONFIG_FILE` to that file, then run plan/apply directly in `examples/basic`. The provider itself does not need registry initialization with a development override. Do not commit credentials or state.

CI runs unit/race tests, Terraform and OpenTofu protocol tests, documentation checks, dependency license checks, and vulnerability scanning. Tagged releases use GoReleaser, platform ZIP archives, a protocol manifest, SHA256 checksums, and GPG signatures. Configure the release signing secrets and registry registration before publishing.

No live Slack API verification has been performed for the initial implementation. Validate against a dedicated development workspace before production adoption.

## License

Original source is licensed under MPL-2.0. Dependency license texts are bundled with release artifacts in `third_party`.
