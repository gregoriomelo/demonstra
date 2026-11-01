package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestPNG(path string, w, h int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 120, B: 200, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

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

func TestConvertToMP4_Success(t *testing.T) {
	tmp := t.TempDir()
	videoPath := filepath.Join(tmp, "video.webm")
	outMP4Path := filepath.Join(tmp, "out", "video.mp4")

	runner := &fakeRunner{
		hook: func(_ int, _ string, args []string) error {
			out := args[len(args)-1]
			return os.WriteFile(out, []byte("fake-mp4-data"), 0o644)
		},
	}

	conv := NewConverter(runner)
	if err := conv.ConvertToMP4(context.Background(), videoPath, outMP4Path); err != nil {
		t.Fatalf("ConvertToMP4 failed: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 ffmpeg call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "ffmpeg" {
		t.Errorf("expected binary ffmpeg, got %q", call.name)
	}
	joined := strings.Join(call.args, " ")
	if !strings.Contains(joined, "libx264") || !strings.Contains(joined, "yuv420p") || !strings.Contains(joined, "+faststart") {
		t.Errorf("expected mp4 conversion flags, got %v", call.args)
	}
	data, err := os.ReadFile(outMP4Path)
	if err != nil || string(data) != "fake-mp4-data" {
		t.Errorf("unexpected output content: %s, err: %v", string(data), err)
	}
}

func TestConvertToMP4_DirectCopyWhenAlreadyMP4(t *testing.T) {
	tmp := t.TempDir()
	inPath := filepath.Join(tmp, "source.mp4")
	outPath := filepath.Join(tmp, "out", "dest.mp4")
	if err := os.WriteFile(inPath, []byte("original-mp4"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{}
	conv := NewConverter(runner)
	if err := conv.ConvertToMP4(context.Background(), inPath, outPath); err != nil {
		t.Fatalf("ConvertToMP4 failed: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected 0 ffmpeg calls when copying mp4, got %d", len(runner.calls))
	}
	data, _ := os.ReadFile(outPath)
	if string(data) != "original-mp4" {
		t.Errorf("expected copied content, got %q", string(data))
	}
}

func TestConvertToWebP_Success(t *testing.T) {
	tmp := t.TempDir()
	videoPath := filepath.Join(tmp, "video.webm")
	outWebPPath := filepath.Join(tmp, "out", "flow.webp")

	runner := &fakeRunner{
		hook: func(_ int, _ string, args []string) error {
			out := args[len(args)-1]
			return os.WriteFile(out, []byte("fake-webp-data"), 0o644)
		},
	}

	conv := NewConverter(runner)
	if err := conv.ConvertToWebP(context.Background(), videoPath, outWebPPath); err != nil {
		t.Fatalf("ConvertToWebP failed: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 ffmpeg call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "ffmpeg" {
		t.Errorf("expected binary ffmpeg, got %q", call.name)
	}
	joined := strings.Join(call.args, " ")
	if !strings.Contains(joined, "libwebp") || !strings.Contains(joined, "-loop 0") {
		t.Errorf("expected webp conversion flags, got %v", call.args)
	}
	data, err := os.ReadFile(outWebPPath)
	if err != nil || string(data) != "fake-webp-data" {
		t.Errorf("unexpected output content: %s, err: %v", string(data), err)
	}
}

func TestConvertToWebP_DirectCopyWhenAlreadyWebP(t *testing.T) {
	tmp := t.TempDir()
	inPath := filepath.Join(tmp, "source.webp")
	outPath := filepath.Join(tmp, "out", "dest.webp")
	if err := os.WriteFile(inPath, []byte("original-webp"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{}
	conv := NewConverter(runner)
	if err := conv.ConvertToWebP(context.Background(), inPath, outPath); err != nil {
		t.Fatalf("ConvertToWebP failed: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected 0 ffmpeg calls when copying webp, got %d", len(runner.calls))
	}
	data, _ := os.ReadFile(outPath)
	if string(data) != "original-webp" {
		t.Errorf("expected copied content, got %q", string(data))
	}
}

func TestEncodeGIFsFromPNGs(t *testing.T) {
	tmp := t.TempDir()
	png1 := filepath.Join(tmp, "step-1.png")
	png2 := filepath.Join(tmp, "step-2.png")
	if err := writeTestPNG(png1, 100, 60); err != nil {
		t.Fatal(err)
	}
	if err := writeTestPNG(png2, 100, 60); err != nil {
		t.Fatal(err)
	}

	outGIF := filepath.Join(tmp, "out", "anim.gif")
	if err := EncodeGIFsFromPNGs([]string{png1, png2}, outGIF, 100); err != nil {
		t.Fatalf("EncodeGIFsFromPNGs failed: %v", err)
	}
	data, err := os.ReadFile(outGIF)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("GIF89a")) {
		t.Errorf("expected valid GIF89a header, got %q", data[:6])
	}
}

