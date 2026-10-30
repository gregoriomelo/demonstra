package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demonstra-me/demonstra/internal/collector"
)

func TestUploadPRScenarios_OrphanBranchCreation(t *testing.T) {
	tmp := t.TempDir()
	gifPath := filepath.Join(tmp, "flow.gif")
	shotPath := filepath.Join(tmp, "step-1.png")
	if err := os.WriteFile(gifPath, []byte("GIF89a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shotPath, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	var blobCount int
	var gotTreeBase string
	var createdRef string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/git/ref/heads/demonstra-assets":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/git/blobs":
			blobCount++
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"sha":"blob-%d"}`, blobCount)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/git/trees":
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			if bt, ok := payload["base_tree"].(string); ok {
				gotTreeBase = bt
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha":"new-tree-sha"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/git/commits":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha":"new-commit-sha"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/git/refs":
			body, _ := io.ReadAll(r.Body)
			var payload struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			}
			_ = json.Unmarshal(body, &payload)
			createdRef = payload.Ref
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ref":"refs/heads/demonstra-assets"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient("acme", "shop", "secret-token", "demonstra-assets")
	client.BaseURL = srv.URL

	uploaded, prev, err := client.UploadPRScenarios(context.Background(), 42, "a8f921c999999", []ScenarioAssetInput{
		{
			Scenario: collector.Scenario{
				Title:  "Apply coupon",
				Slug:   "apply-coupon",
				Status: collector.StatusPassed,
			},
			GIFPath:        gifPath,
			ScreenshotPath: []string{shotPath},
		},
	})
	if err != nil {
		t.Fatalf("UploadPRScenarios failed: %v", err)
	}
	if gotTreeBase != "" {
		t.Errorf("expected empty base_tree on orphan creation, got %q", gotTreeBase)
	}
	if createdRef != "refs/heads/demonstra-assets" {
		t.Errorf("expected created ref refs/heads/demonstra-assets, got %q", createdRef)
	}
	if len(prev) != 0 {
		t.Errorf("expected 0 previous runs, got %d", len(prev))
	}
	if len(uploaded) != 1 {
		t.Fatalf("expected 1 uploaded scenario, got %d", len(uploaded))
	}
	wantGIF := "https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/apply-coupon/flow.gif?raw=true"
	if uploaded[0].GIFURL != wantGIF {
		t.Errorf("expected GIFURL %q, got %q", wantGIF, uploaded[0].GIFURL)
	}
	wantShot := "https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/apply-coupon/screenshots/step-1.png?raw=true"
	if len(uploaded[0].ScreenshotURLs) != 1 || uploaded[0].ScreenshotURLs[0] != wantShot {
		t.Errorf("expected ScreenshotURL %q, got %v", wantShot, uploaded[0].ScreenshotURLs)
	}
}

func TestUploadPRScenarios_ExistingBranchWithPreviousRuns(t *testing.T) {
	tmp := t.TempDir()
	gifPath := filepath.Join(tmp, "flow.gif")
	if err := os.WriteFile(gifPath, []byte("GIF89a"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotBaseTree string
	var patchedRef bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/git/ref/heads/demonstra-assets":
			_, _ = w.Write([]byte(`{"object":{"sha":"existing-commit-sha"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/git/commits/existing-commit-sha":
			_, _ = w.Write([]byte(`{"sha":"existing-commit-sha","tree":{"sha":"existing-tree-sha"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/git/trees/existing-tree-sha":
			_, _ = w.Write([]byte(`{"tree":[{"path":"pr-42/1111111/apply-coupon/flow.gif","type":"blob"},{"path":"pr-99/2222222/other/flow.gif","type":"blob"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/git/blobs":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha":"blob-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/git/trees":
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			if bt, ok := payload["base_tree"].(string); ok {
				gotBaseTree = bt
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha":"new-tree-sha"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/git/commits":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha":"new-commit-sha"}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/acme/shop/git/refs/heads/demonstra-assets":
			patchedRef = true
			_, _ = w.Write([]byte(`{"ref":"refs/heads/demonstra-assets"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient("acme", "shop", "secret-token", "demonstra-assets")
	client.BaseURL = srv.URL

	_, prev, err := client.UploadPRScenarios(context.Background(), 42, "a8f921c999999", []ScenarioAssetInput{
		{
			Scenario: collector.Scenario{Title: "Apply coupon", Slug: "apply-coupon"},
			GIFPath:  gifPath,
		},
	})
	if err != nil {
		t.Fatalf("UploadPRScenarios failed: %v", err)
	}
	if gotBaseTree != "existing-tree-sha" {
		t.Errorf("expected base_tree existing-tree-sha, got %q", gotBaseTree)
	}
	if !patchedRef {
		t.Errorf("expected ref to be updated via PATCH")
	}
	if len(prev) != 1 || prev[0].ShortSHA != "1111111" {
		t.Fatalf("expected 1 previous run with ShortSHA 1111111, got %+v", prev)
	}
	wantTree := "https://github.com/acme/shop/tree/demonstra-assets/pr-42/1111111"
	if prev[0].TreeURL != wantTree {
		t.Errorf("expected TreeURL %q, got %q", wantTree, prev[0].TreeURL)
	}
}

func TestUpsertPRComment_CreateAndUpdate(t *testing.T) {
	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	var postCount, patchCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/issues/42/comments":
			_ = json.NewEncoder(w).Encode(comments)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/issues/42/comments":
			postCount++
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			comments = append(comments, struct {
				ID   int64  `json:"id"`
				Body string `json:"body"`
			}{ID: 777, Body: body.Body})
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":777}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/acme/shop/issues/comments/777":
			patchCount++
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			comments[0].Body = body.Body
			_, _ = w.Write([]byte(`{"id":777}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient("acme", "shop", "secret-token", "demonstra-assets")
	client.BaseURL = srv.URL

	body1 := CommentMarker + "\nRun 1"
	if err := client.UpsertPRComment(context.Background(), 42, body1); err != nil {
		t.Fatalf("first UpsertPRComment failed: %v", err)
	}
	if postCount != 1 || patchCount != 0 {
		t.Fatalf("expected 1 POST and 0 PATCH, got %d POST, %d PATCH", postCount, patchCount)
	}

	body2 := CommentMarker + "\nRun 2"
	if err := client.UpsertPRComment(context.Background(), 42, body2); err != nil {
		t.Fatalf("second UpsertPRComment failed: %v", err)
	}
	if postCount != 1 || patchCount != 1 {
		t.Fatalf("expected 1 POST and 1 PATCH, got %d POST, %d PATCH", postCount, patchCount)
	}
	if !strings.Contains(comments[0].Body, "Run 2") {
		t.Errorf("expected updated comment body to contain Run 2, got %q", comments[0].Body)
	}
}
