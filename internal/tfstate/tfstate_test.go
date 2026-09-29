package tfstate

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSaveCreatedNullsUnknownsAndSetsTheID(t *testing.T) {
	ctx := context.Background()
	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"id":    schema.StringAttribute{Computed: true},
		"name":  schema.StringAttribute{Required: true},
		"index": schema.Int64Attribute{Computed: true},
	}}
	typ := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(typ, map[string]tftypes.Value{
		"id":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":  tftypes.NewValue(tftypes.String, "deny"),
		"index": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(typ, nil)}

	if diags := SaveCreated(ctx, plan, &state, "id", "acl-1"); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.Raw.IsFullyKnown() {
		t.Fatal("state still holds unknown values")
	}

	var got struct {
		ID    *string `tfsdk:"id"`
		Name  string  `tfsdk:"name"`
		Index *int64  `tfsdk:"index"`
	}
	if diags := state.Get(ctx, &got); diags.HasError() {
		t.Fatalf("get: %v", diags)
	}
	if got.ID == nil || *got.ID != "acl-1" || got.Name != "deny" || got.Index != nil {
		t.Fatalf("got id=%v name=%q index=%v", got.ID, got.Name, got.Index)
	}
}
