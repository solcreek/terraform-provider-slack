SHELL := /bin/sh
TFPLUGINDOCS := go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0
GO_LICENSES := go run github.com/google/go-licenses/v2@v2.0.1

.PHONY: build test test-terraform test-tofu vet fmt-check generate licenses license-check vulncheck
build:
	go build -trimpath -o bin/terraform-provider-slack .
test:
	go test -race -count=1 -cover -skip '^(TestProtocol|TestAcc)' ./...
test-terraform:
	./scripts/test-protocol.sh terraform
test-tofu:
	./scripts/test-protocol.sh tofu
vet:
	go vet ./...
fmt-check:
	test -z "$$(gofmt -l main.go internal)"
generate:
	$(TFPLUGINDOCS) generate --provider-name slack
licenses:
	$(GO_LICENSES) save ./... --save_path=third_party --force --ignore=github.com/solcreek/terraform-provider-slack
license-check:
	$(GO_LICENSES) check --include_tests ./... --allowed_licenses=MPL-2.0,Apache-2.0,BSD-2-Clause,BSD-3-Clause,MIT,ISC,Unicode-DFS-2016
vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
