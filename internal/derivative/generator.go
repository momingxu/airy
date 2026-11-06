package derivative

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	ThumbnailDirectory = "thumbnails"
	ThumbnailHeight    = 400
)

type Generator struct {
	ffmpegPath string
	slots      chan struct{}
}

func NewGenerator(ffmpegPath string) *Generator {
	// Leave one core available for the API and database work, while bounding
	// concurrent ffmpeg processes so a large library cannot saturate the host.
	workers := runtime.GOMAXPROCS(0) - 1
	if workers < 1 {
		workers = 1
	}
	if workers > 4 {
		workers = 4
	}
	return &Generator{ffmpegPath: strings.TrimSpace(ffmpegPath), slots: make(chan struct{}, workers)}
}

func (g *Generator) Concurrency() int {
	if g == nil || cap(g.slots) < 1 {
		return 1
	}
	return cap(g.slots)
}

func IsGeneratedDirectory(name string) bool {
	return name == ThumbnailDirectory
}

func ThumbnailPath(albumPath, sourcePath string) (string, error) {
	relative, err := filepath.Rel(albumPath, sourcePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("media path is outside its album")
	}
	// Keep the original extension in the generated filename so foo.jpg and
	// foo.png cannot overwrite each other.
	generatedName := relative + ".webp"
	return filepath.Join(albumPath, ThumbnailDirectory, generatedName), nil
}

func (g *Generator) EnsureThumbnail(
	ctx context.Context,
	albumPath, sourcePath, mediaType string,
	modifiedAt time.Time,
) (string, error) {
	thumbnailPath, err := ThumbnailPath(albumPath, sourcePath)
	if err != nil {
		return "", err
	}
	if freshFile(thumbnailPath, modifiedAt) {
		return thumbnailPath, nil
	}
	if err := g.withSlot(ctx, func() error {
		return writeAtomically(thumbnailPath, modifiedAt, func(file *os.File) error {
			return g.runFFmpeg(ctx, sourcePath, file.Name(), fmt.Sprintf("scale=-1:%d", ThumbnailHeight), mediaType == "video", 80)
		})
	}); err != nil {
		return "", fmt.Errorf("thumbnail: %w", err)
	}
	return thumbnailPath, nil
}

func freshFile(path string, sourceModifiedAt time.Time) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0 && !info.ModTime().Before(sourceModifiedAt)
}

func (g *Generator) withSlot(ctx context.Context, generate func() error) error {
	if g.ffmpegPath == "" {
		return errors.New("ffmpeg is not configured")
	}
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	return generate()
}

func (g *Generator) runFFmpeg(ctx context.Context, sourcePath, outputPath, scale string, isVideo bool, quality int) error {
	arguments := []string{"-v", "error"}
	if isVideo {
		arguments = append(arguments, "-ss", "1")
	}
	arguments = append(arguments, "-i", sourcePath, "-frames:v", "1")
	if scale != "" {
		arguments = append(arguments, "-vf", scale)
	}
	arguments = append(arguments, "-c:v", "libwebp", "-quality", fmt.Sprintf("%d", quality), "-y", outputPath)
	command := exec.CommandContext(ctx, g.ffmpegPath, arguments...)
	output := &limitedProcessOutput{limit: 64 << 10}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return fmt.Errorf("run ffmpeg: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

type limitedProcessOutput struct {
	data      []byte
	limit     int
	truncated bool
}

func (output *limitedProcessOutput) Write(data []byte) (int, error) {
	written := len(data)
	remaining := output.limit - len(output.data)
	if remaining > 0 {
		if remaining > len(data) {
			remaining = len(data)
		}
		output.data = append(output.data, data[:remaining]...)
	}
	if remaining < len(data) {
		output.truncated = true
	}
	return written, nil
}

func (output *limitedProcessOutput) String() string {
	if !output.truncated {
		return string(output.data)
	}
	return string(output.data) + "\n[output truncated]"
}

func writeAtomically(outputPath string, modifiedAt time.Time, encode func(*os.File) error) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(outputPath), ".airy-*.webp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := encode(temp); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return err
	}
	if err := os.Chtimes(tempPath, modifiedAt, modifiedAt); err != nil {
		return err
	}
	return os.Rename(tempPath, outputPath)
}
