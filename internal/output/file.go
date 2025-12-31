package output

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/livetx/livetx/internal/soniox"
)

// FileSink appends final transcripts to a file with timestamps
type FileSink struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// NewFileSink creates a new file output sink
func NewFileSink(path string) (*FileSink, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open output file: %w", err)
	}

	return &FileSink{
		file: file,
		path: path,
	}, nil
}

// Write appends a final transcript event to the file
func (f *FileSink) Write(event soniox.TranscriptEvent) error {
	// Only write final transcripts
	if !event.IsFinal {
		return nil
	}

	text := strings.TrimSpace(event.Text)
	if text == "" {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s  %s\n", timestamp, text)

	if _, err := f.file.WriteString(line); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	// Flush to ensure data is written
	if err := f.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// Close closes the file
func (f *FileSink) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.file != nil {
		err := f.file.Close()
		f.file = nil
		return err
	}
	return nil
}

// Path returns the file path
func (f *FileSink) Path() string {
	return f.path
}
