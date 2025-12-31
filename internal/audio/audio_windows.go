//go:build windows

package audio

import "fmt"

type windowsCapturer struct{}

func newPlatformCapturer() AudioCapturer {
	return &windowsCapturer{}
}

func (c *windowsCapturer) ListDevices() ([]Device, error) {
	return []Device{}, nil
}

func (c *windowsCapturer) Start(cfg CaptureConfig, audioCh chan<- []byte) error {
	return fmt.Errorf("loopback capture not implemented on this platform yet")
}

func (c *windowsCapturer) Stop() error {
	return nil
}
