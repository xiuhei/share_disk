package discovery

import "testing"

func TestInstanceNameIsStableAndDNSFriendly(t *testing.T) {
	if got, want := instanceName(" 2f47ef4f-120c-4e15 "), "Share Disk 2f47ef4f-120"; got != want {
		t.Fatalf("instanceName() = %q, want %q", got, want)
	}
	if got := instanceName(" !! "); got != "" {
		t.Fatalf("instanceName() = %q, want empty", got)
	}
}

func TestSanitizeTXT(t *testing.T) {
	if got, want := sanitizeTXT(" v=1\n"), "v1"; got != want {
		t.Fatalf("sanitizeTXT() = %q, want %q", got, want)
	}
}
