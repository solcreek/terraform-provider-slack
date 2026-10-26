// SPDX-License-Identifier: MPL-2.0
package provider

import (
	"context"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/solcreek/terraform-provider-slack/internal/slack"
)

type SlackProvider struct{ version string }
type providerModel struct {
	Token types.String `tfsdk:"app_configuration_token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &SlackProvider{version: version} }
}
func (p *SlackProvider) Metadata(_ context.Context, _ provider.MetadataRequest, r *provider.MetadataResponse) {
	r.TypeName = "slack"
	r.Version = p.version
}
func (p *SlackProvider) Schema(_ context.Context, _ provider.SchemaRequest, r *provider.SchemaResponse) {
	r.Schema = schema.Schema{Description: "Manage Slack Apps as Code using Slack's Manifest API.", Attributes: map[string]schema.Attribute{
		"app_configuration_token": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Slack app configuration access token. Defaults to SLACK_APP_CONFIGURATION_TOKEN. Configuration tokens expire after 12 hours; refresh and persist rotated tokens outside Terraform."},
	}}
}
func (p *SlackProvider) Configure(ctx context.Context, req provider.ConfigureRequest, r *provider.ConfigureResponse) {
	var m providerModel
	r.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if r.Diagnostics.HasError() {
		return
	}
	if m.Token.IsUnknown() {
		r.Diagnostics.AddError("Unknown configuration token", "The app configuration token must be known before configuring the provider.")
		return
	}
	token := m.Token.ValueString()
	if m.Token.IsNull() {
		token = os.Getenv("SLACK_APP_CONFIGURATION_TOKEN")
	}
	if strings.TrimSpace(token) == "" {
		r.Diagnostics.AddError("Missing configuration token", "Set app_configuration_token or SLACK_APP_CONFIGURATION_TOKEN to a Slack app configuration access token.")
		return
	}
	client := slack.NewClient(token)
	r.ResourceData = client
	r.DataSourceData = client
}
func (*SlackProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewAppResource}
}
func (*SlackProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewAppDataSource}
}
