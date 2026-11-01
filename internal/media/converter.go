package media

import (
	"context"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const DefaultMaxGIFBytes int64 = 9 * 1024 * 1024 // 9 MB (9437184 bytes)

const (
	primaryFilter  = "fps=12,scale='min(800,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"
	fallbackFilter = "fps=8,scale='min(600,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=64:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return nil
}

type Converter struct {
	Runner   CommandRunner
	MaxBytes int64
}

func NewConverter(runner CommandRunner) *Converter {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Converter{
		Runner:   runner,
		MaxBytes: DefaultMaxGIFBytes,
	}
}

func (c *Converter) ConvertToGIF(ctx context.Context, videoPath string, outGIFPath string) error {
	if err := os.MkdirAll(filepath.Dir(outGIFPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if strings.EqualFold(filepath.Ext(videoPath), ".gif") {
		raw, err := os.ReadFile(videoPath)
		if err != nil {
			return err
		}
		return os.WriteFile(outGIFPath, raw, 0o644)
	}
	if err := c.runFFmpeg(ctx, videoPath, outGIFPath, primaryFilter); err != nil {
		return err
	}
	maxBytes := c.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxGIFBytes
	}
	if info, err := os.Stat(outGIFPath); err == nil && info.Size() > maxBytes {
		if err := c.runFFmpeg(ctx, videoPath, outGIFPath, fallbackFilter); err != nil {
			return err
		}
	}
	return nil
}

func (c *Converter) ConvertToMP4(ctx context.Context, videoPath string, outMP4Path string) error {
	if err := os.MkdirAll(filepath.Dir(outMP4Path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if strings.EqualFold(filepath.Ext(videoPath), ".mp4") {
		raw, err := os.ReadFile(videoPath)
		if err != nil {
			return err
		}
		return os.WriteFile(outMP4Path, raw, 0o644)
	}
	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	args := []string{"-y", "-i", videoPath, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", outMP4Path}
	if err := runner.Run(ctx, "ffmpeg", args...); err != nil {
		return fmt.Errorf("ffmpeg mp4 conversion failed: %w", err)
	}
	return nil
}

func (c *Converter) ConvertToWebP(ctx context.Context, videoPath string, outWebPPath string) error {
	if err := os.MkdirAll(filepath.Dir(outWebPPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if strings.EqualFold(filepath.Ext(videoPath), ".webp") {
		raw, err := os.ReadFile(videoPath)
		if err != nil {
			return err
		}
		return os.WriteFile(outWebPPath, raw, 0o644)
	}
	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	filter := "fps=12,scale='min(800,iw)':-1:flags=lanczos"
	args := []string{
		"-y", "-i", videoPath,
		"-vcodec", "libwebp",
		"-filter:v", filter,
		"-lossless", "0",
		"-compression_level", "4",
		"-q:v", "75",
		"-loop", "0",
		outWebPPath,
	}
	if err := runner.Run(ctx, "ffmpeg", args...); err != nil {
		return fmt.Errorf("ffmpeg webp conversion failed: %w", err)
	}
	return nil
}

func EncodeGIFsFromPNGs(pngPaths []string, outGIFPath string, delayHundredths int) error {
	if len(pngPaths) == 0 {
		return fmt.Errorf("no PNG files provided")
	}
	if delayHundredths <= 0 {
		delayHundredths = 120
	}
	if err := os.MkdirAll(filepath.Dir(outGIFPath), 0o755); err != nil {
		return err
	}
	anim := &gif.GIF{}
	for _, p := range pngPaths {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		src, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("decode png %s: %w", p, err)
		}
		bounds := src.Bounds()
		paletted := image.NewPaletted(bounds, palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, bounds, src, bounds.Min)
		anim.Image = append(anim.Image, paletted)
		anim.Delay = append(anim.Delay, delayHundredths)
	}
	outFile, err := os.Create(outGIFPath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	return gif.EncodeAll(outFile, anim)
}

func (c *Converter) runFFmpeg(ctx context.Context, videoPath, outGIFPath, filter string) error {
	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := runner.Run(ctx, "ffmpeg", "-y", "-i", videoPath, "-vf", filter, outGIFPath); err != nil {
		return fmt.Errorf("ffmpeg conversion failed: %w", err)
	}
	return nil
}
