data "slack_app" "existing" {
  id = "A0123456789"
}

output "app_manifest" {
  value = jsondecode(data.slack_app.existing.manifest)
}
