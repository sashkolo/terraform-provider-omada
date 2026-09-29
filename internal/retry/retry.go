// Package retry bounds the short polls the controller's eventually consistent
// lists need (a new or deleted object can take a moment to show in a list).
package retry

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// Budget is the default poll: the controller reflects a write within a few
// seconds; this is a safety margin, not a long poll.
const (
	Attempts = 10
	Interval = 750 * time.Millisecond
)

// Until calls fn up to attempts times, interval apart, until it reports done.
// Each attempt gets fresh diagnostics and only the last attempt's are returned,
// so a transient error followed by a success leaves no error behind. It stops
// early when ctx is cancelled.
func Until(ctx context.Context, attempts int, interval time.Duration, fn func(diags *diag.Diagnostics) bool) (bool, diag.Diagnostics) {
	var last diag.Diagnostics
	for attempt := 0; attempt < attempts; attempt++ {
		last = nil
		if fn(&last) {
			return true, last
		}
		if attempt == attempts-1 {
			break
		}
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			last.AddError("Cancelled", "The operation was cancelled while waiting for the controller: "+ctx.Err().Error())
			return false, last
		}
	}
	return false, last
}
