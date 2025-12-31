package audio

// Device represents an audio capture device
type Device struct {
	ID   string
	Name string
}

// CaptureConfig holds configuration for audio capture
type CaptureConfig struct {
	DeviceID   string
	SampleRate int
	Channels   int
}

// AudioCapturer defines the interface for audio capture implementations
type AudioCapturer interface {
	// ListDevices returns available audio capture devices
	ListDevices() ([]Device, error)

	// Start begins audio capture and sends PCM data to the provided channel.
	// It blocks until the context is cancelled or an error occurs.
	// The channel will be closed when capture stops.
	Start(cfg CaptureConfig, audioCh chan<- []byte) error

	// Stop stops the audio capture
	Stop() error
}

// NewCapturer returns an OS-appropriate AudioCapturer implementation
func NewCapturer() AudioCapturer {
	return newPlatformCapturer()
}
