// SPDX-License-Identifier: MPL-2.0
package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/solcreek/terraform-provider-slack/internal/slack"
)

type AppDataSource struct{ client *slack.Client }
type appDataModel struct {
	ID       types.String `tfsdk:"id"`
	Manifest types.String `tfsdk:"manifest"`
}

func NewAppDataSource() datasource.DataSource { return &AppDataSource{} }
func (*AppDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, r *datasource.MetadataResponse) {
	r.TypeName = req.ProviderTypeName + "_app"
}
func (*AppDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, r *datasource.SchemaResponse) {
	r.Schema = schema.Schema{Description: "Export an existing Slack App manifest. This API does not return app credentials.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.RegexMatches(appID, "Must be a Slack App ID beginning with A.")}, Description: "Slack App ID to export."},
		"manifest": schema.StringAttribute{Computed: true, Description: "Complete JSON manifest exported by Slack. Decode using jsondecode."},
	}}
}
func (a *AppDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, r *datasource.ConfigureResponse) {
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
func (a *AppDataSource) Read(ctx context.Context, req datasource.ReadRequest, r *datasource.ReadResponse) {
	var m appDataModel
	r.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if r.Diagnostics.HasError() {
		return
	}
	exported, err := a.client.Export(ctx, m.ID.ValueString())
	if err != nil {
		r.Diagnostics.AddError("App export failed", err.Error())
		return
	}
	m.Manifest = types.StringValue(exported)
	r.Diagnostics.Append(r.State.Set(ctx, &m)...)
}
