package transfer

import "testing"

func TestValidTransition(t *testing.T) {
	cases := []struct {
		from string
		to   string
		want bool
	}{
		{"queued", "assigned", true},
		{"queued", "canceled", true},
		{"assigned", "discovering", true},
		{"assigned", "canceled", true},
		{"discovering", "connecting", true},
		{"discovering", "retry_wait", true},
		{"discovering", "failed_permanent", true},
		{"discovering", "canceled", true},
		{"connecting", "transferring", true},
		{"connecting", "retry_wait", true},
		{"connecting", "failed_permanent", true},
		{"connecting", "canceled", true},
		{"transferring", "verifying", true},
		{"transferring", "waiting_source", true},
		{"transferring", "paused", true},
		{"transferring", "failed_permanent", true},
		{"transferring", "canceled", true},
		{"verifying", "completed", true},
		{"verifying", "retry_wait", true},
		{"verifying", "failed_permanent", true},
		{"verifying", "canceled", true},
		{"waiting_source", "queued", true},
		{"waiting_source", "canceled", true},
		{"retry_wait", "queued", true},
		{"retry_wait", "canceled", true},
		{"paused", "transferring", true},
		{"paused", "canceled", true},
		// Terminal states have no outgoing edges.
		{"completed", "queued", false},
		{"completed", "canceled", false},
		{"canceled", "queued", false},
		{"failed_permanent", "queued", false},
		// completed may only be entered from verifying.
		{"queued", "completed", false},
		{"assigned", "completed", false},
		{"transferring", "completed", false},
		// queued is only re-entered from waiting_source/retry_wait.
		{"queued", "retry_wait", false},
		{"queued", "waiting_source", false},
		{"queued", "queued", false},
		{"assigned", "assigned", false},
		// Unknown or invalid targets.
		{"queued", "bogus", false},
		{"queued", "transferring", false},
	}

	for _, tc := range cases {
		if got := validTransition(tc.from, tc.to); got != tc.want {
			t.Errorf("validTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}
