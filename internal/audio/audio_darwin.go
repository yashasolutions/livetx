//go:build darwin

package audio

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// darwinCapturer captures audio on macOS by shelling out to ffmpeg's
// avfoundation input, mirroring the Linux capturer's approach of driving a
// system CLI tool rather than binding native frameworks.
//
// macOS has no built-in system-audio ("what you hear") loopback. To transcribe
// output audio, install a virtual loopback device such as BlackHole
// (`brew install blackhole-2ch`), route system output through it, and select it
// as the capture device. Microphone capture works out of the box.
type darwinCapturer struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	stdout io.ReadCloser
}

func newPlatformCapturer() AudioCapturer {
	return &darwinCapturer{}
}

// audioDeviceLine matches avfoundation's device listing, e.g.
// "[AVFoundation indev @ 0x...] [1] BlackHole 2ch"
var audioDeviceLine = regexp.MustCompile(`^\[AVFoundation[^\]]*\]\s*\[(\d+)\]\s+(.*)$`)

func (c *darwinCapturer) ListDevices() ([]Device, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found: install it with `brew install ffmpeg`")
	}

	// avfoundation prints the device list to stderr and exits non-zero because
	// no output file is given; that's expected, so we ignore the error and
	// parse whatever it wrote.
	cmd := exec.Command("ffmpeg", "-hide_banner", "-f", "avfoundation",
		"-list_devices", "true", "-i", "")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()

	return parseAVFoundationDevices(stderr.String()), nil
}

// parseAVFoundationDevices extracts the audio devices from `ffmpeg ... -list_devices`
// output. Devices are listed under an "AVFoundation audio devices:" header,
// preceded by a "video devices:" section we must skip.
func parseAVFoundationDevices(output string) []Device {
	var devices []Device
	inAudio := false

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.Contains(line, "AVFoundation video devices:"):
			inAudio = false
			continue
		case strings.Contains(line, "AVFoundation audio devices:"):
			inAudio = true
			continue
		}
		if !inAudio {
			continue
		}
		m := audioDeviceLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		index, name := m[1], strings.TrimSpace(m[2])
		devices = append(devices, Device{
			ID:   index,
			Name: friendlyDarwinLabel(index, name),
		})
	}
	return devices
}

// friendlyDarwinLabel adds an emoji hint. A BlackHole/Soundflower/loopback
// device is the macOS route for capturing output audio ("what you hear").
func friendlyDarwinLabel(index, name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "blackhole") ||
		strings.Contains(lower, "soundflower") ||
		strings.Contains(lower, "loopback") ||
		strings.Contains(lower, "aggregate") {
		return "🔊 " + name + " (output loopback — what you hear)"
	}
	return "🎙 " + name + " (microphone)"
}

func (c *darwinCapturer) Start(cfg CaptureConfig, audioCh chan<- []byte) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: install it with `brew install ffmpeg`")
	}

	c.mu.Lock()

	// avfoundation input is "[[VIDEO]:[AUDIO]]"; an empty video field plus an
	// audio index/name captures audio only. Fall back to the default input.
	audioSpec := cfg.DeviceID
	if audioSpec == "" {
		audioSpec = "default"
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-f", "avfoundation",
		"-i", ":" + audioSpec,
		"-ar", fmt.Sprintf("%d", cfg.SampleRate),
		"-ac", fmt.Sprintf("%d", cfg.Channels),
		"-f", "s16le",
		"-",
	}
	cmd := exec.Command("ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Keep ffmpeg's stderr so we can surface a useful message on failure.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	c.cmd = cmd
	c.stdout = stdout
	c.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start audio capture: %w", err)
	}

	// Read audio data in chunks (~20ms). Matches the Linux capturer sizing.
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
				return
			}
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				audioCh <- data
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("ffmpeg capture failed: %s", msg)
		}
		return fmt.Errorf("ffmpeg capture failed: %w", err)
	}
	return nil
}

func (c *darwinCapturer) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
