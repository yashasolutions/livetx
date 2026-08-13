package output

import (
	"strings"
	"testing"

	"github.com/livetx/livetx/internal/soniox"
)

func TestFormatTimestamp(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "00:00:00.000"},
		{1240, "00:00:01.240"},
		{61001, "00:01:01.001"},
		{3661500, "01:01:01.500"},
		{-5, "00:00:00.000"},
	}
	for _, c := range cases {
		if got := formatTimestamp(c.ms); got != c.want {
			t.Errorf("formatTimestamp(%d) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestBuildVTTSentenceBreak(t *testing.T) {
	tokens := []soniox.Token{
		{Text: "Hello", StartMs: 1000, EndMs: 1400},
		{Text: " world.", StartMs: 1400, EndMs: 1980},
		{Text: " Next", StartMs: 2100, EndMs: 2400},
		{Text: " one.", StartMs: 2400, EndMs: 2800},
	}

	var sb strings.Builder
	if err := BuildVTT(&sb, tokens, VTTOptions{}); err != nil {
		t.Fatalf("BuildVTT: %v", err)
	}
	got := sb.String()

	want := "WEBVTT\n\n" +
		"1\n00:00:01.000 --> 00:00:01.980\nHello world.\n\n" +
		"2\n00:00:02.100 --> 00:00:02.800\nNext one.\n\n"
	if got != want {
		t.Errorf("unexpected VTT output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuildVTTGapBreak(t *testing.T) {
	// Two words separated by a 900ms silence gap (> default 700ms) => two cues,
	// even without sentence punctuation.
	tokens := []soniox.Token{
		{Text: "one", StartMs: 0, EndMs: 300},
		{Text: " two", StartMs: 1200, EndMs: 1500},
	}

	var sb strings.Builder
	if err := BuildVTT(&sb, tokens, VTTOptions{}); err != nil {
		t.Fatalf("BuildVTT: %v", err)
	}
	got := sb.String()

	if strings.Count(got, "-->") != 2 {
		t.Errorf("expected 2 cues from gap break, got:\n%s", got)
	}
	if !strings.Contains(got, "00:00:00.000 --> 00:00:00.300\none\n") {
		t.Errorf("first cue missing/incorrect:\n%s", got)
	}
	if !strings.Contains(got, "00:00:01.200 --> 00:00:01.500\ntwo\n") {
		t.Errorf("second cue missing/incorrect:\n%s", got)
	}
}

func TestBuildVTTMaxChars(t *testing.T) {
	// No punctuation, no gaps: a low MaxChars must still force a split.
	tokens := []soniox.Token{
		{Text: "alpha", StartMs: 0, EndMs: 200},
		{Text: " bravo", StartMs: 200, EndMs: 400},
		{Text: " charlie", StartMs: 400, EndMs: 600},
		{Text: " delta", StartMs: 600, EndMs: 800},
	}

	var sb strings.Builder
	if err := BuildVTT(&sb, tokens, VTTOptions{MaxChars: 10, MaxLineLen: 100}); err != nil {
		t.Fatalf("BuildVTT: %v", err)
	}
	got := sb.String()

	if n := strings.Count(got, "-->"); n < 2 {
		t.Errorf("expected MaxChars to split into multiple cues, got %d cue(s):\n%s", n, got)
	}
}

func TestBuildVTTEmpty(t *testing.T) {
	var sb strings.Builder
	if err := BuildVTT(&sb, nil, VTTOptions{}); err != nil {
		t.Fatalf("BuildVTT: %v", err)
	}
	if got := sb.String(); got != "WEBVTT\n\n" {
		t.Errorf("empty transcript = %q, want %q", got, "WEBVTT\n\n")
	}
}

func TestWrapLines(t *testing.T) {
	got := wrapLines("the quick brown fox jumps", 9)
	want := "the quick\nbrown fox\njumps"
	if got != want {
		t.Errorf("wrapLines = %q, want %q", got, want)
	}
}
