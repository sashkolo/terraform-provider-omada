package ssid

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestFlattenPskRead_NullPreserved locks the fix for the open/enterprise drift
// bug: when the controller returns no pskSetting (non-PSK security modes), a
// config that omits psk_setting must stay null in state. Previously this path
// returned an empty pskSettingModel{}, which wrote an empty block to state and
// caused perpetual drift / "Provider produced inconsistent result after apply".
func TestFlattenPskRead_NullPreserved(t *testing.T) {
	t.Parallel()

	// No prior model (psk_setting omitted in config) and no upstream pskSetting
	// (open / enterprise SSID): result must be nil so state stays null.
	if got := flattenPskRead(nil, nil); got != nil {
		t.Fatalf("flattenPskRead(nil, nil) = %+v, want nil (preserve null state)", got)
	}

	// Prior model present but upstream masks/omits pskSetting: the prior model
	// is returned unchanged (PSK preserved from state, no false drift).
	prior := &pskSettingModel{Psk: types.StringValue("keep-me")}
	got := flattenPskRead(prior, nil)
	if got != prior {
		t.Fatalf("flattenPskRead(prior, nil) returned a different pointer; PSK would be lost")
	}
	if got.Psk.ValueString() != "keep-me" {
		t.Fatalf("PSK not preserved when upstream omits pskSetting: got %q", got.Psk.ValueString())
	}
}

// TestFlattenPskRead_ReadRefreshesKey asserts that when the controller returns
// a securityKey, it is written into state (so out-of-band PSK changes surface).
func TestFlattenPskRead_ReadRefreshesKey(t *testing.T) {
	t.Parallel()

	key := "rotated-from-controller"
	got := flattenPskRead(&pskSettingModel{Psk: types.StringValue("old")}, &pskReadVO{
		SecurityKey:       &key,
		EncryptionPsk:     ptrInt32(3),
		VersionPsk:        ptrInt32(4),
		GikRekeyPskEnable: ptrBool(false),
	})
	if got.Psk.ValueString() != key {
		t.Fatalf("Psk = %q, want %q", got.Psk.ValueString(), key)
	}
	if got.EncryptionPsk.ValueInt32() != 3 || got.VersionPsk.ValueInt32() != 4 {
		t.Fatalf("psk knobs not refreshed: enc=%d ver=%d", got.EncryptionPsk.ValueInt32(), got.VersionPsk.ValueInt32())
	}
}

func ptrInt32(v int32) *int32 { return &v }
func ptrBool(v bool) *bool    { return &v }

// TestFlattenSsidRead_ClearsStalePskForNonPskModes locks the fix for the
// stale-psk drift: when the controller reports a non-PSK security mode, any
// prior psk_setting block must be cleared from state so it does not linger
// after an out-of-band PSK -> open/enterprise change. For PSK mode the key is
// still refreshed (or preserved when masked).
func TestFlattenSsidRead_ClearsStalePskForNonPskModes(t *testing.T) {
	t.Parallel()

	// Out-of-band PSK -> open: prior state had a psk_setting; controller now
	// reports security=0 with no pskSetting. State must drop psk_setting.
	m := &ssidResourceModel{
		Security:   types.Int32Value(3),
		PskSetting: &pskSettingModel{Psk: types.StringValue("stale")},
	}
	flattenSsidRead(m, &ssidDetailReadVO{Security: ptrInt32(0)})
	if m.PskSetting != nil {
		t.Fatalf("non-PSK read left stale psk_setting in state: %+v", m.PskSetting)
	}
	if m.Security.ValueInt32() != 0 {
		t.Fatalf("Security = %d, want 0", m.Security.ValueInt32())
	}

	// PSK mode with the controller masking the key (no pskSetting upstream):
	// prior key is preserved.
	m2 := &ssidResourceModel{
		Security:   types.Int32Value(3),
		PskSetting: &pskSettingModel{Psk: types.StringValue("keep-me")},
	}
	flattenSsidRead(m2, &ssidDetailReadVO{Security: ptrInt32(3)})
	if m2.PskSetting == nil || m2.PskSetting.Psk.ValueString() != "keep-me" {
		t.Fatalf("PSK mode with masked key did not preserve prior PSK: %+v", m2.PskSetting)
	}
}
