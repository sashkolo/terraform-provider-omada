package retry

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// A transient error followed by success must leave no error behind; the
// per-resource loops this replaces kept every failed attempt's diagnostic, so
// the apply failed anyway (homelab #514).
func TestUntilKeepsOnlyTheLastAttempt(t *testing.T) {
	calls := 0
	ok, diags := Until(context.Background(), 3, time.Millisecond, func(d *diag.Diagnostics) bool {
		calls++
		if calls == 1 {
			d.AddError("transient", "list read failed")
			return false
		}
		return true
	})
	if !ok || diags.HasError() || calls != 2 {
		t.Fatalf("ok=%v calls=%d diags=%v", ok, calls, diags)
	}
}

func TestUntilReturnsTheFinalFailure(t *testing.T) {
	calls := 0
	ok, diags := Until(context.Background(), 3, time.Millisecond, func(d *diag.Diagnostics) bool {
		calls++
		d.AddError("persistent", "list read failed")
		return false
	})
	if ok || calls != 3 || len(diags.Errors()) != 1 {
		t.Fatalf("ok=%v calls=%d errors=%d", ok, calls, len(diags.Errors()))
	}
}

func TestUntilStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	ok, diags := Until(ctx, 5, time.Hour, func(*diag.Diagnostics) bool {
		calls++
		return false
	})
	if ok || calls != 1 || !diags.HasError() {
		t.Fatalf("ok=%v calls=%d diags=%v", ok, calls, diags)
	}
}
