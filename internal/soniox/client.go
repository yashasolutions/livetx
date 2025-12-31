package soniox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	SonioxEndpoint = "wss://stt-rt.soniox.com/transcribe-websocket"
)

// Client handles WebSocket communication with Soniox
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// NewClient creates a new Soniox client
func NewClient() *Client {
	return &Client{}
}

// Connect establishes a WebSocket connection to Soniox
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, SonioxEndpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Soniox: %w", err)
	}
	c.conn = conn
	return nil
}

// SendStart sends the start request to begin transcription
func (c *Client) SendStart(ctx context.Context, req StartRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal start request: %w", err)
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}

	return nil
}

// SendAudio sends PCM audio data to Soniox
func (c *Client) SendAudio(ctx context.Context, pcm []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	if err := c.conn.WriteMessage(websocket.BinaryMessage, pcm); err != nil {
		return fmt.Errorf("failed to send audio: %w", err)
	}

	return nil
}

// Finalize sends an empty frame to signal end of audio
func (c *Client) Finalize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	if err := c.conn.WriteMessage(websocket.BinaryMessage, []byte{}); err != nil {
		return fmt.Errorf("failed to send finalize: %w", err)
	}

	return nil
}

// ReadLoop reads responses from Soniox and sends transcript events to the channel
func (c *Client) ReadLoop(ctx context.Context, out chan<- TranscriptEvent) error {
	defer close(out)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		if conn == nil {
			return fmt.Errorf("not connected")
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return nil
			}
			return fmt.Errorf("failed to read message: %w", err)
		}

		var resp Response
		if err := json.Unmarshal(message, &resp); err != nil {
			continue // Skip malformed messages
		}

		if resp.ErrorCode != "" {
			return fmt.Errorf("soniox error %s: %s", resp.ErrorCode, resp.ErrorMessage)
		}

		if len(resp.Tokens) > 0 {
			event := processTokens(&resp)
			event.Raw = &resp

			select {
			case out <- event:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if resp.Finished {
			return nil
		}
	}
}

// processTokens converts tokens into a TranscriptEvent
func processTokens(resp *Response) TranscriptEvent {
	var textParts []string
	var isFinal bool
	var speaker int

	for _, token := range resp.Tokens {
		textParts = append(textParts, token.Text)
		if token.IsFinal {
			isFinal = true
		}
		if token.Speaker > 0 {
			speaker = token.Speaker
		}
	}

	return TranscriptEvent{
		Text:    strings.Join(textParts, ""),
		IsFinal: isFinal,
		Speaker: speaker,
	}
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}
