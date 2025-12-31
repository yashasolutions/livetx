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
	// Try pactl first (works with both PulseAudio and PipeWire-pulse)
	cmd := exec.Command("pactl", "list", "short", "sources")
	output, err := cmd.Output()
	if err == nil {
		return parsePactlSources(string(output)), nil
	}

	// If pactl fails, try pw-cli
	if _, err := exec.LookPath("pw-cli"); err == nil {
		// pw-cli output is complex, return empty list with note
		return []Device{}, nil
	}

	return nil, fmt.Errorf("neither pactl nor pw-cli available: install pulseaudio-utils or pipewire")
}

func parsePactlSources(output string) []Device {
	var devices []Device
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			devices = append(devices, Device{
				ID:   fields[1],
				Name: fields[1],
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
			args = append([]string{"--target", cfg.DeviceID}, args...)
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
