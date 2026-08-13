package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/livetx/livetx/internal/output"
	"github.com/livetx/livetx/internal/soniox"
)

// FileConfig holds configuration for generating a WebVTT file from a media file.
type FileConfig struct {
	APIKey     string
	InputPath  string
	OutputFile string
	LangIn     string
	MaxChars   int
	GapMs      int64
}

// RunFileVTT transcribes an existing mp3/mp4 (or any ffmpeg-readable media) file
// via Soniox's async API and writes a WebVTT subtitle file. It blocks until the
// job completes, the context is cancelled, or an error occurs.
func RunFileVTT(ctx context.Context, cfg FileConfig) error {
	if err := ensureHasAudio(cfg.InputPath); err != nil {
		return err
	}

	client := soniox.NewAsyncClient(cfg.APIKey)

	fmt.Printf("Uploading %s...\n", cfg.InputPath)
	fileID, err := client.UploadFile(ctx, cfg.InputPath)
	if err != nil {
		return err
	}
	// Best-effort cleanup with a fresh context so it still runs on cancellation.
	defer func() { _ = client.DeleteFile(context.Background(), fileID) }()

	req := soniox.CreateTranscriptionRequest{
		Model:                        soniox.AsyncModel,
		FileID:                       fileID,
		EnableLanguageIdentification: true,
	}
	if cfg.LangIn != "" && cfg.LangIn != "auto" {
		req.LanguageHints = []string{cfg.LangIn}
	}

	fmt.Println("Transcribing...")
	id, err := client.CreateTranscription(ctx, req)
	if err != nil {
		return err
	}
	defer func() { _ = client.DeleteTranscription(context.Background(), id) }()

	if err := client.WaitForCompletion(ctx, id, 2*time.Second); err != nil {
		return err
	}

	tokens, err := client.GetTranscript(ctx, id)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(cfg.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open output file: %w", err)
	}
	defer f.Close()

	opts := output.VTTOptions{
		MaxChars: cfg.MaxChars,
		GapMs:    cfg.GapMs,
	}
	if err := output.BuildVTT(f, tokens, opts); err != nil {
		return fmt.Errorf("failed to write VTT: %w", err)
	}

	fmt.Printf("Wrote %s (%d tokens)\n", cfg.OutputFile, len(tokens))
	return nil
}

// ensureHasAudio fails fast if the input is missing or, when ffprobe is
// available, has no audio stream (e.g. a silent/video-only mp4).
func ensureHasAudio(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("input file not found: %w", err)
	}

	// ffprobe is optional; if it's not installed, let Soniox validate the file.
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return nil
	}

	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "a",
		"-show_entries", "stream=index",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		return fmt.Errorf("could not read input file %q: %w", path, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("input file %q has no audio stream", path)
	}
	return nil
}
