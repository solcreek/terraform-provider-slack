// SPDX-License-Identifier: MPL-2.0
package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/solcreek/terraform-provider-slack/internal/slack"
)

func resourceWithResponse(t *testing.T, handler http.HandlerFunc) (*AppResource, tfsdk.Plan) {
	t.Helper()
	client := slack.NewClient("fixture", transportFunc(func(req *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler(w, req)
		return w.Result(), nil
	}))
	a := &AppResource{client: client}
	var sr resource.SchemaResponse
	a.Schema(context.Background(), resource.SchemaRequest{}, &sr)
	plan := tfsdk.Plan{Schema: sr.Schema}
	diags := plan.Set(context.Background(), appModel{
		ID: types.StringUnknown(), Manifest: manifest(`{"display_information":{"name":"Example"}}`), ExportedManifest: types.StringUnknown(),
		ClientID: types.StringUnknown(), ClientSecret: types.StringUnknown(), SigningSecret: types.StringUnknown(), OAuthAuthorizeURL: types.StringUnknown(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	return a, plan
}
func TestCreateRetainsIdentityAfterExportFailure(t *testing.T) {
	a, plan := resourceWithResponse(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/apps.manifest.validate":
			fmt.Fprint(w, `{"ok":true}`)
		case "/api/apps.manifest.create":
			fmt.Fprint(w, `{"ok":true,"app_id":"A123","credentials":{"client_secret":"created-secret","signing_secret":"signing-secret"}}`)
		default:
			fmt.Fprint(w, `{"ok":false,"error":"access_denied"}`)
		}
	})
	response := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}}
	a.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected export failure")
	}
	var m appModel
	if diags := response.State.Get(context.Background(), &m); diags.HasError() {
		t.Fatal(diags)
	}
	if m.ID.ValueString() != "A123" || m.ClientSecret.ValueString() != "created-secret" || m.SigningSecret.ValueString() != "signing-secret" || !m.ExportedManifest.IsNull() {
		t.Fatal("creation identity or credentials lost")
	}
}
func TestValidationFailureDoesNotCreate(t *testing.T) {
	calls := 0
	a, plan := resourceWithResponse(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/apps.manifest.validate" {
			t.Errorf("unexpected write: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"ok":false,"error":"invalid_manifest"}`)
	})
	response := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}}
	a.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() || calls != 1 {
		t.Fatal("validation did not prevent creation")
	}
}
func TestDeleteAlreadyMissing(t *testing.T) {
	a, plan := resourceWithResponse(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ok":false,"error":"app_not_found"}`) })
	state := tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
	var m appModel
	if d := state.Get(context.Background(), &m); d.HasError() {
		t.Fatal(d)
	}
	m.ID = types.StringValue("A123")
	if d := state.Set(context.Background(), &m); d.HasError() {
		t.Fatal(d)
	}
	response := resource.DeleteResponse{State: state}
	a.Delete(context.Background(), resource.DeleteRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatal(response.Diagnostics)
	}
}
