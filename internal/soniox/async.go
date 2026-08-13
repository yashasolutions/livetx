package soniox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// AsyncBaseURL is the base URL for Soniox's async (batch) file transcription REST API.
const AsyncBaseURL = "https://api.soniox.com"

// AsyncModel is the model used for asynchronous file transcription.
const AsyncModel = "stt-async-v5"

// AsyncClient talks to Soniox's async file-transcription REST API. Unlike the
// real-time WebSocket Client, it uploads a whole media file, waits for the job
// to finish, and returns tokens with word-level timestamps in one shot.
type AsyncClient struct {
	apiKey  string
	http    *http.Client
	baseURL string
}

// NewAsyncClient creates a new async REST client for the given API key.
func NewAsyncClient(apiKey string) *AsyncClient {
	return &AsyncClient{
		apiKey: apiKey,
		// Generous per-request timeout: it only needs to cover a single
		// upload/download; polling is done as separate short requests.
		http:    &http.Client{Timeout: 10 * time.Minute},
		baseURL: AsyncBaseURL,
	}
}

// CreateTranscriptionRequest is the JSON body for POST /v1/transcriptions.
type CreateTranscriptionRequest struct {
	Model                        string   `json:"model"`
	FileID                       string   `json:"file_id"`
	LanguageHints                []string `json:"language_hints,omitempty"`
	EnableLanguageIdentification bool     `json:"enable_language_identification"`
}

// transcriptionStatus is the JSON returned by GET /v1/transcriptions/{id}.
type transcriptionStatus struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// transcriptResult is the JSON returned by GET /v1/transcriptions/{id}/transcript.
type transcriptResult struct {
	Tokens []Token `json:"tokens"`
}

// UploadFile uploads a local media file and returns its server-side file ID.
func (c *AsyncClient) UploadFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open input file: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", fmt.Errorf("failed to build upload request: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", fmt.Errorf("failed to read input file: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize upload request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/files", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.authorize(req)

	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(req, &out); err != nil {
		return "", fmt.Errorf("file upload failed: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("file upload returned no id")
	}
	return out.ID, nil
}

// CreateTranscription starts a transcription job and returns its ID.
func (c *AsyncClient) CreateTranscription(ctx context.Context, tr CreateTranscriptionRequest) (string, error) {
	data, err := json.Marshal(tr)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/transcriptions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(req, &out); err != nil {
		return "", fmt.Errorf("create transcription failed: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("create transcription returned no id")
	}
	return out.ID, nil
}

// WaitForCompletion polls the transcription until it completes or errors.
// Transient failures (gateway 5xx, rate limits, network blips) are tolerated:
// the job keeps running server-side, so we retry a bounded number of
// consecutive times rather than aborting.
func (c *AsyncClient) WaitForCompletion(ctx context.Context, id string, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	const maxConsecutiveFailures = 15
	failures := 0

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		st, err := c.getStatus(ctx, id)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !isRetryable(err) {
				return fmt.Errorf("poll transcription failed: %w", err)
			}
			failures++
			if failures >= maxConsecutiveFailures {
				return fmt.Errorf("poll transcription failed after %d retries: %w", failures, err)
			}
			// Transient (e.g. 502 Bad Gateway) — back off and retry.
			if !sleepCtx(ctx, pollInterval) {
				return ctx.Err()
			}
			continue
		}
		failures = 0

		switch st.Status {
		case "completed":
			return nil
		case "error":
			if st.ErrorMessage != "" {
				return fmt.Errorf("transcription failed: %s", st.ErrorMessage)
			}
			return fmt.Errorf("transcription failed")
		}

		if !sleepCtx(ctx, pollInterval) {
			return ctx.Err()
		}
	}
}

func (c *AsyncClient) getStatus(ctx context.Context, id string) (transcriptionStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/transcriptions/"+id, nil)
	if err != nil {
		return transcriptionStatus{}, err
	}
	c.authorize(req)

	var st transcriptionStatus
	if err := c.do(req, &st); err != nil {
		return transcriptionStatus{}, err
	}
	return st, nil
}

// GetTranscript fetches the completed transcript's tokens, retrying transient
// errors so a momentary gateway blip doesn't discard a finished transcription.
func (c *AsyncClient) GetTranscript(ctx context.Context, id string) ([]Token, error) {
	url := c.baseURL + "/v1/transcriptions/" + id + "/transcript"
	const maxAttempts = 5
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 && !sleepCtx(ctx, 2*time.Second) {
			return nil, ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		c.authorize(req)

		var res transcriptResult
		err = c.do(req, &res)
		if err == nil {
			return res.Tokens, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !isRetryable(err) {
			return nil, fmt.Errorf("get transcript failed: %w", err)
		}
		lastErr = err
	}
	return nil, fmt.Errorf("get transcript failed after retries: %w", lastErr)
}

// DeleteTranscription removes a transcription job (best-effort cleanup).
func (c *AsyncClient) DeleteTranscription(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/transcriptions/"+id)
}

// DeleteFile removes an uploaded file (best-effort cleanup).
func (c *AsyncClient) DeleteFile(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/files/"+id)
}

func (c *AsyncClient) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	return c.do(req, nil)
}

func (c *AsyncClient) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// do executes the request and, if out is non-nil, decodes a JSON body into it.
// A non-2xx status returns an error carrying the response body.
func (c *AsyncClient) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &apiError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(bytes.TrimSpace(body)),
		}
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

// apiError is a non-2xx HTTP response from the Soniox API.
type apiError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *apiError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("soniox API %s: %s", e.Status, e.Body)
	}
	return fmt.Sprintf("soniox API %s", e.Status)
}

// isRetryable reports whether err is a transient failure worth retrying:
// gateway/5xx responses, rate limits, or transport-level errors (timeouts,
// resets). Context cancellation is deliberately not classified here — callers
// must check ctx.Err() themselves before retrying.
func isRetryable(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.StatusCode == http.StatusTooManyRequests || ae.StatusCode >= 500
	}
	// Non-API errors reaching here are transport-level; safe to retry.
	return true
}

// sleepCtx waits for d or until ctx is cancelled. It returns false if ctx was
// cancelled during the wait.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
