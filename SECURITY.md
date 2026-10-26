# Security policy

Please report suspected vulnerabilities through [GitHub private vulnerability reporting](https://github.com/solcreek/terraform-provider-slack/security/advisories/new). Do not include access tokens, app credentials, or Terraform state in public issues.

Security fixes target the latest released version. Before the first release, fixes target the main branch.

Sensitive Terraform attributes are stored in state. Use an encrypted backend, restrict access, and supply provider credentials through the environment. See the README for credential and API limitations.
