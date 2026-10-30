package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
