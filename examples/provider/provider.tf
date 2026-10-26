terraform {
  required_providers {
    slack = {
      source = "registry.opentofu.org/solcreek/slack"
    }
  }
}

# Supply SLACK_APP_CONFIGURATION_TOKEN through your environment.
provider "slack" {}
