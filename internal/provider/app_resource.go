// SPDX-License-Identifier: MPL-2.0
package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/solcreek/terraform-provider-slack/internal/slack"
)

type AppResource struct{ client *slack.Client }

var _ resource.ResourceWithConfigure = &AppResource{}
var _ resource.ResourceWithImportState = &AppResource{}

type appModel struct {
	ID                types.String  `tfsdk:"id"`
	Manifest          manifestValue `tfsdk:"manifest"`
	ExportedManifest  types.String  `tfsdk:"exported_manifest"`
	ClientID          types.String  `tfsdk:"client_id"`
	ClientSecret      types.String  `tfsdk:"client_secret"`
	SigningSecret     types.String  `tfsdk:"signing_secret"`
	OAuthAuthorizeURL types.String  `tfsdk:"oauth_authorize_url"`
}

func NewAppResource() resource.Resource { return &AppResource{} }
func (*AppResource) Metadata(_ context.Context, req resource.MetadataRequest, r *resource.MetadataResponse) {
	r.TypeName = req.ProviderTypeName + "_app"
}
func (*AppResource) Schema(_ context.Context, _ resource.SchemaRequest, r *resource.SchemaResponse) {
	stable := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	r.Schema = schema.Schema{Description: "Manage a Slack App using its complete manifest. Updates replace the entire app configuration. Destroy permanently deletes the app. Credentials returned during creation are stored in state and cannot be recovered by import.", Attributes: map[string]schema.Attribute{
		"id":                  schema.StringAttribute{Computed: true, PlanModifiers: stable, Description: "Slack App ID. Import using this ID."},
		"manifest":            schema.StringAttribute{Required: true, CustomType: manifestType{}, PlanModifiers: []planmodifier.String{manifestPlanModifier{}}, Validators: []validator.String{manifestValidator{}}, Description: "Complete app manifest as a JSON object string. Use jsonencode, or jsonencode(yamldecode(file(...))). Unknown Slack fields are passed through. JSON whitespace and object key order are insignificant."},
		"exported_manifest":   schema.StringAttribute{Computed: true, Description: "Last complete manifest exported by Slack, including server defaults. Used to detect remote changes without recurring diffs from defaults."},
		"client_id":           schema.StringAttribute{Computed: true, PlanModifiers: stable, Description: "OAuth client ID returned at creation. Null for imported apps."},
		"client_secret":       schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: stable, Description: "OAuth client secret returned at creation. Stored in state; null for imported apps. External rotation cannot be detected."},
		"signing_secret":      schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: stable, Description: "Signing secret returned at creation. Stored in state; null for imported apps. External rotation cannot be detected."},
		"oauth_authorize_url": schema.StringAttribute{Computed: true, PlanModifiers: stable, Description: "Initial authorization URL returned at creation. Not refreshed after scope changes; null for imported apps."},
	}}
}
func (a *AppResource) Configure(_ context.Context, req resource.ConfigureRequest, r *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*slack.Client)
	if !ok {
		r.Diagnostics.AddError("Unexpected provider client", fmt.Sprintf("Expected *slack.Client, got %T.", req.ProviderData))
		return
	}
	a.client = c
}
func nullable(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
func (a *AppResource) Create(ctx context.Context, req resource.CreateRequest, r *resource.CreateResponse) {
	var m appModel
	r.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if r.Diagnostics.HasError() {
		return
	}
	args := map[string]string{"manifest": m.Manifest.ValueString()}
	if _, err := a.client.Call(ctx, "validate", args); err != nil {
		r.Diagnostics.AddError("Manifest validation failed", err.Error())
		return
	}
	result, err := a.client.Call(ctx, "create", args)
	if err != nil {
		r.Diagnostics.AddError("App creation failed", err.Error())
		return
	}
	if !appID.MatchString(result.AppID) {
		r.Diagnostics.AddError("Missing app identity", "Slack did not return a valid app_id. Check the app dashboard and import any created app before retrying.")
		return
	}
	m.ID = types.StringValue(result.AppID)
	m.ClientID = nullable(result.Credentials.ClientID)
	m.ClientSecret = nullable(result.Credentials.ClientSecret)
	m.SigningSecret = nullable(result.Credentials.SigningSecret)
	m.OAuthAuthorizeURL = nullable(result.OAuthAuthorizeURL)
	m.ExportedManifest = types.StringNull()
	// Persist identity and credentials even if the follow-up export fails.
	r.Diagnostics.Append(r.State.Set(ctx, &m)...)
	exported, err := a.client.Export(ctx, result.AppID)
	if err != nil {
		r.Diagnostics.AddError("App created but export failed", err.Error()+" The app ID and returned credentials have been saved to state.")
		return
	}
	m.ExportedManifest = types.StringValue(exported)
	r.Diagnostics.Append(r.State.Set(ctx, &m)...)
}
func (a *AppResource) Read(ctx context.Context, req resource.ReadRequest, r *resource.ReadResponse) {
	var m appModel
	r.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if r.Diagnostics.HasError() {
		return
	}
	exported, err := a.client.Export(ctx, m.ID.ValueString())
	if slack.IsNotFound(err) {
		r.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		r.Diagnostics.AddError("App read failed", err.Error())
		return
	}
	// Compare the complete export, not only configured keys: remotely added fields
	// must also produce a plan. Retain user formatting while the baseline matches.
	if m.Manifest.IsNull() || m.ExportedManifest.IsNull() || !jsonEqual(exported, m.ExportedManifest.ValueString()) {
		m.Manifest = manifest(exported)
	}
	m.ExportedManifest = types.StringValue(exported)
	r.Diagnostics.Append(r.State.Set(ctx, &m)...)
}
func (a *AppResource) Update(ctx context.Context, req resource.UpdateRequest, r *resource.UpdateResponse) {
	var m appModel
	r.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if r.Diagnostics.HasError() {
		return
	}
	var prior appModel
	r.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if r.Diagnostics.HasError() {
		return
	}
	// Export/update cannot recover creation credentials. Preserve known nulls as
	// well as values, especially for imported apps and omitted API fields.
	m.ClientID = prior.ClientID
	m.ClientSecret = prior.ClientSecret
	m.SigningSecret = prior.SigningSecret
	m.OAuthAuthorizeURL = prior.OAuthAuthorizeURL
	args := map[string]string{"app_id": m.ID.ValueString(), "manifest": m.Manifest.ValueString()}
	if _, err := a.client.Call(ctx, "validate", args); err != nil {
		r.Diagnostics.AddError("Manifest validation failed", err.Error())
		return
	}
	result, err := a.client.Call(ctx, "update", args)
	if err != nil {
		r.Diagnostics.AddError("App update failed", err.Error())
		return
	}
	m.ExportedManifest = types.StringNull()
	r.Diagnostics.Append(r.State.Set(ctx, &m)...)
	if result.PermissionsUpdated {
		r.Diagnostics.AddWarning("App reinstall required", "Slack reported changed permissions. Reinstall the app in each affected workspace to apply new permissions.")
	}
	exported, err := a.client.Export(ctx, m.ID.ValueString())
	if err != nil {
		r.Diagnostics.AddError("App updated but export failed", err.Error())
		return
	}
	m.ExportedManifest = types.StringValue(exported)
	r.Diagnostics.Append(r.State.Set(ctx, &m)...)
}
func (a *AppResource) Delete(ctx context.Context, req resource.DeleteRequest, r *resource.DeleteResponse) {
	var m appModel
	r.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if r.Diagnostics.HasError() {
		return
	}
	_, err := a.client.Call(ctx, "delete", map[string]string{"app_id": m.ID.ValueString()})
	if err != nil && !slack.IsNotFound(err) {
		r.Diagnostics.AddError("App deletion failed", err.Error())
	}
}

var appID = regexp.MustCompile(`^A[A-Z0-9]+$`)

func (*AppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, r *resource.ImportStateResponse) {
	if !appID.MatchString(req.ID) {
		r.Diagnostics.AddError("Invalid app ID", "Import using a Slack App ID beginning with A followed by uppercase letters or digits.")
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, r)
}

func (*AppResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, r *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var before, after manifestValue
	r.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("manifest"), &before)...)
	r.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("manifest"), &after)...)
	if r.Diagnostics.HasError() || before.IsNull() || after.IsUnknown() || !jsonEqual(before.ValueString(), after.ValueString()) {
		return
	}
	var exported types.String
	r.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("exported_manifest"), &exported)...)
	r.Diagnostics.Append(r.Plan.SetAttribute(ctx, path.Root("exported_manifest"), exported)...)
}
