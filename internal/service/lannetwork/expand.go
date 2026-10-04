package lannetwork

import (
	"context"

	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	// purposeInterface (1) is a gateway-terminated LAN network with an IP
	// interface (gatewaySubnet). This is the common shape and the resource
	// default. purposeVlan (0) is a VLAN-only network with no gateway interface.
	purposeInterface int32 = 1
	// vlanTypeSingle (0) is one VLAN tag per network, as opposed to a
	// multi-VLAN network (vlanType 1).
	vlanTypeSingle int32 = 0
	// errNetworkNotFound is the Omada Open API error code for a missing LAN
	// network. Delete treats it as already gone.
	errNetworkNotFound int32 = -33503
)

// expandDhcpSettings converts the optional dhcp_settings block into the SDK
// type. Returns nil when the block is absent so the network is created without
// gateway DHCP.
func expandDhcpSettings(s *dhcpSettingsModel) *omada.DhcpSettings {
	if s == nil {
		return nil
	}

	return &omada.DhcpSettings{
		Enable:      s.Enable.ValueBoolPointer(),
		Dhcpns:      s.Dhcpns.ValueStringPointer(),
		Gateway:     s.Gateway.ValueStringPointer(),
		IpaddrStart: s.IpaddrStart.ValueStringPointer(),
		IpaddrEnd:   s.IpaddrEnd.ValueStringPointer(),
		Leasetime:   s.Leasetime.ValueInt32Pointer(),
		PriDns:      s.PriDns.ValueStringPointer(),
		SndDns:      s.SndDns.ValueStringPointer(),
		Options:     expandDhcpOptions(s.Options),
	}
}

// expandDhcpOptions converts the custom DHCP options. An unset or empty list
// returns nil, which the SDK omits; carryUnmodeled refuses an update that would
// drop live options that way.
func expandDhcpOptions(opts []dhcpOptionModel) []omada.CustomDHCPOptions {
	if len(opts) == 0 {
		return nil
	}
	out := make([]omada.CustomDHCPOptions, 0, len(opts))
	for _, o := range opts {
		out = append(out, omada.CustomDHCPOptions{
			Code:  o.Code.ValueInt32Pointer(),
			Type:  o.Type.ValueInt32Pointer(),
			Value: o.Value.ValueStringPointer(),
		})
	}
	return out
}

// expandInterfaceIds converts the Terraform list of interface IDs (gateway LAN
// port IDs) into the SDK slice. An unset list returns nil, which the SDK omits,
// so the controller keeps the network's ports; before, it sent [], which asks
// to unbind every port. An explicit [] is still sent.
func expandInterfaceIds(ids []types.String) []string {
	if ids == nil {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id.IsNull() || id.IsUnknown() {
			continue
		}
		out = append(out, id.ValueString())
	}

	return out
}

// expandLanNetwork builds the SDK LAN network value sent on Create and Modify.
// vlanType is fixed to Single and the VLAN tag is taken from vlan_id.
func expandLanNetwork(plan lanNetworkResourceModel) omada.LanNetworkOpenApiVO {
	vlan := plan.VlanId.ValueInt32()
	vlanType := vlanTypeSingle
	purpose := purposeInterface
	if !plan.Purpose.IsNull() && !plan.Purpose.IsUnknown() {
		purpose = plan.Purpose.ValueInt32()
	}

	// isolation is set by the caller from the configuration only (see
	// configIsolation): the plan's value may come from state.
	return omada.LanNetworkOpenApiVO{
		Name:            plan.Name.ValueString(),
		Purpose:         purpose,
		Vlan:            &vlan,
		VlanType:        &vlanType,
		GatewaySubnet:   plan.GatewaySubnet.ValueStringPointer(),
		InterfaceIds:    expandInterfaceIds(plan.InterfaceIds),
		Domain:          plan.Domain.ValueStringPointer(),
		IgmpSnoopEnable: plan.IgmpSnoopEnable.ValueBool(),
		DhcpSettingsVO:  expandDhcpSettings(plan.DhcpSettings),
	}
}

// configIsolation returns isolation when the configuration sets it, else nil.
// The controller doesn't return isolation, so a value that came from state
// can't be told from a live one; sending it would undo a change made in the
// UI. Unset, create leaves the controller default and update carries the live
// value (carryUnmodeled).
func configIsolation(ctx context.Context, cfg tfsdk.Config, diags *diag.Diagnostics) *bool {
	var v types.Bool
	diags.Append(cfg.GetAttribute(ctx, path.Root("isolation"), &v)...)
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return v.ValueBoolPointer()
}
