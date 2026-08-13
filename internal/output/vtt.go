package output

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/livetx/livetx/internal/soniox"
)

// VTTOptions tunes how word-level tokens are grouped into caption cues.
type VTTOptions struct {
	// MaxChars is the soft upper bound on characters per cue before it is split.
	MaxChars int
	// MaxLineLen is the target maximum characters per wrapped line within a cue.
	MaxLineLen int
	// MaxDurationMs is the upper bound on a single cue's duration.
	MaxDurationMs int64
	// GapMs is the silence gap between tokens that forces a new cue.
	GapMs int64
}

func (o VTTOptions) withDefaults() VTTOptions {
	if o.MaxChars <= 0 {
		o.MaxChars = 84 // ~two 42-char lines
	}
	if o.MaxLineLen <= 0 {
		o.MaxLineLen = 42
	}
	if o.MaxDurationMs <= 0 {
		o.MaxDurationMs = 6000
	}
	if o.GapMs <= 0 {
		o.GapMs = 700
	}
	return o
}

type cue struct {
	startMs int64
	endMs   int64
	text    string
}

// BuildVTT writes a WebVTT document to w, grouping the given word-level tokens
// (each carrying StartMs/EndMs) into caption cues.
func BuildVTT(w io.Writer, tokens []soniox.Token, opts VTTOptions) error {
	opts = opts.withDefaults()
	cues := groupCues(tokens, opts)

	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString("WEBVTT\n\n"); err != nil {
		return err
	}
	for i, c := range cues {
		fmt.Fprintf(bw, "%d\n", i+1)
		fmt.Fprintf(bw, "%s --> %s\n", formatTimestamp(c.startMs), formatTimestamp(c.endMs))
		bw.WriteString(wrapLines(c.text, opts.MaxLineLen))
		bw.WriteString("\n\n")
	}
	return bw.Flush()
}

// groupCues walks the tokens in order and cuts a new cue on sentence-ending
// punctuation, a silence gap, a speaker change, or a length/duration cap.
func groupCues(tokens []soniox.Token, opts VTTOptions) []cue {
	var cues []cue
	var (
		curText  strings.Builder
		curStart int64
		curEnd   int64
		haveCur  bool
		prevEnd  int64
		prevSpk  int
	)

	flush := func() {
		if !haveCur {
			return
		}
		text := strings.TrimSpace(curText.String())
		if text != "" {
			cues = append(cues, cue{startMs: curStart, endMs: curEnd, text: text})
		}
		curText.Reset()
		haveCur = false
	}

	for _, tok := range tokens {
		if tok.Text == "" {
			continue
		}

		// Break before this token on a long silence gap or speaker change.
		if haveCur {
			gap := tok.StartMs - prevEnd
			if (opts.GapMs > 0 && gap > opts.GapMs) || tok.Speaker != prevSpk {
				flush()
			}
		}

		if !haveCur {
			curStart = tok.StartMs
			haveCur = true
		}
		curText.WriteString(tok.Text)
		curEnd = tok.EndMs
		prevEnd = tok.EndMs
		prevSpk = tok.Speaker

		// Break after this token on sentence end, char cap, or duration cap.
		trimmed := strings.TrimSpace(curText.String())
		endsSentence := endsWithSentencePunct(trimmed)
		tooLong := opts.MaxChars > 0 && utf8.RuneCountInString(trimmed) >= opts.MaxChars
		tooLong2 := opts.MaxDurationMs > 0 && (curEnd-curStart) >= opts.MaxDurationMs
		if endsSentence || tooLong || tooLong2 {
			flush()
		}
	}
	flush()
	return cues
}

func endsWithSentencePunct(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	switch r {
	case '.', '!', '?', '…', '。', '！', '？':
		return true
	}
	return false
}

// formatTimestamp renders milliseconds as the WebVTT time form HH:MM:SS.mmm.
func formatTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	totalSec := ms / 1000
	millis := ms % 1000
	hours := totalSec / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

// wrapLines greedily wraps text at word boundaries so each line stays within
// maxLineLen characters where possible.
func wrapLines(text string, maxLineLen int) string {
	if maxLineLen <= 0 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}
	var lines []string
	var cur strings.Builder
	for _, word := range words {
		if cur.Len() == 0 {
			cur.WriteString(word)
			continue
		}
		if utf8.RuneCountInString(cur.String())+1+utf8.RuneCountInString(word) > maxLineLen {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(word)
		} else {
			cur.WriteString(" ")
			cur.WriteString(word)
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return strings.Join(lines, "\n")
}
