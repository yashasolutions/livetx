// Command livetx-web serves the LiveTX UI as a local web app and opens it in a
// Chrome "app mode" window (chromeless, looks native). It is pure Go — no cgo —
// so it cross-compiles to macOS/Windows/Linux from any host without a toolchain.
//
// It reuses the same internal/app core as the CLI and Fyne GUI; transcripts are
// streamed to the browser over a WebSocket.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/livetx/livetx/internal/app"
	"github.com/livetx/livetx/internal/soniox"
)

// Compiled, self-contained UI assets. app.css is built from the gsxui/Tailwind
// sources (see `make web-assets`); app.js drives the WebSocket UI.
//
//go:embed web/app.css web/app.js
var webFS embed.FS

func main() {
	godotenv.Load()

	apiKey := os.Getenv("SONIOX_API_KEY")
	if apiKey == "" {
		log.Fatal("SONIOX_API_KEY not set. Put it in .env or the environment.")
	}

	// Listen on a random free port on loopback only — this is a local desktop UI.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	url := fmt.Sprintf("http://%s", ln.Addr().String())

	srv := newServer(apiKey)
	httpSrv := &http.Server{Handler: srv.routes()}

	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	log.Printf("LiveTX web UI at %s", url)

	// Open a chromeless Chrome window pointed at the server. If Chrome isn't
	// found, fall back to the default browser (a normal tab).
	browserDone := openUI(url)

	// Quit when the browser window closes (desktop-app feel), giving an active
	// transcription a moment to finalize.
	<-browserDone
	log.Println("UI closed, shutting down.")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}

// ---- server ----------------------------------------------------------------

type server struct {
	apiKey   string
	upgrader websocket.Upgrader
}

func newServer(apiKey string) *server {
	return &server{apiKey: apiKey, upgrader: websocket.Upgrader{}}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/devices", s.handleDevices)
	mux.HandleFunc("/ws", s.handleWS)
	// Serve the compiled CSS/JS from the embedded web/ directory.
	assets, _ := fs.Sub(webFS, "web")
	mux.Handle("/app.css", http.FileServer(http.FS(assets)))
	mux.Handle("/app.js", http.FileServer(http.FS(assets)))
	return mux
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	devices, err := app.ListDevices()
	if err != nil {
		log.Printf("device list error: %v", err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<!doctype html>\n")
	if err := Page(devices).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (s *server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := app.ListDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(devices)
}

// ---- websocket session -----------------------------------------------------

// clientCmd is a message from the browser.
type clientCmd struct {
	Action  string `json:"action"` // "start" | "stop"
	Device  string `json:"device"`
	LangIn  string `json:"langIn"`
	LangOut string `json:"langOut"`
}

// wsMessage is a message pushed to the browser.
type wsMessage struct {
	Type        string `json:"type"` // "transcript" | "status" | "error"
	Text        string `json:"text,omitempty"`
	Final       bool   `json:"final,omitempty"`
	Speaker     int    `json:"speaker,omitempty"`
	Translation bool   `json:"translation,omitempty"` // true if this text is translated (vs. source)
}

func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// gorilla requires a single concurrent writer; serialize all sends.
	var writeMu sync.Mutex
	send := func(m wsMessage) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.WriteJSON(m)
	}

	var (
		mu     sync.Mutex
		cancel context.CancelFunc
	)
	stop := func() {
		mu.Lock()
		defer mu.Unlock()
		if cancel != nil {
			cancel()
			cancel = nil
		}
	}
	defer stop()

	for {
		var cmd clientCmd
		if err := conn.ReadJSON(&cmd); err != nil {
			return // client disconnected
		}
		switch cmd.Action {
		case "start":
			stop() // ensure only one session at a time
			ctx, c := context.WithCancel(r.Context())
			mu.Lock()
			cancel = c
			mu.Unlock()
			go s.runSession(ctx, cmd, send)
			send(wsMessage{Type: "status", Text: "started"})
		case "stop":
			stop()
			send(wsMessage{Type: "status", Text: "stopped"})
		}
	}
}

func (s *server) runSession(ctx context.Context, cmd clientCmd, send func(wsMessage)) {
	langOut := cmd.LangOut
	if langOut == "" {
		langOut = "en"
	}
	langIn := cmd.LangIn
	if langIn == "" {
		langIn = "auto"
	}

	cfg := app.Config{
		APIKey:     s.apiKey,
		DeviceID:   cmd.Device,
		OutputFile: fmt.Sprintf("transcript-%s.txt", time.Now().Format("20060102-150405")),
		LangIn:     langIn,
		LangOut:    langOut,
		SampleRate: 48000,
		Channels:   1,
		OnTranscript: func(ev soniox.TranscriptEvent) {
			// Detect translated text the same way the Fyne GUI does: a token whose
			// source language differs from its language is a translation.
			translation := false
			if ev.Raw != nil {
				for _, token := range ev.Raw.Tokens {
					if token.SourceLanguage != "" && token.SourceLanguage != token.Language {
						translation = true
						break
					}
				}
			}
			send(wsMessage{
				Type:        "transcript",
				Text:        ev.Text,
				Final:       ev.IsFinal,
				Speaker:     ev.Speaker,
				Translation: translation,
			})
		},
	}

	application, err := app.New(cfg)
	if err != nil {
		send(wsMessage{Type: "error", Text: err.Error()})
		return
	}
	defer application.Close()

	if err := application.Run(ctx); err != nil && ctx.Err() == nil {
		send(wsMessage{Type: "error", Text: err.Error()})
	}
}

// ---- browser launch --------------------------------------------------------

// openUI opens the URL in a chromeless Chrome window if Chrome/Chromium/Edge is
// available, otherwise in the default browser. The returned channel is closed
// when the launched Chrome window exits (nil/never-closing for the fallback).
func openUI(url string) <-chan struct{} {
	done := make(chan struct{})

	if bin := findChrome(); bin != "" {
		profile, _ := os.MkdirTemp("", "livetx-chrome-")
		cmd := exec.Command(bin,
			"--app="+url,
			"--user-data-dir="+profile,
			"--no-first-run",
			"--no-default-browser-check",
			"--window-size=900,700",
			// This is a throwaway local app window: silence Chrome's startup
			// update check and other background networking the user can't act on.
			"--disable-background-networking",
			"--disable-component-update",
			"--disable-default-apps",
		)
		if err := cmd.Start(); err == nil {
			go func() {
				cmd.Wait()
				os.RemoveAll(profile)
				close(done)
			}()
			return done
		}
	}

	// Fallback: default browser (a normal tab). We can't detect when it closes,
	// so this channel stays open and the process runs until interrupted.
	log.Printf("Chrome not found; opening default browser. Open %s manually if needed.", url)
	openDefault(url)
	return done
}

// findChrome returns the path to a Chromium-family browser, or "".
func findChrome() string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		}
	default: // linux and others
		for _, name := range []string{
			"google-chrome", "google-chrome-stable", "chromium",
			"chromium-browser", "microsoft-edge", "brave-browser",
		} {
			if p, err := exec.LookPath(name); err == nil {
				return p
			}
		}
		return ""
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// openDefault opens a URL in the OS default browser.
func openDefault(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
