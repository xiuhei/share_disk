package desktopui

import (
	"context"
	"net/http"
	"strings"
	"testing"

	sharediskv1 "github.com/share-disk/share-disk/proto/sharedisk/v1"
)

type fakeHandler struct{}

func (fakeHandler) Handle(_ context.Context, req *sharediskv1.LocalRequest) *sharediskv1.LocalResponse {
	switch req.GetPayload().(type) {
	case *sharediskv1.LocalRequest_Status:
		return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_Status{Status: &sharediskv1.StatusResponse{Version: "test"}}}
	case *sharediskv1.LocalRequest_ListLocalFiles:
		return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_ListLocalFiles{ListLocalFiles: &sharediskv1.ListLocalFilesResponse{}}}
	default:
		return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_Error{Error: &sharediskv1.LocalError{Code: "INVALID", Message: "invalid"}}}
	}
}

func TestDesktopUIShellAndSameOriginBoundary(t *testing.T) {
	s, err := New("127.0.0.1:0", t.TempDir(), fakeHandler{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer s.Close()
	go func() { _ = s.Serve(ctx) }()

	resp, err := http.Get(s.Address() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index status=%d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPost, s.Address()+"/api/files/id/action", strings.NewReader(`{"action":"trash"}`))
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer blocked.Body.Close()
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin mutation status=%d", blocked.StatusCode)
	}
}
