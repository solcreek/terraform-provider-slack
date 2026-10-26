terraform {
  required_providers {
    slack = {
      source = "registry.opentofu.org/solcreek/slack"
    }
  }
}

# Supply SLACK_APP_CONFIGURATION_TOKEN through your environment.
provider "slack" {}

resource "slack_app" "notifications" {
  manifest = jsonencode({
    display_information = {
      name        = "Notifications"
      description = "Deployment notifications"
    }
    features = {
      bot_user = {
        display_name  = "Notifications"
        always_online = false
      }
    }
    oauth_config = {
      scopes = {
        bot = ["chat:write"]
      }
    }
  })
}
