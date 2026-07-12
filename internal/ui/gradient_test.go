package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func TestLerpEndpointsAndMidpoint(t *testing.T) {
	if got := lerp(0, 100, 0); got != 0 {
		t.Errorf("lerp(0,100,0) = %d, want 0", got)
	}
	if got := lerp(0, 100, 1); got != 100 {
		t.Errorf("lerp(0,100,1) = %d, want 100", got)
	}
	if got := lerp(0, 100, 0.5); got != 50 {
		t.Errorf("lerp(0,100,0.5) = %d, want 50", got)
	}
}

func TestHexToRGB(t *testing.T) {
	r, g, b := hexToRGB("#ff0080")
	if r != 255 || g != 0 || b != 128 {
		t.Errorf("hexToRGB(#ff0080) = (%d,%d,%d), want (255,0,128)", r, g, b)
	}
}

func TestMutedColorBlendsTowardBlack(t *testing.T) {
	muted := mutedColor(lipgloss.Color("#B341F5"))
	r, g, b := hexToRGB(string(muted))
	if r != 89 || g != 32 || b != 122 {
		t.Errorf("mutedColor(#B341F5) = (%d,%d,%d), want (89,32,122)", r, g, b)
	}
}

func TestGradientTextVariesAcrossLine(t *testing.T) {
	out := gradientText("XXXXXXXXXX", lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	if !strings.Contains(out, "38;2;255;0;0") {
		t.Errorf("expected the first character to be pure red, got: %q", out)
	}
	if !strings.Contains(out, "38;2;0;0;255") {
		t.Errorf("expected the last character to be pure blue, got: %q", out)
	}
}

func TestGradientBoxDimensions(t *testing.T) {
	content := "line one\nline two\nline three"
	out := gradientBox(content, 20, 3, lipgloss.Color("#00ff00"), lipgloss.Color("#0000ff"))
	lines := strings.Split(out, "\n")

	if len(lines) != 5 { // 3 content rows + top + bottom
		t.Fatalf("expected 5 total lines, got %d:\n%s", len(lines), out)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != 22 { // width(20) + 2 border columns
			t.Errorf("line %d: visible width = %d, want 22 (%q)", i, w, line)
		}
	}
	if !strings.Contains(lines[0], "╭") || !strings.Contains(lines[0], "╮") {
		t.Errorf("top row missing corner glyphs: %q", lines[0])
	}
	if !strings.Contains(lines[4], "╰") || !strings.Contains(lines[4], "╯") {
		t.Errorf("bottom row missing corner glyphs: %q", lines[4])
	}
}

func TestGradientBoxColorVariesCornerToCorner(t *testing.T) {
	out := gradientBox("x", 10, 1, lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	if !strings.Contains(out, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", out)
	}
	if !strings.Contains(out, "38;2;0;0;255") {
		t.Errorf("expected the bottom-right corner to be pure blue, got:\n%s", out)
	}
}

// TestGradientBoxPadsShorterContent confirms height acts as a floor: content
// shorter than the requested height is padded with blank rows.
func TestGradientBoxPadsShorterContent(t *testing.T) {
	out := gradientBox("one line", 10, 5, lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	lines := strings.Split(out, "\n")
	if len(lines) != 7 { // 5 content rows (padded) + top + bottom
		t.Fatalf("expected 7 total lines, got %d:\n%s", len(lines), out)
	}
}

// TestGradientBoxNeverTruncatesTallerContent is a regression guard for the
// About screen's existing behavior: its content is already longer than its
// assigned height budget, and today's rendering (via lipgloss's own
// Height()) simply grows past that budget rather than cutting content off.
// gradientBox must preserve that, not silently start truncating content.
func TestGradientBoxNeverTruncatesTallerContent(t *testing.T) {
	content := "one\ntwo\nthree\nfour\nfive"
	out := gradientBox(content, 10, 2, lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	lines := strings.Split(out, "\n")
	if len(lines) != 7 { // all 5 content rows + top + bottom, not truncated to 2
		t.Fatalf("expected 7 total lines (no truncation), got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(out, "five") {
		t.Errorf("expected all content lines to survive, \"five\" was truncated:\n%s", out)
	}
}
