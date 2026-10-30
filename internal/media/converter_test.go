package media

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordedCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []recordedCall
	hook  func(callIndex int, name string, args []string) error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	idx := len(f.calls)
	f.calls = append(f.calls, recordedCall{name: name, args: append([]string(nil), args...)})
	if f.hook != nil {
		return f.hook(idx, name, args)
	}
	return nil
}

func TestConvertToGIF_PrimaryEncodingUnderLimit(t *testing.T) {
	tmp := t.TempDir()
	videoPath := filepath.Join(tmp, "video.webm")
	outGIFPath := filepath.Join(tmp, "out", "flow.gif")

	runner := &fakeRunner{
		hook: func(_ int, _ string, args []string) error {
			out := args[len(args)-1]
			return os.WriteFile(out, bytes.Repeat([]byte("a"), 1024), 0o644)
		},
	}

	conv := NewConverter(runner)
	if err := conv.ConvertToGIF(context.Background(), videoPath, outGIFPath); err != nil {
		t.Fatalf("ConvertToGIF failed: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 ffmpeg call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "ffmpeg" {
		t.Errorf("expected binary ffmpeg, got %q", call.name)
	}
	wantFilter := "fps=12,scale='min(800,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"
	joined := strings.Join(call.args, " ")
	if !strings.Contains(joined, wantFilter) {
		t.Errorf("expected args to contain primary filter %q, got %v", wantFilter, call.args)
	}
}

func TestConvertToGIF_FallbackWhenExceedingMaxBytes(t *testing.T) {
	tmp := t.TempDir()
	videoPath := filepath.Join(tmp, "video.webm")
	outGIFPath := filepath.Join(tmp, "flow.gif")

	runner := &fakeRunner{
		hook: func(callIdx int, _ string, args []string) error {
			out := args[len(args)-1]
			if callIdx == 0 {
				return os.WriteFile(out, bytes.Repeat([]byte("x"), 1500), 0o644)
			}
			return os.WriteFile(out, bytes.Repeat([]byte("x"), 500), 0o644)
		},
	}

	conv := NewConverter(runner)
	conv.MaxBytes = 1000
	if err := conv.ConvertToGIF(context.Background(), videoPath, outGIFPath); err != nil {
		t.Fatalf("ConvertToGIF failed: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 ffmpeg calls (primary + fallback), got %d", len(runner.calls))
	}
	wantFallback := "fps=8,scale='min(600,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=64:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"
	joined := strings.Join(runner.calls[1].args, " ")
	if !strings.Contains(joined, wantFallback) {
		t.Errorf("expected second call to use fallback filter %q, got %v", wantFallback, runner.calls[1].args)
	}
}

func TestConvertToGIF_RunnerError(t *testing.T) {
	tmp := t.TempDir()
	runner := &fakeRunner{
		hook: func(_ int, _ string, _ []string) error {
			return errors.New("exit status 1")
		},
	}
	conv := NewConverter(runner)
	err := conv.ConvertToGIF(context.Background(), filepath.Join(tmp, "in.webm"), filepath.Join(tmp, "out.gif"))
	if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("expected error containing 'ffmpeg', got %v", err)
	}
}
