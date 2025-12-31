package soniox

// StartRequest is the initial message sent to start transcription
type StartRequest struct {
	APIKey                       string       `json:"api_key"`
	Model                        string       `json:"model"`
	AudioFormat                  string       `json:"audio_format"`
	NumChannels                  int          `json:"num_channels"`
	SampleRate                   int          `json:"sample_rate"`
	LanguageHints                []string     `json:"language_hints,omitempty"`
	EnableLanguageIdentification bool         `json:"enable_language_identification"`
	Translation                  *Translation `json:"translation,omitempty"`
}

// Translation configures translation settings
type Translation struct {
	Type           string `json:"type"`
	TargetLanguage string `json:"target_language"`
}

// Token represents a single token in the transcription response
type Token struct {
	Text           string  `json:"text"`
	IsFinal        bool    `json:"is_final"`
	Confidence     float64 `json:"confidence,omitempty"`
	Speaker        int     `json:"speaker,omitempty"`
	Language       string  `json:"language,omitempty"`
	SourceLanguage string  `json:"source_language,omitempty"`
	StartMs        int64   `json:"start_ms,omitempty"`
	EndMs          int64   `json:"end_ms,omitempty"`
}

// Response is the message received from Soniox
type Response struct {
	Tokens       []Token `json:"tokens,omitempty"`
	Finished     bool    `json:"finished,omitempty"`
	ErrorCode    string  `json:"error_code,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`
}

// TranscriptEvent represents a processed transcript event for consumers
type TranscriptEvent struct {
	Text    string
	IsFinal bool
	Speaker int
	Raw     *Response
}
