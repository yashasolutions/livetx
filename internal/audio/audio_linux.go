//go:build linux

package audio

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type linuxCapturer struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func newPlatformCapturer() AudioCapturer {
	return &linuxCapturer{}
}

func (c *linuxCapturer) ListDevices() ([]Device, error) {
	// Prefer the long-form listing: it carries friendly Descriptions and
	// tells us which sources are sink monitors (i.e. "what you hear").
	if out, err := exec.Command("pactl", "list", "sources").Output(); err == nil {
		if devices := parsePactlLongSources(string(out)); len(devices) > 0 {
			return devices, nil
		}
	}

	// Fall back to the short listing (IDs only, no friendly labels).
	if out, err := exec.Command("pactl", "list", "short", "sources").Output(); err == nil {
		return parsePactlShortSources(string(out)), nil
	}

	// If pactl fails, try pw-cli
	if _, err := exec.LookPath("pw-cli"); err == nil {
		// pw-cli output is complex, return empty list with note
		return []Device{}, nil
	}

	return nil, fmt.Errorf("neither pactl nor pw-cli available: install pulseaudio-utils or pipewire")
}

// parsePactlLongSources parses `pactl list sources` into devices with
// human-friendly labels that distinguish output monitors (what you hear)
// from microphone inputs.
func parsePactlLongSources(output string) []Device {
	var devices []Device
	var name, description string
	isMonitor := false

	flush := func() {
		if name == "" {
			return
		}
		devices = append(devices, Device{
			ID:   name,
			Name: friendlySourceLabel(name, description, isMonitor),
		})
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Source #"):
			flush()
			name, description, isMonitor = "", "", false
		case strings.HasPrefix(trimmed, "Name:"):
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "Name:"))
		case strings.HasPrefix(trimmed, "Description:"):
			description = strings.TrimSpace(strings.TrimPrefix(trimmed, "Description:"))
		case strings.HasPrefix(trimmed, "Monitor of Sink:"):
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "Monitor of Sink:"))
			if val != "" && val != "n/a" {
				isMonitor = true
			}
		}
	}
	flush()
	return devices
}

// friendlySourceLabel builds a display label with an emoji and a plain-English
// hint about whether the source captures output audio or a microphone.
func friendlySourceLabel(name, description string, isMonitor bool) string {
	label := description
	if label == "" {
		label = name
	}
	if isMonitor || strings.HasSuffix(name, ".monitor") {
		// e.g. "Monitor of EPOS IMPACT 60 Analog Stereo" -> "EPOS IMPACT 60 Analog Stereo"
		label = strings.TrimPrefix(label, "Monitor of ")
		return "🔊 " + label + " (output — what you hear)"
	}
	return "🎙 " + label + " (microphone)"
}

func parsePactlShortSources(output string) []Device {
	var devices []Device
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := fields[1]
			devices = append(devices, Device{
				ID:   name,
				Name: friendlySourceLabel(name, "", strings.HasSuffix(name, ".monitor")),
			})
		}
	}
	return devices
}

func (c *linuxCapturer) Start(cfg CaptureConfig, audioCh chan<- []byte) error {
	c.mu.Lock()

	var cmd *exec.Cmd
	sampleRate := fmt.Sprintf("%d", cfg.SampleRate)
	channels := fmt.Sprintf("%d", cfg.Channels)

	// Prefer pw-record if available
	if _, err := exec.LookPath("pw-record"); err == nil {
		args := []string{
			"--format", "s16",
			"--rate", sampleRate,
			"--channels", channels,
			"-",
		}
		if cfg.DeviceID != "" {
			// Device IDs come from `pactl` as PulseAudio source names, but
			// pw-record's --target matches PipeWire node names. A ".monitor"
			// source (what an output sink plays) has no PipeWire node of that
			// name, so pw-record would silently fall back to the default
			// source (the mic). Target the underlying sink node instead and
			// tell pw-record to capture the sink's monitor.
			target := cfg.DeviceID
			if strings.HasSuffix(target, ".monitor") {
				target = strings.TrimSuffix(target, ".monitor")
				args = append([]string{"-P", "stream.capture.sink=true"}, args...)
			}
			args = append([]string{"--target", target}, args...)
		}
		cmd = exec.Command("pw-record", args...)
	} else if _, err := exec.LookPath("parec"); err == nil {
		args := []string{
			"--format=s16le",
			"--rate=" + sampleRate,
			"--channels=" + channels,
		}
		if cfg.DeviceID != "" {
			args = append(args, "-d", cfg.DeviceID)
		}
		cmd = exec.Command("parec", args...)
	} else {
		c.mu.Unlock()
		return fmt.Errorf("neither pw-record nor parec found: install pipewire or pulseaudio-utils")
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	c.cmd = cmd
	c.stdout = stdout
	c.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start audio capture: %w", err)
	}

	// Read audio data in chunks (~20ms at 48kHz mono s16le = 1920 bytes)
	// 48000 samples/sec * 2 bytes/sample * 0.02 sec = 1920 bytes
	chunkSize := cfg.SampleRate * 2 * cfg.Channels * 20 / 1000
	if chunkSize < 960 {
		chunkSize = 960
	}

	go func() {
		defer close(audioCh)
		buf := make([]byte, chunkSize)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					// Log error but don't block
				}
				return
			}
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				audioCh <- data
			}
		}
	}()

	return cmd.Wait()
}

func (c *linuxCapturer) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
