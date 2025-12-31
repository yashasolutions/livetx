package gui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/livetx/livetx/internal/audio"
	"github.com/livetx/livetx/internal/output"
	"github.com/livetx/livetx/internal/soniox"
)

type GUI struct {
	app    fyne.App
	window fyne.Window

	deviceSelect    *widget.Select
	langInEntry     *widget.Entry
	langOutEntry    *widget.Entry
	startButton     *widget.Button
	stopButton      *widget.Button
	statusLabel     *widget.Label
	transcriptText  *widget.RichText
	scrollContainer *container.Scroll

	mu             sync.Mutex
	devices        []audio.Device
	selectedDevice string
	apiKey         string
	isRunning      bool
	cancel         context.CancelFunc
	capturer       audio.AudioCapturer
	client         *soniox.Client
	fileSink       *output.FileSink
}

func New(apiKey string) *GUI {
	g := &GUI{
		apiKey:   apiKey,
		capturer: audio.NewCapturer(),
	}
	return g
}

// Custom theme for transcript text
type transcriptTheme struct {
	fyne.Theme
}

func (t *transcriptTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameForeground:
		return color.RGBA{R: 0x1a, G: 0x43, B: 0x78, A: 0xff} // #1a4378
	case theme.ColorNameBackground:
		return color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // white
	case theme.ColorNameInputBackground:
		return color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // white
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (g *GUI) Run() {
	g.app = app.New()
	g.window = g.app.NewWindow("LiveTX - Live Transcription")

	g.setupUI()
	g.loadDevices()

	g.window.Resize(fyne.NewSize(800, 600))
	g.window.ShowAndRun()
}

func (g *GUI) setupUI() {
	g.deviceSelect = widget.NewSelect([]string{}, func(selected string) {
		g.mu.Lock()
		g.selectedDevice = selected
		g.mu.Unlock()
	})
	g.deviceSelect.PlaceHolder = "Select audio device..."

	refreshDevicesBtn := widget.NewButton("Refresh", func() {
		g.loadDevices()
	})

	deviceRow := container.NewBorder(nil, nil, nil, refreshDevicesBtn, g.deviceSelect)

	g.langInEntry = widget.NewEntry()
	g.langInEntry.SetPlaceHolder("auto")
	g.langInEntry.SetText("auto")

	g.langOutEntry = widget.NewEntry()
	g.langOutEntry.SetPlaceHolder("en")
	g.langOutEntry.SetText("en")

	langRow := container.NewGridWithColumns(4,
		widget.NewLabel("Input Language:"),
		g.langInEntry,
		widget.NewLabel("Output Language:"),
		g.langOutEntry,
	)

	g.startButton = widget.NewButton("Start Transcription", g.onStart)
	g.stopButton = widget.NewButton("Stop", g.onStop)
	g.stopButton.Disable()

	controlRow := container.NewHBox(g.startButton, g.stopButton)

	g.statusLabel = widget.NewLabel("Ready")

	g.transcriptText = widget.NewRichText()
	g.transcriptText.Wrapping = fyne.TextWrapWord

	g.scrollContainer = container.NewVScroll(g.transcriptText)
	g.scrollContainer.SetMinSize(fyne.NewSize(780, 400))
	
	g.scrollContainer = container.NewVScroll(g.transcriptText)
	g.scrollContainer.SetMinSize(fyne.NewSize(780, 400))

	topSection := container.NewVBox(
		widget.NewLabel("Audio Device:"),
		deviceRow,
		widget.NewSeparator(),
		langRow,
		widget.NewSeparator(),
		controlRow,
		g.statusLabel,
		widget.NewSeparator(),
	)

	content := container.NewBorder(topSection, nil, nil, nil, g.scrollContainer)

	g.window.SetContent(content)
}

func (g *GUI) loadDevices() {
	devices, err := g.capturer.ListDevices()
	if err != nil {
		g.setStatus(fmt.Sprintf("Error loading devices: %v", err))
		return
	}

	g.mu.Lock()
	g.devices = devices
	g.mu.Unlock()

	var deviceNames []string
	for _, d := range devices {
		deviceNames = append(deviceNames, d.ID)
	}

	fyne.Do(func() {
		g.deviceSelect.Options = deviceNames
		g.deviceSelect.Refresh()
	})

	if len(devices) == 0 {
		g.setStatus("No audio devices found. On Linux, ensure PulseAudio or PipeWire is running.")
	} else {
		g.setStatus(fmt.Sprintf("Found %d audio device(s)", len(devices)))
	}
}

func (g *GUI) onStart() {
	g.mu.Lock()
	if g.isRunning {
		g.mu.Unlock()
		return
	}
	selectedDevice := g.selectedDevice
	g.mu.Unlock()

	langIn := strings.TrimSpace(g.langInEntry.Text)
	if langIn == "" {
		langIn = "auto"
	}
	langOut := strings.TrimSpace(g.langOutEntry.Text)
	if langOut == "" {
		langOut = "en"
	}

	outputFile := fmt.Sprintf("transcript-%s.txt", time.Now().Format("20060102-150405"))
	fileSink, err := output.NewFileSink(outputFile)
	if err != nil {
		g.setStatus(fmt.Sprintf("Error creating output file: %v", err))
		return
	}

	g.mu.Lock()
	g.fileSink = fileSink
	g.isRunning = true
	g.mu.Unlock()

	fyne.Do(func() {
		g.startButton.Disable()
		g.stopButton.Enable()
		g.deviceSelect.Disable()
		g.langInEntry.Disable()
		g.langOutEntry.Disable()
	})

	g.setStatus(fmt.Sprintf("Starting transcription... Output: %s", outputFile))
	g.clearTranscript()

	ctx, cancel := context.WithCancel(context.Background())
	g.mu.Lock()
	g.cancel = cancel
	g.mu.Unlock()

	go g.runTranscription(ctx, selectedDevice, langIn, langOut)
}

func (g *GUI) onStop() {
	g.mu.Lock()
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()
}

func (g *GUI) runTranscription(ctx context.Context, deviceID, langIn, langOut string) {
	defer func() {
		g.mu.Lock()
		g.isRunning = false
		if g.fileSink != nil {
			g.fileSink.Close()
			g.fileSink = nil
		}
		g.mu.Unlock()

		fyne.Do(func() {
			g.startButton.Enable()
			g.stopButton.Disable()
			g.deviceSelect.Enable()
			g.langInEntry.Enable()
			g.langOutEntry.Enable()
		})
	}()

	client := soniox.NewClient()
	g.mu.Lock()
	g.client = client
	g.mu.Unlock()

	if err := client.Connect(ctx); err != nil {
		g.setStatus(fmt.Sprintf("Connection error: %v", err))
		return
	}
	defer client.Close()

	startReq := soniox.StartRequest{
		APIKey:                       g.apiKey,
		Model:                        "stt-rt-preview",
		AudioFormat:                  "s16le",
		NumChannels:                  1,
		SampleRate:                   48000,
		EnableLanguageIdentification: true,
	}

	if langIn != "" && langIn != "auto" {
		startReq.LanguageHints = []string{langIn}
	}

	if langOut != "" {
		startReq.Translation = &soniox.Translation{
			Type:           "one_way",
			TargetLanguage: langOut,
		}
	}

	if err := client.SendStart(ctx, startReq); err != nil {
		g.setStatus(fmt.Sprintf("Start error: %v", err))
		return
	}

	g.setStatus("Transcribing...")

	audioCh := make(chan []byte, 100)
	transcriptCh := make(chan soniox.TranscriptEvent, 100)

	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		cfg := audio.CaptureConfig{
			DeviceID:   deviceID,
			SampleRate: 48000,
			Channels:   1,
		}
		if err := g.capturer.Start(cfg, audioCh); err != nil {
			select {
			case errCh <- fmt.Errorf("audio capture error: %w", err):
			default:
			}
		}
	}()

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
				if err := client.SendAudio(runCtx, data); err != nil {
					select {
					case errCh <- fmt.Errorf("send audio error: %w", err):
					default:
					}
					return
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := client.ReadLoop(runCtx, transcriptCh); err != nil {
			select {
			case errCh <- fmt.Errorf("read loop error: %w", err):
			default:
			}
		}
	}()

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
				g.appendTranscript(event)
				g.mu.Lock()
				if g.fileSink != nil {
					g.fileSink.Write(event)
				}
				g.mu.Unlock()
			}
		}
	}()

	select {
	case <-ctx.Done():
		g.setStatus("Stopping...")
	case err := <-errCh:
		g.setStatus(fmt.Sprintf("Error: %v", err))
		runCancel()
	}

	g.capturer.Stop()

	finalizeCtx, finalizeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalizeCancel()
	client.Finalize(finalizeCtx)

	runCancel()
	wg.Wait()

	g.setStatus("Stopped")
}

func (g *GUI) setStatus(status string) {
	fyne.Do(func() {
		g.statusLabel.SetText(status)
	})
}

func (g *GUI) clearTranscript() {
	fyne.Do(func() {
		g.transcriptText.Segments = []widget.RichTextSegment{}
		g.transcriptText.Refresh()
	})
}

func (g *GUI) appendTranscript(event soniox.TranscriptEvent) {
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return
	}

	if event.IsFinal {
		fyne.Do(func() {
			timestamp := time.Now().Format("15:04:05")
			newText := fmt.Sprintf("[%s] %s\n", timestamp, text)
			
			// Determine if this is translated text by checking if we have source language info
			isTranslated := false
			if event.Raw != nil && len(event.Raw.Tokens) > 0 {
				for _, token := range event.Raw.Tokens {
					if token.SourceLanguage != "" && token.SourceLanguage != token.Language {
						isTranslated = true
						break
					}
				}
			}
			
			// Create a text segment with appropriate styling
			var segment widget.RichTextSegment
			if isTranslated {
				// Orange/amber color for translated text
				segment = &widget.TextSegment{
					Text: newText,
					Style: widget.RichTextStyle{
						ColorName: theme.ColorNameWarning, // Orange/amber for translations
						TextStyle: fyne.TextStyle{Italic: true},
					},
				}
			} else {
				// Blue color for source text
				segment = &widget.TextSegment{
					Text: newText,
					Style: widget.RichTextStyle{
						ColorName: theme.ColorNamePrimary, // Blue for source text
						TextStyle: fyne.TextStyle{},
					},
				}
			}
			
			g.transcriptText.Segments = append(g.transcriptText.Segments, segment)
			g.transcriptText.Refresh()
			g.scrollContainer.ScrollToBottom()
		})
	}
}
