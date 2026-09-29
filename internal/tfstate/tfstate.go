// Package tfstate holds state helpers shared by the resources.
package tfstate

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// SaveCreated records an object that Create made on the controller when a later
// step of Create failed (for example the read-back). It saves the plan, with
// every unknown value nulled and the id attribute set, into state. Returned
// together with an error diagnostic, this makes Terraform keep the object as
// tainted instead of forgetting it: the next apply replaces it rather than
// creating a duplicate beside an unmanaged original.
func SaveCreated(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, idAttr string, id string) diag.Diagnostics {
	var diags diag.Diagnostics

	raw, err := tftypes.Transform(plan.Raw, func(_ *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
		if !v.IsKnown() {
			return tftypes.NewValue(v.Type(), nil), nil
		}
		return v, nil
	})
	if err != nil {
		diags.AddError("Error saving the created object", "Could not null unknown plan values: "+err.Error())
		return diags
	}

	state.Raw = raw
	diags.Append(state.SetAttribute(ctx, path.Root(idAttr), id)...)
	return diags
}
