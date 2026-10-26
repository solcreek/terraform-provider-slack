#!/bin/sh
# SPDX-License-Identifier: MPL-2.0
set -eu
cli="${1:-terraform}"
export TF_ACC_TERRAFORM_PATH="$(command -v "$cli")"
export TF_ACC_PROVIDER_NAMESPACE=solcreek
case "$cli" in
  tofu) export TF_ACC_PROVIDER_HOST=registry.opentofu.org ;;
  terraform) export TF_ACC_PROVIDER_HOST=registry.terraform.io ;;
  *) echo "Usage: $0 [terraform|tofu]" >&2; exit 2 ;;
esac
go test -race -count=1 -timeout=10m -run '^TestProtocol' ./internal/provider
