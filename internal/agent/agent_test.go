package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sharediskv1 "github.com/share-disk/share-disk/proto/sharedisk/v1"

	"github.com/share-disk/share-disk/internal/config"
	"github.com/share-disk/share-disk/internal/localapi"
	"github.com/share-disk/share-disk/internal/testutil"
)

func newTestAgent(t *testing.T) (*localapi.Client, string) {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.StorageRoot = filepath.Join(root, "objects")
	cfg.Agent.SocketPath = filepath.Join(root, "agent.sock")
	cfg.Database.SQLite = filepath.Join(root, "agent.db")

	a, err := New(cfg, testutil.SQLiteMigrationsDir())
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = a.Run(ctx) }()

	return localapi.NewClient(cfg.Agent.SocketPath), root
}

func TestAgentImportStatVerifyStatus(t *testing.T) {
	client, root := newTestAgent(t)
	ctx := context.Background()

	src := filepath.Join(root, "source.bin")
	if err := os.WriteFile(src, []byte("hello agent object"), 0600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	importResp, err := client.Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Import{Import: &sharediskv1.ImportRequest{SourcePath: src}},
	})
	if err != nil {
		t.Fatalf("import call: %v", err)
	}
	if e := importResp.GetError(); e != nil {
		t.Fatalf("import returned error: %s: %s", e.GetCode(), e.GetMessage())
	}
	objectID := importResp.GetImport().GetFileObjectId()
	if objectID == "" {
		t.Fatal("expected a non-empty object id")
	}

	statResp, err := client.Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Stat{Stat: &sharediskv1.StatRequest{Identifier: objectID}},
	})
	if err != nil {
		t.Fatalf("stat call: %v", err)
	}
	if e := statResp.GetError(); e != nil {
		t.Fatalf("stat returned error: %s: %s", e.GetCode(), e.GetMessage())
	}
	if got := statResp.GetStat().GetObject().GetStatus(); got != "ready" {
		t.Fatalf("expected ready object, got %s", got)
	}
	if got := statResp.GetStat().GetObject().GetChunkCount(); got == 0 {
		t.Fatal("expected non-zero chunk count in stat after import")
	}

	verifyResp, err := client.Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Verify{Verify: &sharediskv1.VerifyRequest{ObjectId: objectID}},
	})
	if err != nil {
		t.Fatalf("verify call: %v", err)
	}
	if e := verifyResp.GetError(); e != nil {
		t.Fatalf("verify returned error: %s: %s", e.GetCode(), e.GetMessage())
	}
	if !verifyResp.GetVerify().GetValid() {
		t.Fatal("expected object to verify as valid")
	}

	statusResp, err := client.Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Status{Status: &sharediskv1.StatusRequest{}},
	})
	if err != nil {
		t.Fatalf("status call: %v", err)
	}
	if e := statusResp.GetError(); e != nil {
		t.Fatalf("status returned error: %s: %s", e.GetCode(), e.GetMessage())
	}
	if got := statusResp.GetStatus().GetObjectCount(); got != 1 {
		t.Fatalf("expected 1 object, got %d", got)
	}
	if statusResp.GetStatus().GetDeviceId() == "" {
		t.Fatal("expected a device id in status")
	}
}

func TestAgentStatUnknownObject(t *testing.T) {
	client, _ := newTestAgent(t)
	ctx := context.Background()

	resp, err := client.Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Stat{Stat: &sharediskv1.StatRequest{Identifier: strings.Repeat("0", 64)}},
	})
	if err != nil {
		t.Fatalf("stat call: %v", err)
	}
	e := resp.GetError()
	if e == nil {
		t.Fatal("expected an error for unknown object")
	}
	if e.GetCode() != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND, got %s", e.GetCode())
	}
}

func TestAgentRejectsMissingPayload(t *testing.T) {
	client, _ := newTestAgent(t)
	ctx := context.Background()

	resp, err := client.Call(ctx, &sharediskv1.LocalRequest{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	e := resp.GetError()
	if e == nil {
		t.Fatal("expected an error for missing payload")
	}
	if e.GetCode() != "INVALID_REQUEST" {
		t.Fatalf("expected INVALID_REQUEST, got %s", e.GetCode())
	}
}

func TestAgentPreventsConcurrentInstances(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.StorageRoot = filepath.Join(root, "objects")
	cfg.Agent.SocketPath = filepath.Join(root, "agent.sock")
	cfg.Database.SQLite = filepath.Join(root, "agent.db")

	a1, err := New(cfg, testutil.SQLiteMigrationsDir())
	if err != nil {
		t.Fatalf("first agent: %v", err)
	}
	defer a1.Close()

	cfg2 := *cfg
	cfg2.Agent.SocketPath = filepath.Join(root, "agent2.sock")
	cfg2.Database.SQLite = filepath.Join(root, "agent2.db")

	_, err = New(&cfg2, testutil.SQLiteMigrationsDir())
	if err == nil {
		t.Fatal("expected second agent to fail due to lock")
	}
}
