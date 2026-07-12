# Lingo Packs & Theme Settings — Design

**Branch:** `feature/lingo-packs`
**Date:** 2026-07-11

## Goal

Make exfil feel more cyberpunk in two independent, user-selectable ways:

1. **Voice/terminology** — swap the app's UI text between "lingo packs" (flavor presets), full immersion (not just flavor text — key hints, screen titles, transfer status words, etc. all change).
2. **Colors** — let the user set the app's primary and secondary accent colors to any hex value they want, no curated palette.

Both are controlled from a new, dedicated **Settings screen**, kept separate from the (read-only) About screen for SRP.

## Out of scope

- Real language localization (this reuses i18n *structure*, not translation)
- A curated/named color palette — hex entry is free-form
- Per-host or per-session themes — one global choice, persisted in `hosts.yaml`
- Changing semantic colors (transfer done=green, error=red, running=cyan) — only the primary/secondary accent is user-selectable
- Retrofitting old GitHub issues/docs that reference the pre-lingo UI text

## Architecture

### `internal/i18n` package

```go
package i18n

//go:embed locales/*.yaml
var localeFS embed.FS

type Catalog map[string]string

// Loaded once at init from the embedded locale files.
var catalogs map[string]Catalog

var packOrder = []string{"plain", "secretsquirrel", "keyboardcowboy", "corposlut"}

func Packs() []string // returns packOrder, stable order for the Settings screen

type Localizer struct{ pack string }

func NewLocalizer(pack string) *Localizer // falls back to "plain" if pack is unknown
func (l *Localizer) Pack() string
func (l *Localizer) SetPack(pack string) // no-op if pack is unknown
func (l *Localizer) T(messageID string, args ...any) string
```

`T()` looks up `messageID` in the active pack; if that pack doesn't have the key, it falls back to `plain`; if `plain` doesn't have it either, it returns the raw `messageID` (visibly wrong but non-crashing — a signal a key is missing, not a placeholder for missing content). If `args` are given, the resolved template is passed through `fmt.Sprintf`.

### Locale files (`internal/i18n/locales/*.yaml`)

One flat YAML map per pack. `plain.yaml` is the existing professional wording (today's hardcoded strings, extracted verbatim). The other three are full-immersion flavor rewrites. Representative keys (not exhaustive — the full key set is enumerated during implementation by grepping every hardcoded UI string out of `app.go`, `browser.go`, `hostpicker.go`, `hostform.go`, `queuepane.go`, and `about.go`):

| Key | plain | secretsquirrel | keyboardcowboy | corposlut |
|---|---|---|---|---|
| `status_ready` | Ready. | Standing by. | System's clean. Let's ride. | Awaiting directive. |
| `status_connecting` | Connecting to %s… | Establishing uplink to %s… | Jacking into %s… | Negotiating access to %s… |
| `status_connected` | Connected to %s@%s | Uplink secured: %s@%s | You're in: %s@%s | Access granted: %s@%s |
| `status_no_host_selected` | No host selected | No target selected | No system selected, cowboy | No account selected |
| `status_no_files_selected` | No files selected | No assets selected | Nothing tagged | No deliverables selected |
| `status_dir_not_supported` | Directories not supported | Directory extraction not supported | Can't hack a whole directory, slick | Bulk directory transfer requires executive approval |
| `status_host_saved` | Saved host %q | Asset registered: %q | Deck saved: %q | Account provisioned: %q |
| `hint_bar` | [Tab] switch pane [↑/↓] nav [→] push→remote [←] pull←local [↵] enter [⌫] back [space] select [s] hosts [S] settings [?] about [q] quit | [Tab] switch node [↑/↓] nav [→] infil [←] exfil [↵] breach [⌫] retreat [space] mark [s] contacts [S] config [?] dossier [q] extract | [Tab] flip deck [↑/↓] scroll [→] upload [←] download [↵] jack in [⌫] bail [space] tag [s] rolodex [S] settings [?] who am i [q] log off | [Tab] switch division [↑/↓] browse [→] upload [←] download [↵] execute [⌫] escalate [space] flag [s] accounts [S] settings [?] HR [q] clock out |
| `screen_title_hostpicker` | Saved Hosts | Contacts | The Grid | Accounts |
| `screen_title_addhost` | Add Host | New Asset | New Deck | New Account |
| `screen_title_edithost` | Edit Host | Reconfigure Asset | Reflash Deck | Revise Account |
| `screen_title_queue` | Transfer Queue | Operation Log | Data Stream | Deliverables |
| `screen_title_settings` | Settings | Configuration | Rig Setup | Preferences |
| `transfer_status_queued` | queued | staged | queued up | pending |
| `transfer_status_running` | running | in transit | ripping | processing |
| `transfer_status_done` | done | secured | pwned | delivered |
| `transfer_status_error` | error | compromised | crashed | escalated |
| `about_tagline` | cyberpunk TUI SCP/SFTP client | covert data extraction unit | hack the planet | synergizing your data assets |

Host form field labels (Name/Hostname/Port/User/Remote Path) are also catalog keys, following the same pattern, so the theming stays uniform.

## Config changes

`internal/config.Config` gains:

```go
type Config struct {
    Hosts          []Host `yaml:"hosts"`
    Lingo          string `yaml:"lingo,omitempty"`           // default "plain"
    PrimaryColor   string `yaml:"primary_color,omitempty"`   // default "#B341F5", hex
    SecondaryColor string `yaml:"secondary_color,omitempty"` // default "#6E6E6E", hex
}
```

Loaded once at startup in `NewModel()` alongside hosts; empty/missing values default to today's look (so existing `hosts.yaml` files with no `lingo`/color keys are unaffected).

## Theme refactor

`internal/ui/theme.go`'s `NewTheme()` changes signature:

```go
func NewTheme(primary, secondary lipgloss.Color) Theme
```

Every current use of ANSI `"5"` (magenta) becomes `primary`; every use of `"8"` (gray) becomes `secondary`. Colors `"6"` (cyan, directories/running), `"2"` (green, done), `"1"` (red, error), `"7"`/`"0"` (default text/selected-text-on-color) stay hardcoded — they're semantic, not aesthetic.

A `parseHexColor(s string) (lipgloss.Color, error)` helper validates `^#[0-9A-Fa-f]{6}$`; invalid stored hex (e.g. hand-edited `hosts.yaml`) falls back to the default for that slot and logs a warning via the existing logger, rather than crashing.

## Settings screen

New `ScreenSettings` + `SettingsPane` (same architectural pattern as `HostPickerPane`/`HostFormPane` — own `View()`, own `handleSettingsKey` in `app.go`), reachable via **`S`** (capital) from the browsing screen — doesn't collide with lowercase `s` (Site Manager), since bubbletea's `key.String()` distinguishes them.

Three rows:
1. **Lingo Pack** — arrow-cycled (`←`/`→` steps through `i18n.Packs()`)
2. **Primary Color** — `textinput.Model` (same component `HostFormPane` uses), free-form hex entry
3. **Secondary Color** — `textinput.Model`, free-form hex entry

Interaction:
- `Tab`/`Shift+Tab` moves between the three rows (same convention as the host form)
- `←`/`→` only cycles the pack when the **Lingo Pack** row is focused; on the two color rows, `←`/`→` are normal `textinput` cursor movement within the hex text — they don't cycle anything
- Live preview: as soon as a color field's text matches `^#[0-9A-Fa-f]{6}$`, the in-memory `m.theme` is rebuilt immediately so the whole app re-themes live; while the text is incomplete/invalid, the preview holds the last valid color rather than flickering/erroring
- `Enter` saves: re-validates both hex fields (invalid shows an inline error, same pattern as `HostFormPane.errMsg`, and does not close the screen), then persists `Lingo`/`PrimaryColor`/`SecondaryColor` to `hosts.yaml` via `config.Save()` and returns to browsing
- `Esc` discards all changes (reloads `m.theme` from the persisted config) and returns to browsing

The About screen goes back to being purely informational — no settings mutation happens there; its tagline/labels are still pulled from the active lingo pack's catalog, just not editable from that screen.

## Error handling

- Invalid hex on save → inline error, screen stays open (mirrors `HostFormPane`)
- Corrupted hex already in `hosts.yaml` → falls back to default color for that slot, logs a warning, does not crash
- Missing catalog key in a non-plain pack → falls back to `plain`; missing in `plain` too → returns the raw message ID (visible but non-crashing)
- Unknown/corrupted `lingo` value in `hosts.yaml` → `NewLocalizer` falls back to `"plain"`

## Testing

- `internal/i18n`: `Localizer.T()` fallback-to-plain behavior, unknown pack handling, `Packs()` ordering
- `internal/ui/theme.go`: `parseHexColor` valid/invalid cases, `NewTheme(primary, secondary)` applies colors to the right fields
- `internal/ui`: `SettingsPane` row cycling and hex validation, following the same test style as `hostform_test.go`

## Migration notes

Every hardcoded string currently in `app.go`, `browser.go`, `hostpicker.go`, `hostform.go`, `queuepane.go`, and `about.go` that's user-visible gets replaced with `m.loc.T(...)`. This is a straightforward but wide-reaching mechanical change — implementation should grep for the actual current strings (many are already visible in `CLAUDE.md`/`AGENT_GUIDE.md` from this session) to build the complete key list, rather than relying solely on the representative table above.
