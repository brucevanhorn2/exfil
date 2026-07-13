# Visual Effects (Gradient Chrome) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace exfil's flat-color chrome (pane borders, titles, the transfer progress bar) with static gradients derived from the user's existing primary/secondary theme colors, giving the app a neon/cyberpunk look.

**Architecture:** A new shared `gradientBox`/`gradientText` rendering primitive (generalizing the About screen's existing per-character `gradientLogo` gradient technique) replaces lipgloss's single-flat-color border/title styles everywhere a pane is bordered. `Theme` gains raw color fields (`PrimaryColor`/`SecondaryColor`/muted variants) so the gradient renderer has real hex endpoints to interpolate between — something a `lipgloss.Style` alone can't provide.

**Tech Stack:** Go, `github.com/charmbracelet/lipgloss` (styling, including its `Width()`/`MaxWidth()`/`Height()` reflow behavior, reused rather than reimplemented), `github.com/charmbracelet/bubbles/progress` (already-built-in gradient support for the progress bar).

## Global Constraints

- Gradients are **static only** — no animation (pulsing, blinking, scanlines) in this project.
- Gradient endpoints are always the user's existing `primary`/`secondary` theme colors — no new color settings.
- Content text (file/directory listings, selection highlight, semantic transfer-status colors) stays flat-colored — only borders, titles, and the progress bar get the gradient treatment.
- The About screen's logo keeps its own fixed cyan→purple gradient (`logoFrom`/`logoTo` in `about.go`), independent of the user's theme colors — untouched by this plan.
- `gradientBox`'s `width`/`height` parameters use the same convention as `lipgloss.Style.Width()`/`Height()`: the *interior* box size (not counting the two border-character columns/rows). Total rendered size is `width+2` columns by (at least) `height+2` rows.
- `gradientBox`'s height is a **floor, not a ceiling** — it pads content shorter than `height` with blank rows, but never truncates content that's already taller (matching `lipgloss.Style.Height()`'s real behavior, verified empirically against this exact codebase — see Task 1). Width, by contrast, wraps (via `lipgloss.Style.Width()`), matching every pane's current behavior exactly.
- Full verification after every task: `go build ./... && go vet ./... && go test ./... && gofmt -l .` must all be clean before committing.

---

### Task 1: Core gradient primitives (`internal/ui/gradient.go`)

**Files:**
- Create: `internal/ui/gradient.go`
- Create: `internal/ui/gradient_test.go`
- Modify: `internal/ui/about.go`

**Interfaces:**
- Produces: `hexToRGB(hex string) (r, g, b int)`, `lerp(a, b int, t float64) int`, `gradientLogo(text, from, to string) string`, `gradientText(s string, from, to lipgloss.Color) string`, `mutedColor(c lipgloss.Color) lipgloss.Color`, `gradientBox(content string, width, height int, from, to lipgloss.Color) string`

- [x] **Step 1: Create `internal/ui/gradient.go`**

`hexToRGB`, `lerp`, and `gradientLogo` move here verbatim from `about.go` (no behavior change — About's logo gradient keeps working exactly as today, just relocated since it's now a shared building block, not an About-only detail). `gradientText`, `mutedColor`, and `gradientBox` are new.

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// hexToRGB parses a "#RRGGBB" string into its red/green/blue components.
func hexToRGB(hex string) (r, g, b int) {
	fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b)
	return
}

// lerp linearly interpolates between a and b at position t (0.0-1.0).
func lerp(a, b int, t float64) int {
	return int(float64(a) + t*float64(b-a))
}

// gradientLogo renders text with a horizontal color gradient from `from` to
// `to`, interpolated by each character's column position relative to the
// widest line, so the gradient flows consistently across the whole block
// rather than resetting on each line. Used by the About screen's ASCII logo
// (its own fixed cyan/purple endpoints, independent of the user's theme).
func gradientLogo(text, from, to string) string {
	lines := strings.Split(text, "\n")

	maxWidth := 0
	for _, line := range lines {
		if w := len([]rune(line)); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth <= 1 {
		return text
	}

	fr, fg, fb := hexToRGB(from)
	tr, tg, tb := hexToRGB(to)

	out := make([]string, len(lines))
	for li, line := range lines {
		var b strings.Builder
		for i, r := range []rune(line) {
			if r == ' ' {
				b.WriteRune(r)
				continue
			}
			t := float64(i) / float64(maxWidth-1)
			hex := fmt.Sprintf("#%02x%02x%02x", lerp(fr, tr, t), lerp(fg, tg, t), lerp(fb, tb, t))
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(string(r)))
		}
		out[li] = b.String()
	}
	return strings.Join(out, "\n")
}

// gradientText applies the same horizontal gradient as gradientLogo to a
// single line of arbitrary text (e.g. a pane's title bar) — gradientLogo
// already handles a single-line input correctly (maxWidth becomes that
// line's own length), so this is a thin, typed wrapper rather than a
// reimplementation.
func gradientText(s string, from, to lipgloss.Color) string {
	return gradientLogo(s, string(from), string(to))
}

// mutedColor blends c 50% of the way toward black. Used for unfocused
// panes' gradient endpoints — there's no way to query the user's actual
// terminal background color, so black is a deterministic stand-in for
// "dimmer," not a literal background match.
func mutedColor(c lipgloss.Color) lipgloss.Color {
	r, g, b := hexToRGB(string(c))
	hex := fmt.Sprintf("#%02x%02x%02x", lerp(r, 0, 0.5), lerp(g, 0, 0.5), lerp(b, 0, 0.5))
	return lipgloss.Color(hex)
}

// gradientBox manually draws a rounded-corner border around content (built
// exactly as every pane already builds it — already-styled title/listing/
// etc. text, left untouched by this function), sized to width x height —
// the *interior* box size, matching lipgloss.Style's own Width()/Height()
// convention (total rendered size is width+2 columns by at least height+2
// rows). Each border character is colored by its position along the box's
// diagonal, from `from` at the top-left corner to `to` at the bottom-right.
//
// Width wraps overflowing lines (via lipgloss's own Width(), which also
// pads every resulting physical line to exactly the requested width while
// preserving embedded ANSI styling) — the same reflow every pane already
// relies on today via `theme.PaneBorder.Width(...).Render(...)`. Height is
// a floor, not a ceiling: content shorter than height is padded with blank
// rows, but content already taller is never truncated — matching
// lipgloss.Style.Height()'s real behavior (verified empirically: a
// Height(3) box given 6 lines of content renders 6 content rows, not 3).
func gradientBox(content string, width, height int, from, to lipgloss.Color) string {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	innerWidth := width - 2 // 1-space margin each side, like Padding(0, 1)
	if innerWidth < 0 {
		innerWidth = 0
	}

	wrapped := lipgloss.NewStyle().Width(innerWidth).Render(content)
	lines := strings.Split(wrapped, "\n")

	blankRow := strings.Repeat(" ", innerWidth)
	for len(lines) < height {
		lines = append(lines, blankRow)
	}
	actualHeight := len(lines)

	fr, fg, fb := hexToRGB(string(from))
	tr, tg, tb := hexToRGB(string(to))
	denom := float64(width + actualHeight + 2)

	colorAt := func(x, y int) lipgloss.Color {
		t := float64(x+y) / denom
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		hex := fmt.Sprintf("#%02x%02x%02x", lerp(fr, tr, t), lerp(fg, tg, t), lerp(fb, tb, t))
		return lipgloss.Color(hex)
	}
	borderChar := func(x, y int, ch string) string {
		return lipgloss.NewStyle().Foreground(colorAt(x, y)).Render(ch)
	}

	rows := make([]string, 0, actualHeight+2)

	var top strings.Builder
	top.WriteString(borderChar(0, 0, "╭"))
	for x := 1; x <= width; x++ {
		top.WriteString(borderChar(x, 0, "─"))
	}
	top.WriteString(borderChar(width+1, 0, "╮"))
	rows = append(rows, top.String())

	for i, line := range lines {
		y := i + 1
		rows = append(rows, borderChar(0, y, "│")+" "+line+" "+borderChar(width+1, y, "│"))
	}

	bottomY := actualHeight + 1
	var bottom strings.Builder
	bottom.WriteString(borderChar(0, bottomY, "╰"))
	for x := 1; x <= width; x++ {
		bottom.WriteString(borderChar(x, bottomY, "─"))
	}
	bottom.WriteString(borderChar(width+1, bottomY, "╯"))
	rows = append(rows, bottom.String())

	return strings.Join(rows, "\n")
}
```

- [x] **Step 2: Remove the now-duplicated functions from `about.go`**

Find (in `internal/ui/about.go`):

```go
import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/bvanhorn/exfil/internal/version"
	"github.com/charmbracelet/lipgloss"
)
```

Replace with (the `lipgloss` import is no longer referenced directly anywhere else in this file — `gradientLogo`/`hexToRGB`/`lerp` were its only users):

```go
import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/bvanhorn/exfil/internal/version"
)
```

Find:

```go
// gradientLogo renders text with a horizontal color gradient from `from` to
// `to`, interpolated by each character's column position relative to the
// widest line, so the gradient flows consistently across the whole block
// rather than resetting on each line.
func gradientLogo(text, from, to string) string {
	lines := strings.Split(text, "\n")

	maxWidth := 0
	for _, line := range lines {
		if w := len([]rune(line)); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth <= 1 {
		return text
	}

	fr, fg, fb := hexToRGB(from)
	tr, tg, tb := hexToRGB(to)

	out := make([]string, len(lines))
	for li, line := range lines {
		var b strings.Builder
		for i, r := range []rune(line) {
			if r == ' ' {
				b.WriteRune(r)
				continue
			}
			t := float64(i) / float64(maxWidth-1)
			hex := fmt.Sprintf("#%02x%02x%02x", lerp(fr, tr, t), lerp(fg, tg, t), lerp(fb, tb, t))
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(string(r)))
		}
		out[li] = b.String()
	}
	return strings.Join(out, "\n")
}

func hexToRGB(hex string) (r, g, b int) {
	fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b)
	return
}

func lerp(a, b int, t float64) int {
	return int(float64(a) + t*float64(b-a))
}

func (a *AboutPane) View(theme Theme, loc *i18n.Localizer) string {
```

Replace with:

```go
func (a *AboutPane) View(theme Theme, loc *i18n.Localizer) string {
```

- [x] **Step 3: Write `internal/ui/gradient_test.go`**

Tests run with `go test` are not attached to a TTY, so lipgloss defaults to
plain (no-color) output — verified empirically against this exact module: a
`Foreground(...)`-styled `Render()` call produces zero ANSI codes under
`go test` unless the color profile is forced. `init()` forces true-color
output for every test in this package for the rest of this test binary's
run; it doesn't affect any other existing test (they either inspect `Style`
objects directly via `GetForeground()`, unaffected by color profile, or
check plain substrings via `strings.Contains`, which still match regardless
of any ANSI codes now surrounding them).

```go
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
```

- [x] **Step 4: Run `go mod tidy` (new test-only dependency on `github.com/muesli/termenv`)**

Run: `go mod tidy`
Expected: `github.com/muesli/termenv` moves from `// indirect` to a direct
requirement in `go.mod` (it's already in `go.sum` as a transitive dependency
of `lipgloss`/`bubbles`, so no new download is needed — just a `go.mod`
bookkeeping change).

- [x] **Step 5: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS (including the new ones above and
every pre-existing test unchanged), `gofmt -l .` prints nothing.

- [x] **Step 6: Commit**

```bash
git add internal/ui/gradient.go internal/ui/gradient_test.go internal/ui/about.go go.mod
git commit -m "Add gradientBox/gradientText core rendering primitives"
```

---

### Task 2: `Theme` gains gradient color fields

**Files:**
- Modify: `internal/ui/theme.go`
- Modify: `internal/ui/theme_test.go`

**Interfaces:**
- Consumes: `mutedColor(c lipgloss.Color) lipgloss.Color` (Task 1)
- Produces: `Theme.PrimaryColor`, `Theme.SecondaryColor`, `Theme.MutedPrimaryColor`, `Theme.MutedSecondaryColor` (all `lipgloss.Color`)

- [x] **Step 1: Add the new fields to `Theme`**

Find (in `internal/ui/theme.go`):

```go
type Theme struct {
	// Pane borders
	PaneBorder      lipgloss.Style
	PaneBorderFocus lipgloss.Style
	PaneTitle       lipgloss.Style
	PaneTitleFocus  lipgloss.Style
```

Replace with:

```go
type Theme struct {
	// Raw color values, needed by gradientBox/gradientText (a lipgloss.Style
	// only holds one flat color, not enough to interpolate a gradient from).
	PrimaryColor        lipgloss.Color
	SecondaryColor       lipgloss.Color
	MutedPrimaryColor    lipgloss.Color // PrimaryColor blended 50% toward black
	MutedSecondaryColor  lipgloss.Color // SecondaryColor blended 50% toward black

	// Pane borders
	PaneBorder      lipgloss.Style
	PaneBorderFocus lipgloss.Style
	PaneTitle       lipgloss.Style
	PaneTitleFocus  lipgloss.Style
```

- [x] **Step 2: Populate the new fields in `NewTheme`**

Find:

```go
func NewTheme(primary, secondary lipgloss.Color) Theme {
	return Theme{
		// Pane borders
		PaneBorder: lipgloss.NewStyle().
```

Replace with:

```go
func NewTheme(primary, secondary lipgloss.Color) Theme {
	return Theme{
		PrimaryColor:        primary,
		SecondaryColor:      secondary,
		MutedPrimaryColor:   mutedColor(primary),
		MutedSecondaryColor: mutedColor(secondary),

		// Pane borders
		PaneBorder: lipgloss.NewStyle().
```

- [x] **Step 3: Run gofmt to fix struct field alignment**

Run: `gofmt -w internal/ui/theme.go`
Expected: field name/type columns realign automatically; no functional change.

- [x] **Step 4: Add test assertions**

Find (in `internal/ui/theme_test.go`):

```go
func TestNewThemeAppliesPrimaryAndSecondary(t *testing.T) {
	primary := lipgloss.Color("#39FF14")
	secondary := lipgloss.Color("#3A3A4A")
	theme := NewTheme(primary, secondary)

	if theme.PaneTitleFocus.GetForeground() != primary {
		t.Errorf("PaneTitleFocus foreground = %v, want %v", theme.PaneTitleFocus.GetForeground(), primary)
	}
	if theme.PaneTitle.GetForeground() != secondary {
		t.Errorf("PaneTitle foreground = %v, want %v", theme.PaneTitle.GetForeground(), secondary)
	}
	if theme.BrowserSelected.GetBackground() != primary {
		t.Errorf("BrowserSelected background = %v, want %v", theme.BrowserSelected.GetBackground(), primary)
	}
	if theme.QueueTitle.GetForeground() != secondary {
		t.Errorf("QueueTitle foreground = %v, want %v", theme.QueueTitle.GetForeground(), secondary)
	}
}
```

Replace with:

```go
func TestNewThemeAppliesPrimaryAndSecondary(t *testing.T) {
	primary := lipgloss.Color("#39FF14")
	secondary := lipgloss.Color("#3A3A4A")
	theme := NewTheme(primary, secondary)

	if theme.PaneTitleFocus.GetForeground() != primary {
		t.Errorf("PaneTitleFocus foreground = %v, want %v", theme.PaneTitleFocus.GetForeground(), primary)
	}
	if theme.PaneTitle.GetForeground() != secondary {
		t.Errorf("PaneTitle foreground = %v, want %v", theme.PaneTitle.GetForeground(), secondary)
	}
	if theme.BrowserSelected.GetBackground() != primary {
		t.Errorf("BrowserSelected background = %v, want %v", theme.BrowserSelected.GetBackground(), primary)
	}
	if theme.QueueTitle.GetForeground() != secondary {
		t.Errorf("QueueTitle foreground = %v, want %v", theme.QueueTitle.GetForeground(), secondary)
	}
}

func TestNewThemeStoresRawGradientColors(t *testing.T) {
	primary := lipgloss.Color("#39FF14")
	secondary := lipgloss.Color("#3A3A4A")
	theme := NewTheme(primary, secondary)

	if theme.PrimaryColor != primary {
		t.Errorf("PrimaryColor = %v, want %v", theme.PrimaryColor, primary)
	}
	if theme.SecondaryColor != secondary {
		t.Errorf("SecondaryColor = %v, want %v", theme.SecondaryColor, secondary)
	}
	if theme.MutedPrimaryColor != mutedColor(primary) {
		t.Errorf("MutedPrimaryColor = %v, want %v", theme.MutedPrimaryColor, mutedColor(primary))
	}
	if theme.MutedSecondaryColor != mutedColor(secondary) {
		t.Errorf("MutedSecondaryColor = %v, want %v", theme.MutedSecondaryColor, mutedColor(secondary))
	}
}
```

- [x] **Step 5: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [x] **Step 6: Commit**

```bash
git add internal/ui/theme.go internal/ui/theme_test.go
git commit -m "Theme: store raw primary/secondary/muted gradient colors"
```

---

### Task 3: BrowserPane gradient border and title

**Files:**
- Modify: `internal/ui/browser.go`
- Modify: `internal/ui/browser_test.go`

**Interfaces:**
- Consumes: `gradientBox`, `gradientText` (Task 1), `Theme.PrimaryColor`/`SecondaryColor`/`MutedPrimaryColor`/`MutedSecondaryColor` (Task 2)

- [x] **Step 1: Switch `View()`'s border and title rendering**

Find (in `internal/ui/browser.go`):

```go
func (b *BrowserPane) View(theme Theme) string {
	titleStyle := theme.PaneTitle
	borderStyle := theme.PaneBorder

	if b.Focus {
		titleStyle = theme.PaneTitleFocus
		borderStyle = theme.PaneBorderFocus
	}

	titleWithPath := titleStyle.Render(fmt.Sprintf(" %s:%s ", b.Title, b.Cwd))
```

Replace with:

```go
func (b *BrowserPane) View(theme Theme) string {
	from, to := theme.MutedPrimaryColor, theme.MutedSecondaryColor
	if b.Focus {
		from, to = theme.PrimaryColor, theme.SecondaryColor
	}

	titleWithPath := gradientText(fmt.Sprintf(" %s:%s ", b.Title, b.Cwd), from, to)
```

Find:

```go
	content := strings.Join(lines, "\n")
	bordered := borderStyle.Width(b.Width).Render(content)
	return bordered
}
```

Replace with:

```go
	content := strings.Join(lines, "\n")
	// -2: gradientBox's height convention is interior rows only, and the
	// title row is already baked into content (same accounting as
	// BrowserPane.View()) — keeps the total rendered size at b.Height.
	return gradientBox(content, b.Width, b.Height-2, from, to)
}
```

- [x] **Step 2: Run the existing test suite to confirm no regressions**

Run: `go test ./internal/ui/... -run TestBrowserPane -v`
Expected: `TestBrowserPaneBack`, `TestBrowserPaneEnsureVisible`,
`TestBrowserPaneEmptyMessageRendersWhenNoEntries`,
`TestBrowserPaneEmptyMessageHiddenOnceEntriesExist` all still PASS
unchanged — none of them depend on the border rendering mechanism.

- [x] **Step 3: Add a focus-vs-unfocus color regression test**

Find (in `internal/ui/browser_test.go`):

```go
// TestBrowserPaneEmptyMessageHiddenOnceEntriesExist confirms EmptyMessage is
// purely a placeholder for the empty state — it must not linger once real
// entries are set (e.g. after connecting and listing a directory).
func TestBrowserPaneEmptyMessageHiddenOnceEntriesExist(t *testing.T) {
	b := NewBrowserPane("remote", fsys.LocalFS{})
	b.Width = 40
	b.Height = 10
	b.EmptyMessage = "Not connected. Press [s] to select a host."
	b.SetEntries([]fsys.Entry{{Name: "file.txt"}})

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := b.View(theme)

	if strings.Contains(view, "Not connected") {
		t.Errorf("EmptyMessage should not render once entries are present, got:\n%s", view)
	}
}
```

Replace with:

```go
// TestBrowserPaneEmptyMessageHiddenOnceEntriesExist confirms EmptyMessage is
// purely a placeholder for the empty state — it must not linger once real
// entries are set (e.g. after connecting and listing a directory).
func TestBrowserPaneEmptyMessageHiddenOnceEntriesExist(t *testing.T) {
	b := NewBrowserPane("remote", fsys.LocalFS{})
	b.Width = 40
	b.Height = 10
	b.EmptyMessage = "Not connected. Press [s] to select a host."
	b.SetEntries([]fsys.Entry{{Name: "file.txt"}})

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := b.View(theme)

	if strings.Contains(view, "Not connected") {
		t.Errorf("EmptyMessage should not render once entries are present, got:\n%s", view)
	}
}

// TestBrowserPaneFocusUsesVividGradientUnfocusedUsesMuted is a regression
// test for the visual-effects feature: a focused pane's border/title must
// render with the full-intensity primary/secondary gradient, and an
// unfocused pane with the muted (50%-toward-black) variant — proving the
// two are visually distinguishable, not just structurally different.
func TestBrowserPaneFocusUsesVividGradientUnfocusedUsesMuted(t *testing.T) {
	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))

	b := NewBrowserPane("test", fsys.LocalFS{})
	b.Width = 30
	b.Height = 10

	b.Focus = false
	unfocused := b.View(theme)
	vividRed := "38;2;255;0;0"
	if strings.Contains(unfocused, vividRed) {
		t.Errorf("unfocused pane should not use the vivid primary color, got:\n%s", unfocused)
	}

	b.Focus = true
	focused := b.View(theme)
	if !strings.Contains(focused, vividRed) {
		t.Errorf("focused pane's title/border should include the vivid primary color, got:\n%s", focused)
	}
}
```

- [x] **Step 4: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [x] **Step 5: Commit**

```bash
git add internal/ui/browser.go internal/ui/browser_test.go
git commit -m "BrowserPane: gradient border/title, muted when unfocused"
```

---

### Task 4: QueuePane gradient border/title, and the progress bar's colors

**Files:**
- Modify: `internal/ui/queuepane.go`
- Modify: `internal/ui/queuepane_test.go`

**Interfaces:**
- Consumes: `gradientBox`, `gradientText` (Task 1), `Theme.PrimaryColor`/`SecondaryColor` (Task 2)

- [x] **Step 1: Switch `View()`'s border and title rendering**

Find (in `internal/ui/queuepane.go`):

```go
func (q *QueuePane) View(theme Theme, loc *i18n.Localizer) string {
	title := theme.QueueTitle.Render(loc.T("screen_title_queue"))
	border := theme.QueueBorder

	// -2 for the border's top/bottom lines, -1 for the title line above.
	maxRows := q.Height - 3
```

Replace with:

```go
func (q *QueuePane) View(theme Theme, loc *i18n.Localizer) string {
	title := gradientText(loc.T("screen_title_queue"), theme.PrimaryColor, theme.SecondaryColor)

	// -2 for the border's top/bottom lines, -1 for the title line above.
	maxRows := q.Height - 3
```

Find:

```go
	content := strings.Join(lines, "\n")
	return border.Width(q.Width).Render(content)
}
```

Replace with:

```go
	content := strings.Join(lines, "\n")
	// Height only: gradientBox's height convention is interior rows, and the
	// title row is already baked into content (same accounting as
	// BrowserPane.View()) — q.Height-2 keeps the total rendered size at
	// q.Height. Width needs no such adjustment — q.Width passes straight
	// through, matching the old border.Width(q.Width) call's own total
	// rendered width (q.Width+2, since QueueBorder already carries
	// Padding(0, 1) baked into that budget) exactly.
	return gradientBox(content, q.Width, q.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
```

- [x] **Step 2: Swap the progress bar's hardcoded gradient for the theme's colors**

Find:

```go
	var progressView string
	if t.Total > 0 {
		pct := float64(t.Done) / float64(t.Total)
		prog := progress.New(progress.WithScaledGradient("#ff00ff", "#00ffff"))
		progressView = prog.ViewAs(pct)
	} else {
		progressView = "      "
	}
```

Replace with:

```go
	var progressView string
	if t.Total > 0 {
		pct := float64(t.Done) / float64(t.Total)
		prog := progress.New(progress.WithScaledGradient(string(theme.PrimaryColor), string(theme.SecondaryColor)))
		progressView = prog.ViewAs(pct)
	} else {
		progressView = "      "
	}
```

- [x] **Step 3: Run the existing test suite to confirm no regressions**

Run: `go test ./internal/ui/... -run TestQueuePane -v`
Expected: `TestQueuePaneViewCapsHeight` and `TestQueuePaneViewEmptyFillsHeight`
both still PASS unchanged — `q.Transfers` is already capped to `maxRows`
before `content` is built, so `gradientBox`'s height-is-a-floor behavior
never has more than `q.Height-2` lines to work with, producing exactly
`q.Height` total rendered lines exactly as before.

- [x] **Step 4: Add a gradient color-variation regression test**

Find (in `internal/ui/queuepane_test.go`):

```go
func TestQueuePaneViewEmptyFillsHeight(t *testing.T) {
	q := NewQueuePane()
	q.Width = 40
	q.Height = 8

	view := q.View(NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor)), i18n.NewLocalizer("plain"))
	lines := strings.Split(view, "\n")
	if len(lines) != q.Height {
		t.Errorf("expected empty view to still render exactly %d lines, got %d", q.Height, len(lines))
	}
}
```

Replace with:

```go
func TestQueuePaneViewEmptyFillsHeight(t *testing.T) {
	q := NewQueuePane()
	q.Width = 40
	q.Height = 8

	view := q.View(NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor)), i18n.NewLocalizer("plain"))
	lines := strings.Split(view, "\n")
	if len(lines) != q.Height {
		t.Errorf("expected empty view to still render exactly %d lines, got %d", q.Height, len(lines))
	}
}

// TestQueuePaneViewBorderUsesThemeGradient is a regression test for the
// visual-effects feature: the queue border must actually vary in color
// (proving a real gradient, not a flat single color) between its
// primary-colored and secondary-colored endpoints.
func TestQueuePaneViewBorderUsesThemeGradient(t *testing.T) {
	q := NewQueuePane()
	q.Width = 40
	q.Height = 8

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	view := q.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
	if !strings.Contains(view, "38;2;0;0;255") {
		t.Errorf("expected the bottom-right corner to be pure blue, got:\n%s", view)
	}
}
```

- [x] **Step 5: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [x] **Step 6: Commit**

```bash
git add internal/ui/queuepane.go internal/ui/queuepane_test.go
git commit -m "QueuePane: gradient border/title; theme-colored progress bar"
```

---

### Task 5: AboutPane gradient border

**Files:**
- Modify: `internal/ui/about.go`
- Create: `internal/ui/about_test.go`

**Interfaces:**
- Consumes: `gradientBox` (Task 1), `Theme.PrimaryColor`/`SecondaryColor` (Task 2)

No previous task touches `about.go`'s `View()` method (Task 1 only removed
the relocated helper functions) — this task changes only the border call,
leaving the logo's own fixed-gradient rendering completely untouched.

- [x] **Step 1: Switch the border rendering**

Find (in `internal/ui/about.go`):

```go
	content := strings.Join(lines, "\n")
	return theme.PaneBorderFocus.Width(a.Width).Height(a.Height).Render(content)
}
```

Replace with:

```go
	content := strings.Join(lines, "\n")
	return gradientBox(content, a.Width, a.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
```

(Height only, same as `BrowserPane`/`QueuePane`: the `-2` accounts for
gradientBox's height being interior-only. Width needs no adjustment — `a.Width`
passes straight through, matching the old `.Width(a.Width).Height(a.Height)`
call's own total rendered width (`a.Width+2`, since `PaneBorderFocus` already
carries `Padding(0, 1)` baked into that budget) exactly.)

- [x] **Step 2: Create `internal/ui/about_test.go`**

No test file exists for `AboutPane` yet. This adds a baseline smoke test
covering the content that's still present after switching the border
mechanism, plus a color-variation check for the new gradient border.

```go
package ui

import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

func TestAboutPaneViewIncludesTaglineAndCloseHint(t *testing.T) {
	a := NewAboutPane()
	a.Width = 60
	a.Height = 20

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	loc := i18n.NewLocalizer("plain")
	view := a.View(theme, loc)

	if !strings.Contains(view, "cyberpunk TUI SCP/SFTP client") {
		t.Errorf("expected the plain-pack tagline in the view, got:\n%s", view)
	}
	if !strings.Contains(view, "[Esc/q] close") {
		t.Errorf("expected the close hint in the view, got:\n%s", view)
	}
}

// TestAboutPaneViewBorderUsesThemeGradient is a regression test for the
// visual-effects feature: the border must actually vary in color between
// its primary-colored and secondary-colored endpoints, not stay one flat
// color.
func TestAboutPaneViewBorderUsesThemeGradient(t *testing.T) {
	a := NewAboutPane()
	a.Width = 60
	a.Height = 20

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	loc := i18n.NewLocalizer("plain")
	view := a.View(theme, loc)

	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
	if !strings.Contains(view, "38;2;0;0;255") {
		t.Errorf("expected the bottom-right corner to be pure blue, got:\n%s", view)
	}
}
```

- [x] **Step 3: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [x] **Step 4: Commit**

```bash
git add internal/ui/about.go internal/ui/about_test.go
git commit -m "AboutPane: gradient border (logo's own gradient untouched)"
```

---

### Task 6: SettingsPane gains a border

**Files:**
- Modify: `internal/ui/settings.go`
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/settings_test.go`

**Interfaces:**
- Consumes: `gradientBox`, `gradientText` (Task 1), `Theme.PrimaryColor`/`SecondaryColor` (Task 2)

Settings currently renders as plain, unbordered text — this task gives it a
border for the first time, wiring up its already-existing (previously
unused) `Width`/`Height` fields the same way `app.go`'s `View()` already
wires `AboutPane.Width`/`Height`.

- [x] **Step 1: Wire `Width`/`Height` in `app.go`**

Find (in `internal/ui/app.go`):

```go
	m.aboutPane.Width = m.width - 4
	m.aboutPane.Height = m.height - queueHeight - 2

	if m.connected || m.testMode {
```

Replace with:

```go
	m.aboutPane.Width = m.width - 4
	m.aboutPane.Height = m.height - queueHeight - 2

	m.settingsPane.Width = m.width - 4
	m.settingsPane.Height = m.height - queueHeight - 2

	if m.connected || m.testMode {
```

- [x] **Step 2: Wrap `SettingsPane.View()`'s output in a gradient border**

Find (in `internal/ui/settings.go`):

```go
func (s *SettingsPane) View(theme Theme, loc *i18n.Localizer) string {
	title := theme.PaneTitle.Render(loc.T("screen_title_settings"))

	rows := []string{title, ""}
```

Replace with:

```go
func (s *SettingsPane) View(theme Theme, loc *i18n.Localizer) string {
	title := gradientText(loc.T("screen_title_settings"), theme.PrimaryColor, theme.SecondaryColor)

	rows := []string{title, ""}
```

Find:

```go
	rows = append(rows, "", theme.PaneTitle.Render(loc.T("settings_hint")))

	return strings.Join(rows, "\n")
}
```

Replace with:

```go
	rows = append(rows, "", theme.PaneTitle.Render(loc.T("settings_hint")))

	content := strings.Join(rows, "\n")
	return gradientBox(content, s.Width, s.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
```

- [x] **Step 3: Update existing tests that call `View()` (if any check exact content only)**

Run: `go test ./internal/ui/... -run TestSettingsPane -v`
Expected: All existing `SettingsPane` tests still PASS — none of them call
`View()` (they test `ResetFromConfig`, `CyclePack`, field navigation,
`Validate`, `PreviewColors` directly), so no test changes are needed here.

- [x] **Step 4: Add a border regression test**

Find (in `internal/ui/settings_test.go`, at the end of the file):

```go
func TestSettingsPanePreviewColorsFallsBackOnIncompleteInput(t *testing.T) {
```

Read the surrounding function to find its closing `}`, then add this new
test immediately after that function's closing brace:

```go
// TestSettingsPaneViewHasGradientBorder is a regression test for the
// visual-effects feature: Settings previously rendered as plain unbordered
// text — it must now be wrapped in a gradient border like every other
// screen.
func TestSettingsPaneViewHasGradientBorder(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")
	s.Width = 40
	s.Height = 12

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	view := s.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("expected a bordered box, got:\n%s", view)
	}
	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
}
```

Add the missing imports at the top of `internal/ui/settings_test.go` if not
already present:

```go
import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)
```

- [x] **Step 5: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [x] **Step 6: Commit**

```bash
git add internal/ui/settings.go internal/ui/app.go internal/ui/settings_test.go
git commit -m "SettingsPane: add gradient border (previously unbordered)"
```

---

### Task 7: HostPickerPane (Site Manager) gains a border

**Files:**
- Modify: `internal/ui/hostpicker.go`
- Modify: `internal/ui/app.go`
- Create: `internal/ui/hostpicker_test.go`

**Interfaces:**
- Consumes: `gradientBox`, `gradientText` (Task 1), `Theme.PrimaryColor`/`SecondaryColor` (Task 2)

No test file exists for `HostPickerPane` yet.

- [x] **Step 1: Wire `Width`/`Height` in `app.go`**

Find (in `internal/ui/app.go`, right after the line added in Task 6):

```go
	m.settingsPane.Width = m.width - 4
	m.settingsPane.Height = m.height - queueHeight - 2

	if m.connected || m.testMode {
```

Replace with:

```go
	m.settingsPane.Width = m.width - 4
	m.settingsPane.Height = m.height - queueHeight - 2

	m.hostPicker.Width = m.width - 4
	m.hostPicker.Height = m.height - queueHeight - 2

	if m.connected || m.testMode {
```

- [x] **Step 2: Wrap `HostPickerPane.View()`'s output in a gradient border**

Find (in `internal/ui/hostpicker.go`):

```go
func (hp *HostPickerPane) View(theme Theme, loc *i18n.Localizer) string {
	if len(hp.Hosts) == 0 {
		return theme.PaneTitle.Render(loc.T("hostpicker_empty"))
	}

	lines := []string{
		theme.PaneTitle.Render(loc.T("hostpicker_header")),
	}

	for i, host := range hp.Hosts {
		prefix := "  "
		style := theme.BrowserFile
		if i == hp.Cursor {
			prefix = "► "
			style = theme.BrowserDir
		}
		line := prefix + style.Render(host.Name) + " (" + host.User + "@" + host.Hostname + ")"
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
```

Replace with:

```go
func (hp *HostPickerPane) View(theme Theme, loc *i18n.Localizer) string {
	var content string
	if len(hp.Hosts) == 0 {
		content = gradientText(loc.T("hostpicker_empty"), theme.PrimaryColor, theme.SecondaryColor)
	} else {
		lines := []string{
			gradientText(loc.T("hostpicker_header"), theme.PrimaryColor, theme.SecondaryColor),
		}

		for i, host := range hp.Hosts {
			prefix := "  "
			style := theme.BrowserFile
			if i == hp.Cursor {
				prefix = "► "
				style = theme.BrowserDir
			}
			line := prefix + style.Render(host.Name) + " (" + host.User + "@" + host.Hostname + ")"
			lines = append(lines, line)
		}

		content = strings.Join(lines, "\n")
	}

	return gradientBox(content, hp.Width, hp.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
```

- [x] **Step 3: Create `internal/ui/hostpicker_test.go`**

```go
package ui

import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

func TestHostPickerPaneViewListsHosts(t *testing.T) {
	hp := NewHostPickerPane()
	hp.Hosts = []config.Host{
		{Name: "wintermute", Hostname: "192.168.1.51", User: "daddy"},
	}
	hp.Width = 40
	hp.Height = 12

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := hp.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "wintermute") {
		t.Errorf("expected the host name in the view, got:\n%s", view)
	}
}

func TestHostPickerPaneViewEmptyStillHasBorder(t *testing.T) {
	hp := NewHostPickerPane()
	hp.Width = 40
	hp.Height = 12

	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
	view := hp.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("expected a bordered box even with no hosts, got:\n%s", view)
	}
}

// TestHostPickerPaneViewBorderUsesThemeGradient is a regression test for
// the visual-effects feature: Host Picker previously rendered as plain
// unbordered text — it must now show an actual color gradient, not a flat
// border.
func TestHostPickerPaneViewBorderUsesThemeGradient(t *testing.T) {
	hp := NewHostPickerPane()
	hp.Hosts = []config.Host{{Name: "wintermute", Hostname: "192.168.1.51", User: "daddy"}}
	hp.Width = 40
	hp.Height = 12

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	view := hp.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
}
```

- [x] **Step 4: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [x] **Step 5: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/app.go internal/ui/hostpicker_test.go
git commit -m "HostPickerPane: add gradient border (previously unbordered)"
```

---

### Task 8: HostFormPane (Add/Edit Host) gains a border

**Files:**
- Modify: `internal/ui/hostform.go`
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/hostform_test.go`

**Interfaces:**
- Consumes: `gradientBox`, `gradientText` (Task 1), `Theme.PrimaryColor`/`SecondaryColor` (Task 2)

- [x] **Step 1: Wire `Width`/`Height` in `app.go`**

Find (in `internal/ui/app.go`, right after the line added in Task 7):

```go
	m.hostPicker.Width = m.width - 4
	m.hostPicker.Height = m.height - queueHeight - 2

	if m.connected || m.testMode {
```

Replace with:

```go
	m.hostPicker.Width = m.width - 4
	m.hostPicker.Height = m.height - queueHeight - 2

	m.hostForm.Width = m.width - 4
	m.hostForm.Height = m.height - queueHeight - 2

	if m.connected || m.testMode {
```

- [x] **Step 2: Wrap `HostFormPane.View()`'s output in a gradient border**

Find (in `internal/ui/hostform.go`):

```go
func (hf *HostFormPane) View(theme Theme, loc *i18n.Localizer) string {
	labelKeys := []string{"host_label_name", "host_label_hostname", "host_label_port", "host_label_user", "host_label_remotepath"}

	header := loc.T("hostform_header_add")
	if hf.IsEditing() {
		header = loc.T("hostform_header_edit")
	}

	lines := []string{
		theme.PaneTitle.Render(header),
		"",
	}
```

Replace with:

```go
func (hf *HostFormPane) View(theme Theme, loc *i18n.Localizer) string {
	labelKeys := []string{"host_label_name", "host_label_hostname", "host_label_port", "host_label_user", "host_label_remotepath"}

	header := loc.T("hostform_header_add")
	if hf.IsEditing() {
		header = loc.T("hostform_header_edit")
	}

	lines := []string{
		gradientText(header, theme.PrimaryColor, theme.SecondaryColor),
		"",
	}
```

Find:

```go
	if hf.errMsg != "" {
		lines = append(lines, "", theme.StatusError.Render(loc.T("error_prefix")+hf.errMsg))
	}

	return strings.Join(lines, "\n")
}
```

Replace with:

```go
	if hf.errMsg != "" {
		lines = append(lines, "", theme.StatusError.Render(loc.T("error_prefix")+hf.errMsg))
	}

	content := strings.Join(lines, "\n")
	return gradientBox(content, hf.Width, hf.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
```

- [x] **Step 3: Run the existing test suite to confirm no regressions**

Run: `go test ./internal/ui/... -run TestBuildHost -v && go test ./internal/ui/... -run TestSave -v`
Expected: `TestBuildHostValidation`, `TestBuildHostDefaultPort`,
`TestSaveEditsByName`, `TestSaveAddsNewHost` all still PASS unchanged — none
of them call `View()`.

- [x] **Step 4: Add a border regression test**

Add to the end of `internal/ui/hostform_test.go`:

```go
// TestHostFormPaneViewHasGradientBorder is a regression test for the
// visual-effects feature: Add/Edit Host previously rendered as plain
// unbordered text — it must now show an actual color gradient border.
func TestHostFormPaneViewHasGradientBorder(t *testing.T) {
	f := newTestForm()
	f.ResetForAdd()
	f.Width = 40
	f.Height = 14

	theme := NewTheme(lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"))
	view := f.View(theme, i18n.NewLocalizer("plain"))

	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("expected a bordered box, got:\n%s", view)
	}
	if !strings.Contains(view, "38;2;255;0;0") {
		t.Errorf("expected the top-left corner to be pure red, got:\n%s", view)
	}
}
```

Add the missing imports at the top of `internal/ui/hostform_test.go`:

```go
import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)
```

- [x] **Step 5: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [x] **Step 6: Commit**

```bash
git add internal/ui/hostform.go internal/ui/app.go internal/ui/hostform_test.go
git commit -m "HostFormPane: add gradient border (previously unbordered)"
```

---

### Task 9: Manual smoke test and doc updates

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

**Interfaces:** none (docs + manual verification only)

- [x] **Step 1: Run the full automated check suite**

Run: `make build && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS across every package, `gofmt -l .`
prints nothing.

- [x] **Step 2: Manual smoke test (not automatable — run and observe)**

```bash
./exfil
```

Walk through, confirming every screen shows a visible color gradient (not a
flat single-color border) and that the local pane (focused) looks visibly
more vivid than the remote pane (unfocused):

1. Local pane (focused): border/title should show a clear gradient from the
   primary color at one corner to the secondary color at the other.
2. `Tab` to the remote pane: its border/title should still gradient, but
   noticeably dimmer than the local pane was.
3. Select a file and press `→`/`←` a few times: the transfer queue's
   progress bar should show a gradient fill (primary→secondary) as it
   moves, not a flat color.
4. Press `s`: Site Manager (Host Picker) should now show a bordered,
   gradient-framed box (previously plain text).
5. Press `n`: Add Host form should also show a bordered, gradient-framed
   box (previously plain text).
6. Press `Esc`, `Esc`, then `S`: Settings screen should also show a
   bordered, gradient-framed box (previously plain text); changing the
   primary/secondary hex values and watching the live preview should show
   the gradient border itself updating live too, not just the flat colors
   it drove before.
7. Press `Esc`, then `?`: About screen's border should gradient the same
   way; the ASCII logo itself should look unchanged (still its own fixed
   cyan→purple).
8. Resize the terminal narrower/shorter and confirm nothing crashes or
   renders a garbled/corrupted border (gradientBox's wrap/pad-as-floor
   behavior should degrade gracefully, matching each pane's pre-existing
   narrow-terminal behavior).

- [x] **Step 3: Update `README.md`**

Add a new bullet to the "Status" list (after the lingo-packs/theme bullet):

```markdown
- ✅ Gradient/neon chrome: pane borders, titles, and the transfer progress bar all render as a color gradient between your chosen primary/secondary theme colors, instead of a flat color
```

- [x] **Step 4: Update `CLAUDE.md`**

Add to the "Current Status" checklist:

```markdown
- ✅ Gradient/neon chrome (`internal/ui/gradient.go`): borders, titles, and the progress bar render as a primary→secondary gradient instead of a flat color; unfocused panes use a muted (50%-toward-black) variant
```

Add a new subsection under "Code Patterns & Guidelines":

```markdown
### Gradient chrome (`internal/ui/gradient.go`)

`gradientBox`/`gradientText` replace lipgloss's single-flat-color border/title styles everywhere a pane is bordered — a `lipgloss.Style` only holds one color, not enough to interpolate a gradient, so `Theme` also stores raw `PrimaryColor`/`SecondaryColor`/`MutedPrimaryColor`/`MutedSecondaryColor` (`lipgloss.Color`) values alongside its derived styles. The gradient runs diagonally (top-left to bottom-right) by character position; focused panes use the vivid primary/secondary pair, unfocused panes use the muted (50%-toward-black) pair. `gradientBox`'s `width`/`height` match `lipgloss.Style`'s own `Width()`/`Height()` convention (interior size, not counting border columns/rows) — width wraps overflowing content (via lipgloss's own reflow), height is a floor that pads shorter content but never truncates taller content. The About screen's ASCII logo keeps its own independent fixed cyan→purple gradient (`gradientLogo`, `logoFrom`/`logoTo`), unrelated to the user's theme colors.
```

- [x] **Step 5: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "Document gradient chrome in README/CLAUDE.md"
```

---

## Self-Review

**Spec coverage:**
- Core gradient rendering primitive (diagonal, Approach A) → Task 1
- `Theme` gains raw gradient color fields, muted variant computation → Task 2
- Browser panes: gradient border/title, vivid-when-focused/muted-when-not → Task 3
- Queue pane: gradient border/title; progress bar uses theme colors → Task 4
- About: gradient border; logo gradient untouched → Task 5
- Settings/Host Picker/Host Form gain a border for the first time → Tasks 6, 7, 8
- Manual verification + docs (including Approach B documented in the spec, not a plan task since it's explicitly deferred) → Task 9

**Placeholder scan:** No "TBD"/"TODO" in any task; every step has complete, runnable code.

**Type consistency check:**
- `gradientBox(content string, width, height int, from, to lipgloss.Color) string` (Task 1) — called identically (same parameter order/types) in Tasks 3, 4, 5, 6, 7, 8.
- `gradientText(s string, from, to lipgloss.Color) string` (Task 1) — same signature used identically in Tasks 3, 4, 6, 7, 8.
- `Theme.PrimaryColor`/`SecondaryColor`/`MutedPrimaryColor`/`MutedSecondaryColor` (Task 2, all `lipgloss.Color`) — referenced with matching names/types in every later task.
- Every pane's `Width`/`Height` fields (pre-existing `int` fields) are passed to `gradientBox` as `pane.Width-2`/`pane.Height-2` consistently across Tasks 3–8, preserving each screen's total rendered size exactly as before.
