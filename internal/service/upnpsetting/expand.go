package upnpsetting

import (
	"context"
	"sort"

	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// expandUpnp builds the PUT body. The endpoint replaces the whole setting, so
// every field is sent on every write:
//   - enable comes from the plan;
//   - network_ids and wan_port_ids come from the plan when the configuration
//     sets them, and otherwise from the live setting read just before the
//     write, so an edit of one field never clears another;
//   - supportByDsLiteAndMapE, which the controller reports and the resource
//     does not manage, is echoed back as read.
//
// The ID lists are always non-nil: the SDK only omits a nil slice, so an empty
// selection is sent as [] rather than left out.
func expandUpnp(ctx context.Context, diags *diag.Diagnostics, plan upnpSettingResourceModel, live *upnpReadVO) omada.UpnpSettingOpenApiVO {
	if live == nil {
		live = &upnpReadVO{}
	}
	return omada.UpnpSettingOpenApiVO{
		Enable:                 plan.Enable.ValueBool(),
		NetworkIds:             idsOrLive(ctx, diags, plan.NetworkIds, live.NetworkIds),
		WanPortIds:             idsOrLive(ctx, diags, plan.WanPortIds, live.WanPortIds),
		SupportByDsLiteAndMapE: live.SupportByDsLiteAndMapE,
	}
}

// idsOrLive returns the set's elements, sorted for a stable body, or a copy of
// the live IDs when the set is null or unknown (left out of the configuration).
func idsOrLive(ctx context.Context, diags *diag.Diagnostics, set types.Set, live []string) []string {
	if set.IsNull() || set.IsUnknown() {
		return uniqueSorted(live)
	}
	var ids []string
	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	return uniqueSorted(ids)
}

// uniqueSorted returns a sorted, de-duplicated, non-nil copy of ids.
func uniqueSorted(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// stringsToSet converts controller IDs to a known set. An omitted field reads
// as the empty set, never null, so an explicit `[]` in the configuration
// matches a setting with nothing selected.
func stringsToSet(ids []string) types.Set {
	unique := uniqueSorted(ids)
	elements := make([]attr.Value, 0, len(unique))
	for _, id := range unique {
		elements = append(elements, types.StringValue(id))
	}
	result, _ := types.SetValue(types.StringType, elements)
	return result
}

// flattenUpnp overwrites every attribute but site_id (which keys the singleton)
// from the read payload.
func flattenUpnp(m *upnpSettingResourceModel, r *upnpReadVO) {
	if r == nil {
		r = &upnpReadVO{}
	}
	m.Enable = types.BoolValue(r.Enable)
	m.NetworkIds = stringsToSet(r.NetworkIds)
	m.WanPortIds = stringsToSet(r.WanPortIds)
	m.SupportByDsLiteAndMapE = types.BoolPointerValue(r.SupportByDsLiteAndMapE)
}
