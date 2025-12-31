//go:build darwin

package audio

import "fmt"

type darwinCapturer struct{}

func newPlatformCapturer() AudioCapturer {
	return &darwinCapturer{}
}

func (c *darwinCapturer) ListDevices() ([]Device, error) {
	return []Device{}, nil
}

func (c *darwinCapturer) Start(cfg CaptureConfig, audioCh chan<- []byte) error {
	return fmt.Errorf("loopback capture not implemented on this platform yet")
}

func (c *darwinCapturer) Stop() error {
	return nil
}
