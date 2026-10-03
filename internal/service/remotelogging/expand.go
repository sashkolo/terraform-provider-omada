package remotelogging

import (
	"github.com/Tohaker/omada-go-sdk/omada"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// expandRemoteLogging builds the PATCH body. The whole target is sent on every
// write; more_client_log carries its prior (or live) value when the config
// leaves it unset, so an edit never resets it.
func expandRemoteLogging(plan remoteLoggingResourceModel) omada.SiteRemoteLoggingSetting {
	vo := omada.RemoteLogSettingVO{
		Enable: plan.Enable.ValueBool(),
		Host:   plan.Host.ValueStringPointer(),
		Port:   plan.Port.ValueInt32Pointer(),
	}
	if !plan.MoreClientLog.IsNull() && !plan.MoreClientLog.IsUnknown() {
		vo.MoreClientLog = plan.MoreClientLog.ValueBoolPointer()
	}
	return omada.SiteRemoteLoggingSetting{RemoteLog: &vo}
}

// flattenRemoteLogging overwrites the model from the read payload. site_id is
// preserved from prior state (it keys the singleton). A missing remoteLog
// object reads as disabled with no target.
func flattenRemoteLogging(m *remoteLoggingResourceModel, r *remoteLoggingReadVO) {
	if r == nil || r.RemoteLog == nil {
		m.Enable = types.BoolValue(false)
		m.Host = types.StringNull()
		m.Port = types.Int32Null()
		m.MoreClientLog = types.BoolValue(false)
		return
	}
	l := r.RemoteLog
	m.Enable = types.BoolValue(l.Enable)
	m.Host = types.StringPointerValue(l.Host)
	m.Port = types.Int32PointerValue(l.Port)
	if l.MoreClientLog == nil {
		m.MoreClientLog = types.BoolValue(false)
	} else {
		m.MoreClientLog = types.BoolValue(*l.MoreClientLog)
	}
}
