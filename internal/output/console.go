package output

import (
	"fmt"
	"strings"
	"sync"

	"github.com/livetx/livetx/internal/soniox"
)

// ConsoleSink outputs transcript events to the console
type ConsoleSink struct {
	mu            sync.Mutex
	lastPartial   string
	lastWasPartial bool
}

// NewConsoleSink creates a new console output sink
func NewConsoleSink() *ConsoleSink {
	return &ConsoleSink{}
}

// Write outputs a transcript event to the console
func (c *ConsoleSink) Write(event soniox.TranscriptEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	text := strings.TrimSpace(event.Text)
	if text == "" {
		return
	}

	if event.IsFinal {
		// Clear partial line if there was one
		if c.lastWasPartial {
			// Move to beginning of line and clear
			fmt.Print("\r\033[K")
		}
		fmt.Println(text)
		c.lastWasPartial = false
		c.lastPartial = ""
	} else {
		// Partial transcript - print with prefix, overwriting previous partial
		if c.lastWasPartial {
			fmt.Print("\r\033[K")
		}
		fmt.Printf("~ %s", text)
		c.lastWasPartial = true
		c.lastPartial = text
	}
}

// Close performs any cleanup needed
func (c *ConsoleSink) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure we end on a new line
	if c.lastWasPartial {
		fmt.Println()
	}
	return nil
}
