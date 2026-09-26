package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/livetx/livetx/internal/audio"
	"github.com/livetx/livetx/internal/output"
	"github.com/livetx/livetx/internal/soniox"
)

// Config holds the application configuration
type Config struct {
	APIKey     string
	DeviceID   string
	OutputFile string
	LangIn     string
	LangOut    string
	SampleRate int
	Channels   int

	// OnTranscript, if set, is called for every transcript event in addition to
	// the console and file sinks. Used by alternative frontends (e.g. the web
	// UI) to stream transcripts to a client. It must not block for long.
	OnTranscript func(soniox.TranscriptEvent)
}

// App orchestrates the audio capture and transcription
type App struct {
	config      Config
	capturer    audio.AudioCapturer
	client      *soniox.Client
	consoleSink *output.ConsoleSink
	fileSink    *output.FileSink
}

// New creates a new App instance
func New(cfg Config) (*App, error) {
	fileSink, err := output.NewFileSink(cfg.OutputFile)
	if err != nil {
		return nil, err
	}

	return &App{
		config:      cfg,
		capturer:    audio.NewCapturer(),
		client:      soniox.NewClient(),
		consoleSink: output.NewConsoleSink(),
		fileSink:    fileSink,
	}, nil
}

// Run starts the application and blocks until context is cancelled
func (a *App) Run(ctx context.Context) error {
	// Connect to Soniox
	if err := a.client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Soniox: %w", err)
	}
	defer a.client.Close()

	// Build start request
	startReq := soniox.StartRequest{
		APIKey:                       a.config.APIKey,
		Model:                        "stt-rt-preview",
		AudioFormat:                  "s16le",
		NumChannels:                  a.config.Channels,
		SampleRate:                   a.config.SampleRate,
		EnableLanguageIdentification: true,
	}

	// Set language hints if not auto
	if a.config.LangIn != "" && a.config.LangIn != "auto" {
		startReq.LanguageHints = []string{a.config.LangIn}
	}

	// Set translation if target language is specified
	if a.config.LangOut != "" {
		startReq.Translation = &soniox.Translation{
			Type:           "one_way",
			TargetLanguage: a.config.LangOut,
		}
	}

	// Send start request
	if err := a.client.SendStart(ctx, startReq); err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}

	// Create channels
	audioCh := make(chan []byte, 100)
	transcriptCh := make(chan soniox.TranscriptEvent, 100)

	// Error collection
	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	// Create a cancellable context for goroutines
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start audio capture goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		cfg := audio.CaptureConfig{
			DeviceID:   a.config.DeviceID,
			SampleRate: a.config.SampleRate,
			Channels:   a.config.Channels,
		}
		if err := a.capturer.Start(cfg, audioCh); err != nil {
			select {
			case errCh <- fmt.Errorf("audio capture error: %w", err):
			default:
			}
		}
	}()

	// Start audio sending goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-runCtx.Done():
				return
			case data, ok := <-audioCh:
				if !ok {
					return
				}
				if err := a.client.SendAudio(runCtx, data); err != nil {
					select {
					case errCh <- fmt.Errorf("send audio error: %w", err):
					default:
					}
					return
				}
			}
		}
	}()

	// Start transcript reading goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.client.ReadLoop(runCtx, transcriptCh); err != nil {
			select {
			case errCh <- fmt.Errorf("read loop error: %w", err):
			default:
			}
		}
	}()

	// Start transcript processing goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-runCtx.Done():
				return
			case event, ok := <-transcriptCh:
				if !ok {
					return
				}
				a.consoleSink.Write(event)
				if a.config.OnTranscript != nil {
					a.config.OnTranscript(event)
				}
				if err := a.fileSink.Write(event); err != nil {
					select {
					case errCh <- fmt.Errorf("file write error: %w", err):
					default:
					}
				}
			}
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		fmt.Println("\nShutting down...")
	case err := <-errCh:
		cancel()
		return err
	}

	// Stop audio capture
	if err := a.capturer.Stop(); err != nil {
		// Log but don't fail
	}

	// Send finalize to Soniox
	finalizeCtx, finalizeCancel := context.WithTimeout(context.Background(), 5*1e9)
	defer finalizeCancel()
	a.client.Finalize(finalizeCtx)

	// Cancel running goroutines
	cancel()

	// Wait for goroutines to finish
	wg.Wait()

	return nil
}

// Close cleans up resources
func (a *App) Close() error {
	a.consoleSink.Close()
	return a.fileSink.Close()
}

// ListDevices returns available audio devices
func ListDevices() ([]audio.Device, error) {
	capturer := audio.NewCapturer()
	return capturer.ListDevices()
}
