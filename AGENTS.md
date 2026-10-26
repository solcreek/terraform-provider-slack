# Contributor instructions

- Write repository documentation, comments, examples, and diagnostics in US English.
- License original source under MPL-2.0. Preserve required dependency notices.
- Implement against Slack's documented public APIs. Do not copy community provider implementations or add community comparison/provenance sections.
- Maintain Terraform Plugin Framework plan/state consistency, import behavior, and protocol 6 compatibility.
- Run unit tests and Terraform/OpenTofu protocol tests for lifecycle changes. Use local API fixtures by default; never imply fixture tests verify the live Slack service.
- Never send test requests to a real Slack workspace without explicit authorization for that workspace.
- Never expose credentials in diagnostics, logs, example files, or URLs. Sensitive Terraform attributes still persist in state.
- Never automatically retry ambiguous app creation outcomes or rotate configuration refresh tokens during provider configuration.
- Run make generate after schema or example changes. Keep dependency versions pinned and include dependency licenses in release archives.
