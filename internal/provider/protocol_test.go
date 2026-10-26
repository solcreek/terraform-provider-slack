// SPDX-License-Identifier: MPL-2.0
package provider

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"

	framework "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/solcreek/terraform-provider-slack/internal/slack"
)

var protocolCLI = flag.String("protocol-cli", "", "Terraform-compatible CLI for local protocol tests")

type testProvider struct {
	SlackProvider
	client *slack.Client
}

func (p *testProvider) Configure(_ context.Context, _ framework.ConfigureRequest, r *framework.ConfigureResponse) {
	r.ResourceData = p.client
	r.DataSourceData = p.client
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakeSlack struct {
	mu                        sync.Mutex
	manifest                  map[string]any
	creates, updates, deletes int
	deny                      bool
}

func (f *fakeSlack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(v any) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			panic(err)
		}
	}
	args := map[string]string{}
	if json.NewDecoder(r.Body).Decode(&args) != nil {
		w.WriteHeader(400)
		return
	}
	if r.Header.Get("Authorization") != "Bearer fixture" {
		write(map[string]any{"ok": false, "error": "invalid_auth"})
		return
	}
	method := strings.TrimPrefix(r.URL.Path, "/api/apps.manifest.")
	if f.deny {
		write(map[string]any{"ok": false, "error": "access_denied"})
		return
	}
	if method != "create" && method != "validate" && f.manifest == nil {
		write(map[string]any{"ok": false, "error": "app_not_found"})
		return
	}
	switch method {
	case "validate":
		write(map[string]any{"ok": true})
	case "create", "update":
		if method == "create" {
			f.creates++
		} else {
			f.updates++
		}
		if err := json.Unmarshal([]byte(args["manifest"]), &f.manifest); err != nil {
			w.WriteHeader(400)
			return
		}
		// Simulate Slack adding defaults. This must not cause a perpetual plan.
		f.manifest["_metadata"] = map[string]any{"major_version": 1.0, "minor_version": 1.0}
		write(map[string]any{"ok": true, "app_id": "A123", "permissions_updated": method == "update", "credentials": map[string]string{"client_id": "123.456", "client_secret": "fixture-client-secret", "signing_secret": "fixture-signing-secret"}})
	case "export":
		write(map[string]any{"ok": true, "manifest": f.manifest})
	case "delete":
		f.deletes++
		f.manifest = nil
		write(map[string]any{"ok": true})
	default:
		w.WriteHeader(404)
	}
}
func protocolFixture(t *testing.T) (*fakeSlack, map[string]func() (tfprotov6.ProviderServer, error)) {
	t.Helper()
	if *protocolCLI != "" {
		executable, err := exec.LookPath(*protocolCLI)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("TF_ACC_TERRAFORM_PATH", executable)
		t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "solcreek")
		host := "registry.terraform.io"
		if *protocolCLI == "tofu" {
			host = "registry.opentofu.org"
		}
		t.Setenv("TF_ACC_PROVIDER_HOST", host)
	}
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		t.Skip("run make test-terraform or make test-tofu")
	}
	f := &fakeSlack{}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	client := slack.NewClient("fixture", transportFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return http.DefaultTransport.RoundTrip(clone)
	}))
	factories := map[string]func() (tfprotov6.ProviderServer, error){"slack": providerserver.NewProtocol6WithError(&testProvider{client: client})}
	return f, factories
}
func appConfig(name string) string {
	return fmt.Sprintf(`resource "slack_app" "test" { manifest = jsonencode({display_information={name=%q}}) }`, name)
}
func TestProtocolLifecycle(t *testing.T) {
	f, factories := protocolFixture(t)
	config := appConfig("First")
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("slack_app.test", "id", "A123"), resource.TestCheckResourceAttr("slack_app.test", "client_secret", "fixture-client-secret"))},
		{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
		{Config: appConfig("Second"), Check: resource.TestCheckResourceAttr("slack_app.test", "client_secret", "fixture-client-secret")},
		{Config: appConfig("Second"), PreConfig: func() {
			f.mu.Lock()
			f.manifest["display_information"].(map[string]any)["name"] = "Remote"
			f.mu.Unlock()
		}, PlanOnly: true, ExpectNonEmptyPlan: true},
		{Config: appConfig("Second")},
		{Config: appConfig("Second"), PreConfig: func() { f.mu.Lock(); f.manifest["new_slack_field"] = true; f.mu.Unlock() }, PlanOnly: true, ExpectNonEmptyPlan: true},
		{Config: appConfig("Second")},
		{ResourceName: "slack_app.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"manifest", "client_id", "client_secret", "signing_secret", "oauth_authorize_url"}},
		{Config: appConfig("Second") + `
data "slack_app" "existing" { id=slack_app.test.id }`, Check: resource.TestCheckResourceAttr("data.slack_app.existing", "id", "A123")},
	}})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.creates != 1 || f.updates != 3 || f.deletes != 1 {
		t.Fatalf("create/update/delete=%d/%d/%d", f.creates, f.updates, f.deletes)
	}
}
func TestProtocolRemoteDelete(t *testing.T) {
	f, factories := protocolFixture(t)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: appConfig("First")},
		{Config: appConfig("First"), PreConfig: func() { f.mu.Lock(); f.manifest = nil; f.mu.Unlock() }},
	}})
	if f.creates != 2 {
		t.Fatalf("expected recreation, got %d creates", f.creates)
	}
}
func TestProtocolAccessDenied(t *testing.T) {
	f, factories := protocolFixture(t)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: appConfig("First")},
		{Config: appConfig("First"), PreConfig: func() { f.mu.Lock(); f.deny = true; f.mu.Unlock() }, PlanOnly: true, ExpectError: regexp.MustCompile("access_denied")},
		{Config: appConfig("First"), PreConfig: func() { f.mu.Lock(); f.deny = false; f.mu.Unlock() }},
	}})
	if f.creates != 1 {
		t.Fatal("permission failure must not recreate the app")
	}
}
func TestProtocolInvalidManifest(t *testing.T) {
	_, factories := protocolFixture(t)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: `resource "slack_app" "test" {manifest="[]"}`, PlanOnly: true, ExpectError: regexp.MustCompile("Invalid manifest JSON")},
		{Config: `resource "slack_app" "test" {manifest="{}"}`, PlanOnly: true, ExpectError: regexp.MustCompile("Missing app name")},
	}})
}

func TestProtocolJSONFormatting(t *testing.T) {
	_, factories := protocolFixture(t)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: appConfig("First")},
		{Config: `resource "slack_app" "test" {
 manifest = <<EOT
{ "display_information": { "name": "First" } }
EOT
}`, PlanOnly: true, ExpectNonEmptyPlan: false},
	}})
}

func TestProtocolUpdateImportedApp(t *testing.T) {
	f, factories := protocolFixture(t)
	f.manifest = map[string]any{"display_information": map[string]any{"name": "Existing"}}
	header := "terraform {\n required_providers {\n slack = { source = \"solcreek/slack\" }\n }\n}\n"

	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: header + appConfig("Existing"), ResourceName: "slack_app.test", ImportState: true, ImportStateId: "A123", ImportStatePersist: true},
		{Config: header + appConfig("Updated"), Check: resource.TestCheckNoResourceAttr("slack_app.test", "client_secret")},
	}})
	if f.creates != 0 || f.updates != 1 {
		t.Fatalf("unexpected create/update: %d/%d", f.creates, f.updates)
	}
}
