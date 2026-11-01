package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createFakeFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(binDir, "ffmpeg")
	// Fake ffmpeg writes a valid small GIF file to the last argument
	script := `#!/bin/sh
for last; do true; done
printf 'GIF89a' > "$last"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binDir
}

func newMockGitHubServer(t *testing.T, postedComments *[]string) *httptest.Server {
	t.Helper()
	var blobSeq int
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/git/ref/heads/"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/blobs"):
			blobSeq++
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"sha":"blob-%d"}`, blobSeq)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/trees"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha":"tree-1"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/commits"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha":"commit-1"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/refs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ref":"refs/heads/demonstra-assets"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/comments"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments"):
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*postedComments = append(*postedComments, body.Body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func TestExecute_RecordPreservesFailingTestExitCodeAndPublishes(t *testing.T) {
	tmp := t.TempDir()
	binDir := createFakeFFmpeg(t, tmp)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var posted []string
	srv := newMockGitHubServer(t, &posted)
	defer srv.Close()

	eventPath := filepath.Join(tmp, "event.json")
	if err := os.WriteFile(eventPath, []byte(`{"pull_request":{"number":42,"head":{"ref":"feat/login"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	summaryPath := filepath.Join(tmp, "summary.md")
	resultsDir := filepath.Join(tmp, "test-results")

	var stdout, stderr bytes.Buffer
	envMap := map[string]string{
		"GITHUB_REPOSITORY":   "acme/shop",
		"GITHUB_SHA":          "a8f921c999999",
		"GITHUB_TOKEN":        "ghs_test",
		"GITHUB_EVENT_PATH":   eventPath,
		"GITHUB_STEP_SUMMARY": summaryPath,
	}

	testCmd := fmt.Sprintf("mkdir -p %q/login-flow && printf 'webm' > %q/login-flow/video.webm && printf 'png' > %q/login-flow/test-failed-1.png && exit 1",
		resultsDir, resultsDir, resultsDir)

	code := Execute(context.Background(), []string{
		"record",
		"--results-dir", resultsDir,
		"--api-url", srv.URL,
		"--",
		"sh", "-c", testCmd,
	}, Env{
		LookupEnv: func(k string) string { return envMap[k] },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})

	if code != 1 {
		t.Fatalf("expected exit code 1 (preserving failing test command), got %d (stderr: %s)", code, stderr.String())
	}
	if len(posted) != 1 {
		t.Fatalf("expected 1 PR comment posted even on test failure, got %d (stderr: %s)", len(posted), stderr.String())
	}
	if !strings.Contains(posted[0], "<!-- demonstra-pr-comment -->") || !strings.Contains(posted[0], "login flow") {
		t.Errorf("unexpected comment body:\n%s", posted[0])
	}
	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil || !strings.Contains(string(summaryBytes), "login flow") {
		t.Errorf("expected GITHUB_STEP_SUMMARY to be populated, err=%v, content=%q", err, string(summaryBytes))
	}
}

func TestExecute_PublishSubcommand(t *testing.T) {
	tmp := t.TempDir()
	binDir := createFakeFFmpeg(t, tmp)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	resultsDir := filepath.Join(tmp, "test-results")
	flowDir := filepath.Join(resultsDir, "checkout-flow")
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flowDir, "video.webm"), []byte("webm"), 0o644); err != nil {
		t.Fatal(err)
	}

	var posted []string
	srv := newMockGitHubServer(t, &posted)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"publish",
		"--results-dir", resultsDir,
		"--repo", "acme/shop",
		"--pr", "7",
		"--sha", "abcdef1234567",
		"--token", "tok",
		"--api-url", srv.URL,
	}, Env{
		LookupEnv: func(string) string { return "" },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if len(posted) != 1 || !strings.Contains(posted[0], "pr-7/abcdef1/checkout-flow/flow.webp?raw=true") || !strings.Contains(posted[0], "pr-7/abcdef1/checkout-flow/video.mp4") {
		t.Fatalf("expected PR #7 comment with WebP and MP4 URLs, got %v", posted)
	}
}

func TestExecute_NonStrictGracefulDegradationOnAPIError(t *testing.T) {
	tmp := t.TempDir()
	binDir := createFakeFFmpeg(t, tmp)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	resultsDir := filepath.Join(tmp, "test-results")
	flowDir := filepath.Join(resultsDir, "checkout-flow")
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flowDir, "video.webm"), []byte("webm"), 0o644); err != nil {
		t.Fatal(err)
	}

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"server error"}`))
	}))
	defer failSrv.Close()

	var stdout, stderr bytes.Buffer
	baseArgs := []string{
		"publish",
		"--results-dir", resultsDir,
		"--repo", "acme/shop",
		"--pr", "7",
		"--sha", "abcdef1234567",
		"--token", "tok",
		"--api-url", failSrv.URL,
	}

	code := Execute(context.Background(), baseArgs, Env{
		LookupEnv: func(string) string { return "" },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if code != 0 {
		t.Fatalf("expected non-strict mode to return 0 on API failure, got %d", code)
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "warning") {
		t.Errorf("expected warning in stderr, got %q", stderr.String())
	}

	stderr.Reset()
	strictArgs := append(append([]string(nil), baseArgs...), "--strict")
	strictCode := Execute(context.Background(), strictArgs, Env{
		LookupEnv: func(string) string { return "" },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if strictCode == 0 {
		t.Fatalf("expected --strict mode to return non-zero on API failure, got 0")
	}
}

func TestExecute_ValidatesBeforeFFmpegAndPrefersPRHeadSHA(t *testing.T) {
	tmp := t.TempDir()
	ffmpegMarker := filepath.Join(tmp, "ffmpeg-ran")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\ntouch %q\nfor last; do true; done\nprintf 'GIF89a' > \"$last\"\n", ffmpegMarker)
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	resultsDir := filepath.Join(tmp, "test-results")
	flowDir := filepath.Join(resultsDir, "checkout-flow")
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flowDir, "video.webm"), []byte("webm"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Missing token with --strict should fail BEFORE invoking ffmpeg
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"publish",
		"--results-dir", resultsDir,
		"--repo", "acme/shop",
		"--pr", "7",
		"--sha", "abcdef1234567",
		"--strict",
	}, Env{
		LookupEnv: func(string) string { return "" },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit code when token is missing in strict mode")
	}
	if _, err := os.Stat(ffmpegMarker); err == nil {
		t.Fatalf("expected validation to fail BEFORE running ffmpeg, but ffmpeg was executed")
	}

	// 2. When GITHUB_EVENT_PATH has pull_request.head.sha, prefer it over GITHUB_SHA merge commit
	var posted []string
	srv := newMockGitHubServer(t, &posted)
	defer srv.Close()

	eventPath := filepath.Join(tmp, "event.json")
	if err := os.WriteFile(eventPath, []byte(`{"pull_request":{"number":19,"head":{"ref":"feat/head-sha","sha":"9876543210fedcba"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	envMap := map[string]string{
		"GITHUB_REPOSITORY": "acme/shop",
		"GITHUB_SHA":        "mergecommit00000",
		"GITHUB_TOKEN":      "ghs_valid",
		"GITHUB_EVENT_PATH": eventPath,
	}
	code = Execute(context.Background(), []string{
		"publish",
		"--results-dir", resultsDir,
		"--api-url", srv.URL,
		"--strict",
	}, Env{
		LookupEnv: func(k string) string { return envMap[k] },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if len(posted) != 1 || !strings.Contains(posted[0], "Commit `9876543`") {
		t.Fatalf("expected comment to use PR head SHA 9876543 instead of merge SHA, got:\n%v", posted)
	}
}

func TestExecute_InitSubcommand(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "playwright.config.ts"), []byte("// pw"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"init",
		"--dir", tmp,
	}, Env{
		LookupEnv: func(string) string { return "" },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Demonstra initialized for playwright project!") {
		t.Errorf("unexpected stdout: %s", stdout.String())
	}

	workflowPath := filepath.Join(tmp, ".github", "workflows", "demonstra.yml")
	if _, err := os.Stat(workflowPath); err != nil {
		t.Errorf("workflow file not created: %v", err)
	}
}

func TestExecute_VersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--version"}, Env{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "demonstra version") {
		t.Errorf("expected version output, got %q", stdout.String())
	}
}

