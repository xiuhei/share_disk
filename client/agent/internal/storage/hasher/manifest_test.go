package hasher

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestComputeManifestCanonicalDigest(t *testing.T) {
	value, err := ComputeManifest(strings.NewReader("abcdef"), 4)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := value.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonical.Digest()
	if err != nil {
		t.Fatal(err)
	}
	const expected = "5f471dd3266cda5d000188bad27a55ec2bd34b2a591458f01c96731ce054acc1"
	if got := hex.EncodeToString(digest[:]); got != expected {
		t.Fatalf("digest = %s, want %s", got, expected)
	}
}

func TestComputeManifestUsesEffectiveDefaultChunkSize(t *testing.T) {
	value, err := ComputeManifest(strings.NewReader("data"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if value.ChunkSize != 4*1024*1024 {
		t.Fatalf("chunk size = %d", value.ChunkSize)
	}
	if _, err := value.Canonical(); err != nil {
		t.Fatal(err)
	}
}
