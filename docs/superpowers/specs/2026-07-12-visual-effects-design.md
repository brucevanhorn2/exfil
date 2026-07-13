# Visual Effects (Gradient Chrome) — Design

**Branch:** `visual-effects`
**Date:** 2026-07-12

## Goal

Make exfil look more cyberpunk/neon by replacing flat-color chrome (borders, titles,
progress bars) with color gradients derived from the user's existing theme colors
(`primary`/`secondary`, set via the Settings screen). Static gradients only for this
project — no animation (pulsing, blinking, scanlines). That's an explicit follow-up,
not in scope here.

## Out of scope

- Animated effects (pulsing/breathing borders, scanlines, flicker) — a possible
  future project, not this one
- Gradients on file/directory listing text, selection highlight, or semantic
  transfer-status colors (queued/running/done/error) — these stay flat, since they
  carry meaning rather than being purely aesthetic
- Changing the About screen's logo gradient — it keeps its own fixed cyan→purple
  treatment (`internal/ui/about.go`'s existing `gradientLogo`/`logoFrom`/`logoTo`),
  independent of the user's chosen theme colors
- Real "true perimeter" gradient rendering (Approach B below) — documented here so
  it can be picked up later in its own branch, but not implemented in this project

## Scope: which screens get gradient chrome

Every screen that renders a bordered box gets a gradient border + gradient title,
using the diagonal approach described below:

- **Local/remote browser panes** (`internal/ui/browser.go`) — already bordered today
- **Transfer queue** (`internal/ui/queuepane.go`) — already bordered today
- **About** (`internal/ui/about.go`) — already bordered today
- **Settings**, **Host Picker** (Site Manager), **Host Form** (Add/Edit Host) — these
  three currently render as plain, unbordered text. This project gives them a
  border for the first time, with the same gradient treatment as everything else.
  All three already have unused `Width`/`Height` fields on their structs
  (`SettingsPane`, `HostPickerPane`, `HostFormPane`) that this project wires up,
  the same way `AboutPane.Width`/`Height` are already set today in `app.go`'s
  `View()`.

## Focus treatment

Today, a browser pane's border/title is a flat `primary` color when focused and a
flat `secondary` color when unfocused — that's the whole focus signal. This project
keeps that signal but makes both states gradients instead of flat colors:

- **Focused**: vivid gradient, `PrimaryColor → SecondaryColor`
- **Unfocused**: the same gradient, but *muted* — each endpoint color blended 50%
  toward black (`#000000`) before interpolating. There's no reliable way to query
  the user's actual terminal background color, so black is used as a deterministic
  stand-in for "dimmer," not a literal background match.

Screens that are always the sole active view when shown (queue pane, About,
Settings, Host Picker, Host Form) always use the vivid (focused-style) gradient —
there's no "unfocused" state for them to distinguish.

The Settings screen's existing live color-preview (`previewTheme`, built via
`NewTheme(...)` before the user saves) needs no extra plumbing: since `NewTheme`
now also populates the new gradient-related fields, the live preview automatically
renders with its own gradient box.

## Progress bar gradient

The transfer queue's progress bar (`internal/ui/queuepane.go`) already renders via
`github.com/charmbracelet/bubbles/progress`, which has built-in gradient support:

```go
prog := progress.New(progress.WithScaledGradient("#ff00ff", "#00ffff"))
```

Today this is a hardcoded magenta→cyan gradient, unrelated to the user's theme.
This project swaps those two literals for the theme's primary/secondary hex
strings — no new rendering code needed, just reusing the widget's existing feature
with the user's actual colors instead of a fixed pair.

## Architecture: gradient border/title rendering

lipgloss's built-in `Border()`/`BorderForeground()` only support a single flat
color for an entire border — there's no way to get a real per-character gradient
through the existing `Theme` styles (`lipgloss.Style` values), since a `Style` only
carries one foreground/border color. Getting an actual gradient means manually
drawing the border character-by-character with an interpolated color per
character, the same technique the About screen's `gradientLogo` already uses for
its ASCII-art logo text — generalized here to wrap around a bordered *box*, not
just color a single line of text.

### New file: `internal/ui/gradient.go`

Consolidates and generalizes the gradient-math helpers currently private to
`about.go` (`hexToRGB`, `lerp`), plus two new functions:

```go
// hexToRGB and lerp move here from about.go, unchanged, now shared.
func hexToRGB(hex string) (r, g, b int)
func lerp(a, b int, t float64) int

// gradientText colors s left-to-right, from `from` at the first rune to `to` at
// the last — the same technique gradientLogo already uses per-line, generalized
// to a single line of arbitrary (non-logo) text, e.g. a pane's title bar.
func gradientText(s string, from, to lipgloss.Color) string

// gradientBox manually draws a rounded-corner border around content (which is
// pre-built exactly as today — already-styled title/listing/etc. text, untouched
// by this function), sized to width x height, coloring each border character
// by its position along the box's diagonal:
//
//   t := float64(x+y) / float64(width+height-2)   // 0.0 at top-left, 1.0 at bottom-right
//   color := lerp(from, to, t)
//
// Corner glyphs (╭ ╮ ╰ ╯) each get one interpolated color (a corner is a single
// character, so there's no sub-character split to worry about). Content lines
// are padded/truncated to fit exactly like lipgloss's own Width()/Height() do
// today, including the existing 1-space horizontal padding inside the left/
// right border (today's PaneBorder/PaneBorderFocus/QueueBorder styles all set
// Padding(0, 1) — gradientBox reproduces that same inner margin manually, so
// switching to it doesn't shift any existing pane's content by a column).
func gradientBox(content string, width, height int, from, to lipgloss.Color) string
```

### `Theme` struct changes (`internal/ui/theme.go`)

`Theme` currently stores only *derived* `lipgloss.Style` values (each baking in
one flat color) — the gradient renderer needs the raw hex/`lipgloss.Color`
endpoints to interpolate between, so `NewTheme` also stores those directly:

```go
type Theme struct {
	// ... existing fields unchanged ...

	// Raw color values, needed by gradientBox/gradientText (a Style only holds
	// one flat color, not enough to interpolate a gradient from).
	PrimaryColor         lipgloss.Color
	SecondaryColor       lipgloss.Color
	MutedPrimaryColor    lipgloss.Color // PrimaryColor blended 50% toward black
	MutedSecondaryColor  lipgloss.Color // SecondaryColor blended 50% toward black
}
```

### Per-file call-site changes

Each screen's `View()` replaces its `theme.PaneBorder*.Width(w)...Render(content)`
call with `gradientBox(content, w, h, from, to)`, and its title line's style
`.Render(...)` call with `gradientText(...)`:

- `browser.go`: `from, to = theme.PrimaryColor, theme.SecondaryColor` when
  focused; `theme.MutedPrimaryColor, theme.MutedSecondaryColor` when not
- `queuepane.go`: always the vivid pair; also swap the progress bar's hardcoded
  gradient hex literals for `theme.PrimaryColor`/`theme.SecondaryColor`
- `about.go`: always the vivid pair (logo gradient itself untouched)
- `settings.go`, `hostpicker.go`, `hostform.go`: always the vivid pair; each
  wraps its existing plain-text content in `gradientBox` for the first time,
  using its own `Width`/`Height` fields (wired up in `app.go`'s `View()`,
  matching the existing `AboutPane.Width`/`Height` pattern)

## Alternative: Approach B (true perimeter-continuous gradient)

Not implemented in this project — documented here so it can be picked up later
without re-deriving the design. Instead of a diagonal `(x+y)` position, the color
at each border character is based on its actual distance traveled around the
box's perimeter, starting at the top-left corner and going clockwise:

```go
// perimeter = 2*(width-1) + 2*(height-1), the total border length
// For a character at (x, y) on the border, walk clockwise from (0,0) to find
// its distance traveled, then t = distance / perimeter.
```

This produces a continuous "traced outline" look (color flows around the frame)
rather than a diagonal split, at the cost of meaningfully fiddlier position math
(need actual walked distance per edge, not just `x+y`). Visually the difference
is subtle at typical terminal pane sizes (a few dozen columns by a dozen-ish
rows), which is why Approach A (diagonal) was chosen for this project — but if a
future pass wants to try B, `gradientBox`'s position-function (`t := ...`) is the
only piece that needs to change; everything else (padding, corner/edge drawing,
muted-color logic, per-screen wiring) stays the same.

## Testing

- **Unit tests** (`internal/ui/gradient_test.go`, new):
  - `lerp`/`hexToRGB`: endpoint correctness (`t=0` → `from`, `t=1` → `to`) and
    midpoint interpolation
  - `gradientBox`: correct line count (`height`) and visible width, corner
    glyphs present at the four corners, and — since `lipgloss.Color(hex)` emits
    deterministic truecolor ANSI codes — assert the rendered string contains the
    expected `38;2;R;G;Bm` sequences near the start vs. end of the border,
    proving the color actually varies across it rather than staying flat
- **Manual smoke test** (tmux-driven, same technique used earlier on this
  branch's prior work): visually confirm real terminal rendering, that focused
  vs. unfocused panes are still clearly distinguishable, and that muted panes
  stay legible rather than becoming too dark to read
- **Existing pane tests** (`browser_test.go`, `queuepane_test.go`, etc.) get
  updated as needed — `gradientBox` wraps the same content each pane already
  builds, so assertions on inner text should mostly hold; assertions tied to the
  old flat-bordered rendering (if any) get adjusted to match the new gradient
  border

## Error handling

- Invalid/missing theme colors already fall back to `DefaultPrimaryColor`/
  `DefaultSecondaryColor` before `NewTheme` is ever called (existing behavior,
  unchanged) — `gradientBox`/`gradientText` never receive an invalid color
- `gradientBox` given a `width`/`height` too small to fit a border (e.g. 0 or 1)
  degrades the same way `lipgloss`'s own `Width()`/`Height()` already do today
  (clamped/no-op rather than panicking) — matches existing pane-sizing guards
  already in place (e.g. `browser.go`'s `contentHeight < 0` clamp)
