// SPDX-License-Identifier: MPL-2.0
package provider

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type manifestType struct{ basetypes.StringType }

func (manifestType) String() string                       { return "provider.manifestType" }
func (manifestType) Equal(t attr.Type) bool               { _, ok := t.(manifestType); return ok }
func (manifestType) ValueType(context.Context) attr.Value { return manifestValue{} }
func (manifestType) ValueFromString(_ context.Context, v basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return manifestValue{v}, nil
}
func (t manifestType) ValueFromTerraform(ctx context.Context, v tftypes.Value) (attr.Value, error) {
	x, err := t.StringType.ValueFromTerraform(ctx, v)
	if err != nil {
		return nil, err
	}
	return manifestValue{x.(basetypes.StringValue)}, nil
}

type manifestValue struct{ basetypes.StringValue }

func manifest(s string) manifestValue                { return manifestValue{basetypes.NewStringValue(s)} }
func (manifestValue) Type(context.Context) attr.Type { return manifestType{} }
func (v manifestValue) Equal(o attr.Value) bool {
	x, ok := o.(manifestValue)
	return ok && v.StringValue.Equal(x.StringValue)
}
func (v manifestValue) StringSemanticEquals(_ context.Context, o basetypes.StringValuable) (bool, diag.Diagnostics) {
	x, ok := o.(manifestValue)
	return ok && jsonEqual(v.ValueString(), x.ValueString()), nil
}
func decodeObject(s string) (map[string]any, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var o map[string]any
	if err := d.Decode(&o); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	if o == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return o, nil
}
func jsonEqual(a, b string) bool {
	x, e := decodeObject(a)
	y, f := decodeObject(b)
	return e == nil && f == nil && reflect.DeepEqual(x, y)
}

type manifestValidator struct{}

func (manifestValidator) Description(context.Context) string {
	return "A JSON object containing display_information.name."
}
func (v manifestValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (manifestValidator) ValidateString(ctx context.Context, req validator.StringRequest, r *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	o, err := decodeObject(req.ConfigValue.ValueString())
	if err != nil {
		r.Diagnostics.AddAttributeError(req.Path, "Invalid manifest JSON", "Use jsonencode(...) or jsonencode(yamldecode(file(...))) to supply one JSON object.")
		return
	}
	display, _ := o["display_information"].(map[string]any)
	name, _ := display["name"].(string)
	if strings.TrimSpace(name) == "" {
		r.Diagnostics.AddAttributeError(req.Path, "Missing app name", "The manifest must contain a nonempty display_information.name string.")
	}
}

// Preserve a prior JSON representation when only formatting changed. Terraform
// permits retaining a prior value; real manifest changes still use the plan.
type manifestPlanModifier struct{}

func (manifestPlanModifier) Description(context.Context) string {
	return "Preserve semantically equivalent manifest JSON."
}
func (m manifestPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (manifestPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, r *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		return
	}
	if jsonEqual(req.StateValue.ValueString(), req.PlanValue.ValueString()) {
		r.PlanValue = req.StateValue
	}
}
