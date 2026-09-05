package lanapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/identity"
	"github.com/share-disk/share-disk/internal/storage"
	"github.com/share-disk/share-disk/internal/testutil"
)

func TestAuthenticatedResumableUploadListAndRangeDownload(t *testing.T) {
	root := t.TempDir()
	db, err := database.OpenSQLite(filepath.Join(root, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	st, err := storage.New(filepath.Join(root, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewSQLiteObjectStore(st, db, 4*1024*1024)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	privPEM, pubPEM, err := identity.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	server, err := New("127.0.0.1:0", pubPEM, st.IncomingDir(), 1024*1024, 7*24*time.Hour, "", store)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()

	manager, err := identity.NewTokenManager(privPEM, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.GenerateAccessToken("user-1", uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	otherToken, err := manager.GenerateAccessToken("user-2", uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + server.Address()
	requestStatus := func(method, target, bearer string, body io.Reader) int {
		t.Helper()
		req, requestErr := http.NewRequest(method, target, body)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, requestErr := http.DefaultClient.Do(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	unauthorized, err := http.Get(base + "/v1/lan/files")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.StatusCode)
	}

	content := []byte("hello LAN transfer")
	create, err := http.NewRequest(http.MethodPost, base+uploadsPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	create.Header.Set("Authorization", "Bearer "+token)
	create.Header.Set("Tus-Resumable", "1.0.0")
	create.Header.Set("Upload-Length", "18")
	create.Header.Set("Upload-Metadata", "filename "+base64.StdEncoding.EncodeToString([]byte("hello.txt"))+",mime "+base64.StdEncoding.EncodeToString([]byte("text/plain")))
	createResp, err := http.DefaultClient.Do(create)
	if err != nil {
		t.Fatal(err)
	}
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create upload: got %d", createResp.StatusCode)
	}
	location := createResp.Header.Get("Location")
	if location == "" {
		t.Fatal("missing upload location")
	}
	uploadURL, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	if !uploadURL.IsAbs() {
		uploadURL, err = url.Parse(base + location)
		if err != nil {
			t.Fatal(err)
		}
	}

	patch, err := http.NewRequest(http.MethodPatch, uploadURL.String(), bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	patch.Header.Set("Authorization", "Bearer "+token)
	patch.Header.Set("Tus-Resumable", "1.0.0")
	patch.Header.Set("Upload-Offset", "0")
	patch.Header.Set("Content-Type", "application/offset+octet-stream")
	patchResp, err := http.DefaultClient.Do(patch)
	if err != nil {
		t.Fatal(err)
	}
	patchBody, _ := io.ReadAll(patchResp.Body)
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusNoContent {
		t.Fatalf("patch upload: got %d: %s", patchResp.StatusCode, patchBody)
	}
	fileID := patchResp.Header.Get("X-Share-Disk-File-ID")
	sha256 := patchResp.Header.Get("X-Share-Disk-SHA256")
	if fileID == "" || sha256 == "" {
		t.Fatalf("missing completion headers: file=%q sha=%q", fileID, sha256)
	}

	listReq, _ := http.NewRequest(http.MethodGet, base+"/v1/lan/files", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	var listed struct {
		Files []storage.LANFile `json:"files"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Files) != 1 || listed.Files[0].Name != "hello.txt" || listed.Files[0].SHA256 != sha256 {
		t.Fatalf("unexpected file list: %+v", listed.Files)
	}

	duplicate, _ := http.NewRequest(http.MethodPost, base+uploadsPath, nil)
	duplicate.Header.Set("Authorization", "Bearer "+token)
	duplicate.Header.Set("Tus-Resumable", "1.0.0")
	duplicate.Header.Set("Upload-Length", "1")
	duplicate.Header.Set("Upload-Metadata", "filename "+base64.StdEncoding.EncodeToString([]byte("hello.txt")))
	duplicateResp, err := http.DefaultClient.Do(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	duplicateResp.Body.Close()
	if duplicateResp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate filename create returned %d", duplicateResp.StatusCode)
	}

	download, _ := http.NewRequest(http.MethodGet, base+"/v1/lan/files/"+fileID+"/content", nil)
	download.Header.Set("Authorization", "Bearer "+token)
	download.Header.Set("Range", "bytes=6-8")
	downloadResp, err := http.DefaultClient.Do(download)
	if err != nil {
		t.Fatal(err)
	}
	downloadBody, _ := io.ReadAll(downloadResp.Body)
	downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusPartialContent || string(downloadBody) != "LAN" {
		t.Fatalf("unexpected range response: status=%d body=%q", downloadResp.StatusCode, downloadBody)
	}
	if got := downloadResp.Header.Get("X-Content-SHA256"); got != sha256 {
		t.Fatalf("unexpected download hash: %s", got)
	}
	shareToken, err := manager.GenerateShareAccessToken("user-1", fileID, uuid.NewString(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got := requestStatus(http.MethodGet, base+"/v1/lan/files/"+fileID+"/content", shareToken, nil); got != http.StatusOK {
		t.Fatalf("scoped share download returned %d", got)
	}
	if got := requestStatus(http.MethodGet, base+"/v1/lan/files", shareToken, nil); got != http.StatusForbidden {
		t.Fatalf("scoped share list returned %d, want forbidden", got)
	}
	if got := requestStatus(http.MethodDelete, base+"/v1/lan/files/"+fileID, shareToken, nil); got != http.StatusForbidden {
		t.Fatalf("scoped share mutation returned %d, want forbidden", got)
	}

	otherListReq, _ := http.NewRequest(http.MethodGet, base+"/v1/lan/files", nil)
	otherListReq.Header.Set("Authorization", "Bearer "+otherToken)
	otherListResp, err := http.DefaultClient.Do(otherListReq)
	if err != nil {
		t.Fatal(err)
	}
	var otherListed struct {
		Files []storage.LANFile `json:"files"`
	}
	if err := json.NewDecoder(otherListResp.Body).Decode(&otherListed); err != nil {
		t.Fatal(err)
	}
	otherListResp.Body.Close()
	if otherListResp.StatusCode != http.StatusOK || len(otherListed.Files) != 0 {
		t.Fatalf("other account list leaked files: status=%d files=%+v", otherListResp.StatusCode, otherListed.Files)
	}
	if got := requestStatus(http.MethodGet, base+"/v1/lan/files/"+fileID+"/content", otherToken, nil); got != http.StatusNotFound {
		t.Fatalf("other account download returned %d", got)
	}
	if got := requestStatus(http.MethodPatch, base+"/v1/lan/files/"+fileID, otherToken, bytes.NewBufferString(`{"name":"stolen.txt"}`)); got != http.StatusNotFound {
		t.Fatalf("other account rename returned %d", got)
	}
	if got := requestStatus(http.MethodDelete, base+"/v1/lan/files/"+fileID, otherToken, nil); got != http.StatusNotFound {
		t.Fatalf("other account trash returned %d", got)
	}
	if got := requestStatus(http.MethodPatch, base+"/v1/lan/files/"+fileID, token, bytes.NewBufferString(`{"name":"bad.txt"} {}`)); got != http.StatusBadRequest {
		t.Fatalf("rename with trailing JSON returned %d", got)
	}

	rename, _ := http.NewRequest(http.MethodPatch, base+"/v1/lan/files/"+fileID, bytes.NewBufferString(`{"name":"renamed.txt"}`))
	rename.Header.Set("Authorization", "Bearer "+token)
	rename.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(rename)
	if err != nil {
		t.Fatal(err)
	}
	var renamed storage.LANFile
	if err := json.NewDecoder(renameResp.Body).Decode(&renamed); err != nil {
		t.Fatal(err)
	}
	renameResp.Body.Close()
	if renameResp.StatusCode != http.StatusOK || renamed.Name != "renamed.txt" || renamed.SHA256 != sha256 {
		t.Fatalf("unexpected rename response: status=%d file=%+v", renameResp.StatusCode, renamed)
	}

	trash, _ := http.NewRequest(http.MethodDelete, base+"/v1/lan/files/"+fileID, nil)
	trash.Header.Set("Authorization", "Bearer "+token)
	trashResp, err := http.DefaultClient.Do(trash)
	if err != nil {
		t.Fatal(err)
	}
	trashResp.Body.Close()
	if trashResp.StatusCode != http.StatusOK {
		t.Fatalf("trash returned %d", trashResp.StatusCode)
	}

	trashList, _ := http.NewRequest(http.MethodGet, base+"/v1/lan/trash", nil)
	trashList.Header.Set("Authorization", "Bearer "+token)
	trashListResp, err := http.DefaultClient.Do(trashList)
	if err != nil {
		t.Fatal(err)
	}
	var trashListed struct {
		Files []storage.LANFile `json:"files"`
	}
	if err := json.NewDecoder(trashListResp.Body).Decode(&trashListed); err != nil {
		t.Fatal(err)
	}
	trashListResp.Body.Close()
	if len(trashListed.Files) != 1 || trashListed.Files[0].Status != "trashed" {
		t.Fatalf("unexpected trash list: %+v", trashListed.Files)
	}
	otherTrashReq, _ := http.NewRequest(http.MethodGet, base+"/v1/lan/trash", nil)
	otherTrashReq.Header.Set("Authorization", "Bearer "+otherToken)
	otherTrashResp, err := http.DefaultClient.Do(otherTrashReq)
	if err != nil {
		t.Fatal(err)
	}
	var otherTrashListed struct {
		Files []storage.LANFile `json:"files"`
	}
	if err := json.NewDecoder(otherTrashResp.Body).Decode(&otherTrashListed); err != nil {
		t.Fatal(err)
	}
	otherTrashResp.Body.Close()
	if len(otherTrashListed.Files) != 0 {
		t.Fatalf("other account can see trash: %+v", otherTrashListed.Files)
	}
	if got := requestStatus(http.MethodPost, base+"/v1/lan/trash/"+fileID+"/restore", otherToken, nil); got != http.StatusNotFound {
		t.Fatalf("other account restore returned %d", got)
	}
	if got := requestStatus(http.MethodDelete, base+"/v1/lan/trash/"+fileID, otherToken, nil); got != http.StatusNotFound {
		t.Fatalf("other account purge returned %d", got)
	}

	restore, _ := http.NewRequest(http.MethodPost, base+"/v1/lan/trash/"+fileID+"/restore", nil)
	restore.Header.Set("Authorization", "Bearer "+token)
	restoreResp, err := http.DefaultClient.Do(restore)
	if err != nil {
		t.Fatal(err)
	}
	restoreResp.Body.Close()
	if restoreResp.StatusCode != http.StatusOK {
		t.Fatalf("restore returned %d", restoreResp.StatusCode)
	}

	trashAgain, _ := http.NewRequest(http.MethodDelete, base+"/v1/lan/files/"+fileID, nil)
	trashAgain.Header.Set("Authorization", "Bearer "+token)
	trashAgainResp, err := http.DefaultClient.Do(trashAgain)
	if err != nil {
		t.Fatal(err)
	}
	trashAgainResp.Body.Close()

	purge, _ := http.NewRequest(http.MethodDelete, base+"/v1/lan/trash/"+fileID, nil)
	purge.Header.Set("Authorization", "Bearer "+token)
	purgeResp, err := http.DefaultClient.Do(purge)
	if err != nil {
		t.Fatal(err)
	}
	purgeResp.Body.Close()
	if purgeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("purge returned %d", purgeResp.StatusCode)
	}

	afterPurge, _ := http.NewRequest(http.MethodGet, base+"/v1/lan/files/"+fileID+"/content", nil)
	afterPurge.Header.Set("Authorization", "Bearer "+token)
	afterPurgeResp, err := http.DefaultClient.Do(afterPurge)
	if err != nil {
		t.Fatal(err)
	}
	afterPurgeResp.Body.Close()
	if afterPurgeResp.StatusCode != http.StatusNotFound {
		t.Fatalf("download after purge returned %d", afterPurgeResp.StatusCode)
	}
}
