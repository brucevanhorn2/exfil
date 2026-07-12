# Lingo Packs & Theme Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let exfil users switch between four "lingo packs" (plain/secretsquirrel/keyboardcowboy/corposlut) that reskin every user-visible string in the app, and freely pick the primary/secondary accent colors by hex, all from a new dedicated Settings screen (`S`).

**Architecture:** A new `internal/i18n` package holds one embedded YAML catalog per pack behind a `Localizer` with fallback-to-`plain` lookup. `internal/config.Config` persists the chosen pack and colors. `internal/ui/theme.go`'s `NewTheme` takes the primary/secondary colors as parameters instead of hardcoding them, and every pane's `View()` takes `theme Theme` (and `loc *i18n.Localizer` where it renders text) as a parameter instead of storing it at construction — this is what lets the whole app re-theme immediately after a Settings save, since nothing is holding a stale copy.

**Tech Stack:** Go 1.26, Bubbletea/Lipgloss (existing), `gopkg.in/yaml.v3` (existing, already used by `internal/config`), Go's `embed` package (stdlib).

## Global Constraints

- No new third-party dependencies — the i18n layer is custom, not `go-i18n` (per the approved design spec).
- Every new user-facing string must go through `internal/i18n`; don't hardcode new UI text.
- Config field names/YAML keys: `lingo`, `primary_color`, `secondary_color`, all `omitempty` so existing `hosts.yaml` files without them are unaffected.
- Match existing test conventions: `t.TempDir()` + `t.Setenv("XDG_CONFIG_HOME", dir)` for isolating config I/O in tests (see `internal/ui/hostform_test.go`).
- `gofmt -l .` and `go vet ./...` must stay clean after every task (CI enforces this).
- Branch: `feature/lingo-packs` (already created, spec already committed there).

---

### Task 1: `internal/i18n` package core + `plain` catalog

**Files:**
- Create: `internal/i18n/i18n.go`
- Create: `internal/i18n/locales/plain.yaml`
- Test: `internal/i18n/i18n_test.go`

**Interfaces:**
- Produces: `i18n.Packs() []string`, `i18n.NewLocalizer(pack string) *Localizer`, `(*Localizer).Pack() string`, `(*Localizer).SetPack(pack string)`, `(*Localizer).T(messageID string, args ...any) string`

- [ ] **Step 1: Write the failing test**

```go
// internal/i18n/i18n_test.go
package i18n

import "testing"

func TestNewLocalizerDefaultsToPlainForUnknownPack(t *testing.T) {
	l := NewLocalizer("nonexistent")
	if l.Pack() != "plain" {
		t.Errorf("expected unknown pack to fall back to plain, got %q", l.Pack())
	}
}

func TestTLooksUpActivePack(t *testing.T) {
	l := NewLocalizer("plain")
	got := l.T("status_ready")
	if got != "Ready." {
		t.Errorf("T(status_ready) = %q, want %q", got, "Ready.")
	}
}

func TestTFormatsArgs(t *testing.T) {
	l := NewLocalizer("plain")
	got := l.T("status_connecting", "wintermute")
	want := "Connecting to wintermute…"
	if got != want {
		t.Errorf("T(status_connecting, ...) = %q, want %q", got, want)
	}
}

func TestTFallsBackToPlainForMissingKeyInPack(t *testing.T) {
	l := NewLocalizer("plain")
	l.pack = "doesnotexist" // bypass SetPack's validation to simulate a pack with no catalog at all
	got := l.T("status_ready")
	if got != "Ready." {
		t.Errorf("expected fallback to plain catalog, got %q", got)
	}
}

func TestTReturnsMessageIDWhenMissingEverywhere(t *testing.T) {
	l := NewLocalizer("plain")
	got := l.T("no_such_key_anywhere")
	if got != "no_such_key_anywhere" {
		t.Errorf("expected raw message ID as last-resort fallback, got %q", got)
	}
}

func TestSetPackRejectsUnknownPack(t *testing.T) {
	l := NewLocalizer("plain")
	l.SetPack("nonexistent")
	if l.Pack() != "plain" {
		t.Errorf("SetPack with unknown pack should be a no-op, got %q", l.Pack())
	}
}

func TestPacksIncludesPlainFirst(t *testing.T) {
	packs := Packs()
	if len(packs) == 0 || packs[0] != "plain" {
		t.Errorf("expected Packs() to start with \"plain\", got %v", packs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/i18n/... -v`
Expected: FAIL — package doesn't exist yet (`no Go files in ...` or similar).

- [ ] **Step 3: Write `internal/i18n/locales/plain.yaml`**

This is the baseline catalog — every key the rest of the plan will reference. Every other pack (Task 2) must define the same key set.

```yaml
status_ready: "Ready."
status_connecting: "Connecting to %s…"
status_connected: "Connected to %s@%s"
status_connection_failed: "Connection to %s failed: %v"
status_read_dir_error: "Error reading dir: %v"
status_no_host_selected: "No host selected"
status_hosts_load_error: "Error loading hosts: %v"
status_host_saved: "Saved host %q"
status_no_files_selected: "No files selected"
status_dir_not_supported: "Directories not supported"
hint_bar: "[Tab] switch pane  [↑/↓] nav  [→] push→remote  [←] pull←local  [↵] enter  [⌫] back  [space] select  [s] hosts  [S] settings  [?] about  [q] quit"
hostpicker_empty: " No hosts configured. Press [n] to add a host. "
hostpicker_header: " Saved Hosts - [↑/↓] navigate, [↵] connect, [n] add, [e] edit "
hostform_header_add: " Add Host - [Tab/Shift+Tab] move  [Enter] save  [Esc] cancel "
hostform_header_edit: " Edit Host - [Tab/Shift+Tab] move  [Enter] save  [Esc] cancel "
host_label_name: "Name"
host_label_hostname: "Hostname"
host_label_port: "Port"
host_label_user: "User"
host_label_remotepath: "Remote Path"
err_name_required: "name is required"
err_hostname_required: "hostname is required"
err_user_required: "user is required"
err_port_invalid: "port must be a number between 1 and 65535"
err_config_load: "failed to load hosts.yaml: %v"
err_config_save: "failed to save hosts.yaml: %v"
error_prefix: "Error: "
screen_title_queue: " Transfer Queue "
queue_empty: "  No transfers"
transfer_status_queued: "queued"
transfer_status_running: "running"
transfer_status_done: "done"
transfer_status_error: "error"
about_tagline: "cyberpunk TUI SCP/SFTP client"
about_label_version: "Version:"
about_label_license: "License:"
about_label_source: "Source:"
about_close_hint: "[Esc/q] close"
screen_title_settings: " Settings "
settings_label_lingo: "Lingo Pack"
settings_label_primary: "Primary Color"
settings_label_secondary: "Secondary Color"
settings_hint: "[Tab/Shift+Tab] move  [←/→] cycle pack  [Enter] save  [Esc] cancel"
```

- [ ] **Step 4: Write `internal/i18n/i18n.go`**

```go
// Package i18n provides "lingo pack" text catalogs for exfil's UI — the same
// structural pattern as conventional internationalization (locale files +
// message-ID lookup + fallback), reused here for tone/flavor packs instead
// of language translation.
package i18n

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localeFS embed.FS

// Catalog maps a message ID to that pack's template string for it.
type Catalog map[string]string

var catalogs map[string]Catalog

// packOrder is the stable display/cycle order for Packs(). "plain" is first
// since it's the fallback and the sensible default.
var packOrder = []string{"plain", "secretsquirrel", "keyboardcowboy", "corposlut"}

func init() {
	catalogs = make(map[string]Catalog, len(packOrder))
	for _, name := range packOrder {
		data, err := localeFS.ReadFile("locales/" + name + ".yaml")
		if err != nil {
			// Embedded files are part of the build; a missing one is a
			// build-time bug, not a runtime condition to recover from.
			panic(fmt.Sprintf("i18n: missing embedded locale %q: %v", name, err))
		}
		var cat Catalog
		if err := yaml.Unmarshal(data, &cat); err != nil {
			panic(fmt.Sprintf("i18n: invalid locale %q: %v", name, err))
		}
		catalogs[name] = cat
	}
}

// Packs returns the available lingo pack names in a stable order, for the
// Settings screen to cycle through.
func Packs() []string {
	return append([]string(nil), packOrder...)
}

func isKnownPack(pack string) bool {
	_, ok := catalogs[pack]
	return ok
}

// Localizer resolves message IDs against one active pack.
type Localizer struct {
	pack string
}

// NewLocalizer returns a Localizer bound to pack, falling back to "plain"
// if pack is unrecognized (e.g. a corrupted hosts.yaml).
func NewLocalizer(pack string) *Localizer {
	if !isKnownPack(pack) {
		pack = "plain"
	}
	return &Localizer{pack: pack}
}

// Pack returns the active pack name.
func (l *Localizer) Pack() string { return l.pack }

// SetPack switches the active pack. No-op if pack is unrecognized.
func (l *Localizer) SetPack(pack string) {
	if isKnownPack(pack) {
		l.pack = pack
	}
}

// T resolves messageID against the active pack, falling back to "plain" if
// the active pack doesn't have that key (lets new packs omit keys during
// development without crashing), and finally to the raw messageID if even
// "plain" doesn't have it (visibly wrong, but never crashes the UI).
// If args are given, the resolved template is passed through fmt.Sprintf.
func (l *Localizer) T(messageID string, args ...any) string {
	msg, ok := catalogs[l.pack][messageID]
	if !ok {
		msg, ok = catalogs["plain"][messageID]
		if !ok {
			return messageID
		}
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/i18n/... -v`
Expected: PASS (all 7 tests)

- [ ] **Step 6: Commit**

```bash
git add internal/i18n/i18n.go internal/i18n/locales/plain.yaml internal/i18n/i18n_test.go
git commit -m "Add internal/i18n package with the plain lingo pack"
```

---

### Task 2: Flavor packs — secretsquirrel, keyboardcowboy, corposlut

**Files:**
- Create: `internal/i18n/locales/secretsquirrel.yaml`
- Create: `internal/i18n/locales/keyboardcowboy.yaml`
- Create: `internal/i18n/locales/corposlut.yaml`
- Modify: `internal/i18n/i18n.go` (restore `packOrder` to all four packs — Task 1 deliberately scoped it down to `{"plain"}` since the other three locale files didn't exist yet; see Task 1's report)
- Modify: `internal/i18n/i18n_test.go` (add parity test)

**Interfaces:**
- Consumes: `Catalog`, `catalogs` (package-internal, from Task 1)

**Note:** Task 1's `packOrder` in `internal/i18n/i18n.go` currently reads `var packOrder = []string{"plain"}` (not all four) — that was a necessary scoping-down for Task 1 to be independently testable, since `//go:embed locales/*.yaml` only embeds files that physically exist, and `init()` panics on a `packOrder` entry with no matching embedded file. This task both adds the three files AND restores `packOrder` to `{"plain", "secretsquirrel", "keyboardcowboy", "corposlut"}` — order matters here.

- [ ] **Step 1: Write the failing tests**

Add to `internal/i18n/i18n_test.go`:

```go
func TestPacksHasFourEntries(t *testing.T) {
	if len(Packs()) != 4 {
		t.Errorf("Packs() = %v, want 4 entries", Packs())
	}
}

func TestNonPlainPacksHaveEveryPlainKey(t *testing.T) {
	plainKeys := catalogs["plain"]
	for _, pack := range Packs() {
		if pack == "plain" {
			continue
		}
		for key := range plainKeys {
			if _, ok := catalogs[pack][key]; !ok {
				t.Errorf("pack %q is missing key %q (present in plain)", pack, key)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/i18n/... -run TestPacksHasFourEntries -v`
Expected: FAIL — `Packs()` currently returns only `["plain"]` (1 entry), since Task 1 scoped `packOrder` down to just that one pack. This is a clean assertion failure, not a panic, because `packOrder` doesn't yet reference the other three (still-missing) locale files.

- [ ] **Step 3: Write `internal/i18n/locales/secretsquirrel.yaml`**

```yaml
status_ready: "Standing by."
status_connecting: "Establishing uplink to %s…"
status_connected: "Uplink secured: %s@%s"
status_connection_failed: "Uplink to %s failed: %v"
status_read_dir_error: "Recon failed: %v"
status_no_host_selected: "No target selected"
status_hosts_load_error: "Failed to load contact list: %v"
status_host_saved: "Asset registered: %q"
status_no_files_selected: "No assets selected"
status_dir_not_supported: "Directory extraction not supported"
hint_bar: "[Tab] switch node  [↑/↓] nav  [→] infil  [←] exfil  [↵] breach  [⌫] retreat  [space] mark  [s] contacts  [S] config  [?] dossier  [q] extract"
hostpicker_empty: " No contacts on file. Press [n] to recruit one. "
hostpicker_header: " Contacts - [↑/↓] navigate, [↵] breach, [n] recruit, [e] reconfigure "
hostform_header_add: " New Asset - [Tab/Shift+Tab] move  [Enter] register  [Esc] abort "
hostform_header_edit: " Reconfigure Asset - [Tab/Shift+Tab] move  [Enter] register  [Esc] abort "
host_label_name: "Codename"
host_label_hostname: "Target"
host_label_port: "Port"
host_label_user: "Alias"
host_label_remotepath: "Drop Point"
err_name_required: "a codename is required"
err_hostname_required: "a target is required"
err_user_required: "an alias is required"
err_port_invalid: "port must be a number between 1 and 65535"
err_config_load: "failed to access contact list: %v"
err_config_save: "failed to update contact list: %v"
error_prefix: "COMPLICATION: "
screen_title_queue: " Operation Log "
queue_empty: "  No operations in progress"
transfer_status_queued: "staged"
transfer_status_running: "in transit"
transfer_status_done: "secured"
transfer_status_error: "compromised"
about_tagline: "covert data extraction unit"
about_label_version: "Build:"
about_label_license: "Clearance:"
about_label_source: "Origin:"
about_close_hint: "[Esc/q] close dossier"
screen_title_settings: " Configuration "
settings_label_lingo: "Cover Identity"
settings_label_primary: "Signal Color"
settings_label_secondary: "Shadow Color"
settings_hint: "[Tab/Shift+Tab] move  [←/→] cycle  [Enter] confirm  [Esc] abort"
```

- [ ] **Step 4: Write `internal/i18n/locales/keyboardcowboy.yaml`**

```yaml
status_ready: "System's clean. Let's ride."
status_connecting: "Jacking into %s…"
status_connected: "You're in: %s@%s"
status_connection_failed: "Couldn't jack into %s: %v"
status_read_dir_error: "Choked reading the directory: %v"
status_no_host_selected: "No system selected, cowboy"
status_hosts_load_error: "Rolodex won't load: %v"
status_host_saved: "Deck saved: %q"
status_no_files_selected: "Nothing tagged"
status_dir_not_supported: "Can't hack a whole directory, slick"
hint_bar: "[Tab] flip deck  [↑/↓] scroll  [→] upload  [←] download  [↵] jack in  [⌫] bail  [space] tag  [s] rolodex  [S] settings  [?] who am i  [q] log off"
hostpicker_empty: " Rolodex is empty. Press [n] to add a deck. "
hostpicker_header: " The Grid - [↑/↓] navigate, [↵] jack in, [n] add, [e] reflash "
hostform_header_add: " New Deck - [Tab/Shift+Tab] move  [Enter] save  [Esc] bail "
hostform_header_edit: " Reflash Deck - [Tab/Shift+Tab] move  [Enter] save  [Esc] bail "
host_label_name: "Handle"
host_label_hostname: "System"
host_label_port: "Port"
host_label_user: "Login"
host_label_remotepath: "Directory"
err_name_required: "you need a handle"
err_hostname_required: "you need a system to hack"
err_user_required: "you need a login"
err_port_invalid: "port must be a number between 1 and 65535"
err_config_load: "rolodex won't load: %v"
err_config_save: "couldn't save the rolodex: %v"
error_prefix: "WHOA: "
screen_title_queue: " Data Stream "
queue_empty: "  Nothing ripping"
transfer_status_queued: "queued up"
transfer_status_running: "ripping"
transfer_status_done: "pwned"
transfer_status_error: "crashed"
about_tagline: "hack the planet"
about_label_version: "Firmware:"
about_label_license: "Rights:"
about_label_source: "Deck:"
about_close_hint: "[Esc/q] log off"
screen_title_settings: " Rig Setup "
settings_label_lingo: "Crew"
settings_label_primary: "Neon"
settings_label_secondary: "Shade"
settings_hint: "[Tab/Shift+Tab] move  [←/→] flip  [Enter] save  [Esc] bail"
```

- [ ] **Step 5: Write `internal/i18n/locales/corposlut.yaml`**

```yaml
status_ready: "Awaiting directive."
status_connecting: "Negotiating access to %s…"
status_connected: "Access granted: %s@%s"
status_connection_failed: "Access to %s denied: %v"
status_read_dir_error: "Directory audit failed: %v"
status_no_host_selected: "No account selected"
status_hosts_load_error: "Failed to load accounts: %v"
status_host_saved: "Account provisioned: %q"
status_no_files_selected: "No deliverables selected"
status_dir_not_supported: "Bulk directory transfer requires executive approval"
hint_bar: "[Tab] switch division  [↑/↓] browse  [→] upload  [←] download  [↵] execute  [⌫] escalate  [space] flag  [s] accounts  [S] settings  [?] HR  [q] clock out"
hostpicker_empty: " No accounts provisioned. Press [n] to onboard one. "
hostpicker_header: " Accounts - [↑/↓] navigate, [↵] execute, [n] add, [e] revise "
hostform_header_add: " New Account - [Tab/Shift+Tab] move  [Enter] submit  [Esc] cancel "
hostform_header_edit: " Revise Account - [Tab/Shift+Tab] move  [Enter] submit  [Esc] cancel "
host_label_name: "Account Name"
host_label_hostname: "Endpoint"
host_label_port: "Port"
host_label_user: "Employee ID"
host_label_remotepath: "Directory"
err_name_required: "an account name is required"
err_hostname_required: "an endpoint is required"
err_user_required: "an employee ID is required"
err_port_invalid: "port must be a number between 1 and 65535"
err_config_load: "failed to load accounts: %v"
err_config_save: "failed to provision account: %v"
error_prefix: "ESCALATION: "
screen_title_queue: " Deliverables "
queue_empty: "  No deliverables pending"
transfer_status_queued: "pending"
transfer_status_running: "processing"
transfer_status_done: "delivered"
transfer_status_error: "escalated"
about_tagline: "synergizing your data assets"
about_label_version: "Release:"
about_label_license: "Compliance:"
about_label_source: "Repository:"
about_close_hint: "[Esc/q] clock out"
screen_title_settings: " Preferences "
settings_label_lingo: "Persona"
settings_label_primary: "Brand Color"
settings_label_secondary: "Accent Color"
settings_hint: "[Tab/Shift+Tab] move  [←/→] cycle  [Enter] submit  [Esc] cancel"
```

- [ ] **Step 6: Restore `packOrder` in `internal/i18n/i18n.go`**

Find:

```go
var packOrder = []string{"plain"}
```

Replace with:

```go
var packOrder = []string{"plain", "secretsquirrel", "keyboardcowboy", "corposlut"}
```

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./internal/i18n/... -v`
Expected: PASS (all 9 tests: the original 7 from Task 1, plus `TestPacksHasFourEntries` and `TestNonPlainPacksHaveEveryPlainKey`)

- [ ] **Step 8: Commit**

```bash
git add internal/i18n/locales/secretsquirrel.yaml internal/i18n/locales/keyboardcowboy.yaml internal/i18n/locales/corposlut.yaml internal/i18n/i18n_test.go internal/i18n/i18n.go
git commit -m "Add secretsquirrel, keyboardcowboy, and corposlut lingo packs"
```

---

### Task 3: Config fields for lingo pack and colors

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (new file)

**Interfaces:**
- Produces: `config.Config{Lingo, PrimaryColor, SecondaryColor string}` fields

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config

import "testing"

func TestSaveLoadRoundTripsLingoAndColors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &Config{
		Hosts:          []Host{{Name: "h", Hostname: "1.2.3.4", Port: 22, User: "u"}},
		Lingo:          "corposlut",
		PrimaryColor:   "#39FF14",
		SecondaryColor: "#3A3A4A",
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Lingo != "corposlut" {
		t.Errorf("Lingo = %q, want %q", got.Lingo, "corposlut")
	}
	if got.PrimaryColor != "#39FF14" {
		t.Errorf("PrimaryColor = %q, want %q", got.PrimaryColor, "#39FF14")
	}
	if got.SecondaryColor != "#3A3A4A" {
		t.Errorf("SecondaryColor = %q, want %q", got.SecondaryColor, "#3A3A4A")
	}
}

func TestLoadDefaultsToEmptyLingoAndColorsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &Config{Hosts: []Host{{Name: "h", Hostname: "1.2.3.4", Port: 22, User: "u"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Lingo != "" || got.PrimaryColor != "" || got.SecondaryColor != "" {
		t.Errorf("expected empty Lingo/PrimaryColor/SecondaryColor for a hosts.yaml predating this feature, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -v`
Expected: FAIL with a compile error (`Config` has no field `Lingo`)

- [ ] **Step 3: Modify `internal/config/config.go`**

Change the `Config` struct (currently at line 19-21):

```go
type Config struct {
	Hosts          []Host `yaml:"hosts"`
	Lingo          string `yaml:"lingo,omitempty"`
	PrimaryColor   string `yaml:"primary_color,omitempty"`
	SecondaryColor string `yaml:"secondary_color,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Add Lingo, PrimaryColor, SecondaryColor to Config"
```

---

### Task 4: Theme refactor — configurable primary/secondary colors

**Files:**
- Modify: `internal/ui/theme.go`
- Modify: `internal/ui/app.go:94` (the one `NewTheme()` call site, updated minimally to keep compiling)
- Test: `internal/ui/theme_test.go` (new file)

**Interfaces:**
- Produces: `NewTheme(primary, secondary lipgloss.Color) Theme`, `parseHexColor(s string) (lipgloss.Color, error)`, `DefaultPrimaryColor`, `DefaultSecondaryColor` (string constants)
- Consumes (unchanged): nothing new

- [ ] **Step 1: Write the failing test**

`lipgloss.Style.GetForeground()` returns a `lipgloss.TerminalColor` interface whose concrete type is `lipgloss.Color` (a `string`-backed type), so comparing it directly to a `lipgloss.Color` value works with no helper needed.

```go
// internal/ui/theme_test.go
package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestParseHexColorValid(t *testing.T) {
	c, err := parseHexColor("#B341F5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != lipgloss.Color("#B341F5") {
		t.Errorf("got %v, want #B341F5", c)
	}
}

func TestParseHexColorInvalid(t *testing.T) {
	tests := []string{"", "B341F5", "#B341F", "#GGGGGG", "purple"}
	for _, in := range tests {
		if _, err := parseHexColor(in); err == nil {
			t.Errorf("parseHexColor(%q): expected error, got none", in)
		}
	}
}

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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestNewThemeAppliesPrimaryAndSecondary -v`
Expected: FAIL with a compile error (`NewTheme` still takes 0 args)

- [ ] **Step 3: Rewrite `internal/ui/theme.go`**

Replace the whole file:

```go
package ui

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/lipgloss"
)

// DefaultPrimaryColor and DefaultSecondaryColor are used when hosts.yaml has
// no saved color choices yet (a fresh install) or an invalid one (hand-edited
// YAML) — matching exfil's original magenta/gray look, but as precise
// true-color hex instead of a 16-color ANSI approximation.
const (
	DefaultPrimaryColor   = "#B341F5"
	DefaultSecondaryColor = "#6E6E6E"
)

var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// parseHexColor validates s as a "#RRGGBB" hex color and returns it as a
// lipgloss.Color. Used for user-supplied colors (Settings screen input, or
// values loaded from hosts.yaml) so invalid input can be caught and reported
// rather than silently misrendering.
func parseHexColor(s string) (lipgloss.Color, error) {
	if !hexColorPattern.MatchString(s) {
		return "", fmt.Errorf("%q is not a valid hex color (expected format #RRGGBB)", s)
	}
	return lipgloss.Color(s), nil
}

type Theme struct {
	// Pane borders
	PaneBorder      lipgloss.Style
	PaneBorderFocus lipgloss.Style
	PaneTitle       lipgloss.Style
	PaneTitleFocus  lipgloss.Style

	// Browser content
	BrowserDir      lipgloss.Style
	BrowserFile     lipgloss.Style
	BrowserSelected lipgloss.Style
	BrowserCursor   lipgloss.Style

	// Queue pane
	QueueBorder     lipgloss.Style
	QueueTitle      lipgloss.Style
	TransferQueued  lipgloss.Style
	TransferRunning lipgloss.Style
	TransferDone    lipgloss.Style
	TransferError   lipgloss.Style
	ProgressBar     lipgloss.Style

	// Status bar
	StatusBar   lipgloss.Style
	StatusKey   lipgloss.Style
	StatusValue lipgloss.Style
	StatusError lipgloss.Style
}

// NewTheme builds the theme from a user-selectable primary color (focus
// borders/titles, selection background, progress bar, status values) and
// secondary color (unfocused borders/titles, queue chrome, queued-status
// text). Cyan/green/red stay fixed here since they carry meaning — running,
// done, and error — rather than being purely aesthetic.
func NewTheme(primary, secondary lipgloss.Color) Theme {
	return Theme{
		// Pane borders
		PaneBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(secondary).
			Foreground(lipgloss.Color("7")).
			Padding(0, 1),

		PaneBorderFocus: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Foreground(lipgloss.Color("7")).
			Padding(0, 1),

		PaneTitle: lipgloss.NewStyle().
			Foreground(secondary).
			Bold(true),

		PaneTitleFocus: lipgloss.NewStyle().
			Foreground(primary).
			Bold(true),

		// Browser items
		BrowserDir: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		BrowserFile: lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")),

		BrowserSelected: lipgloss.NewStyle().
			Background(primary).
			Foreground(lipgloss.Color("0")),

		BrowserCursor: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		// Queue
		QueueBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(secondary).
			Padding(0, 1),

		QueueTitle: lipgloss.NewStyle().
			Foreground(secondary).
			Bold(true),

		TransferQueued: lipgloss.NewStyle().
			Foreground(secondary),

		TransferRunning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")),

		TransferDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")),

		TransferError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),

		ProgressBar: lipgloss.NewStyle().
			Foreground(primary),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")),

		StatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true),

		StatusValue: lipgloss.NewStyle().
			Foreground(primary),

		StatusError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),
	}
}
```

- [ ] **Step 4: Update the call site in `internal/ui/app.go`**

Find (line 94):

```go
	theme := NewTheme()
```

Replace with:

```go
	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
```

(`lipgloss` is already imported in `app.go`.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: All packages PASS, build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/theme.go internal/ui/theme_test.go internal/ui/app.go
git commit -m "Make theme colors configurable via NewTheme(primary, secondary)"
```

---

### Task 5: Wire Localizer and config-driven colors into Model

**Files:**
- Modify: `internal/ui/app.go`
- Test: `internal/ui/app_test.go` (new file)

**Interfaces:**
- Consumes: `i18n.NewLocalizer`, `config.Load`, `parseHexColor`, `DefaultPrimaryColor`, `DefaultSecondaryColor` (Tasks 1, 3, 4)
- Produces: `Model.loc *i18n.Localizer`, `Model.primaryColorHex string`, `Model.secondaryColorHex string` — later tasks read these

- [ ] **Step 1: Write the failing test**

```go
// internal/ui/app_test.go
package ui

import (
	"log"
	"os"
	"testing"

	"github.com/bvanhorn/exfil/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "", 0)
}

func TestNewModelDefaultsWhenConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(make(chan tea.Msg, 1), make(chan tea.Msg, 1), testLogger())

	if m.loc.Pack() != "plain" {
		t.Errorf("expected default pack \"plain\", got %q", m.loc.Pack())
	}
	if m.primaryColorHex != DefaultPrimaryColor {
		t.Errorf("expected default primary color %q, got %q", DefaultPrimaryColor, m.primaryColorHex)
	}
	if m.secondaryColorHex != DefaultSecondaryColor {
		t.Errorf("expected default secondary color %q, got %q", DefaultSecondaryColor, m.secondaryColorHex)
	}
}

func TestNewModelUsesSavedLingoAndColors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.Config{Lingo: "keyboardcowboy", PrimaryColor: "#39FF14", SecondaryColor: "#3A3A4A"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan tea.Msg, 1), testLogger())

	if m.loc.Pack() != "keyboardcowboy" {
		t.Errorf("expected pack \"keyboardcowboy\", got %q", m.loc.Pack())
	}
	if m.primaryColorHex != "#39FF14" {
		t.Errorf("expected primary color #39FF14, got %q", m.primaryColorHex)
	}
}

func TestNewModelFallsBackOnInvalidStoredColor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.Config{PrimaryColor: "not-a-color"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	m := NewModel(make(chan tea.Msg, 1), make(chan tea.Msg, 1), testLogger())

	if m.primaryColorHex != DefaultPrimaryColor {
		t.Errorf("expected fallback to default primary color for invalid stored value, got %q", m.primaryColorHex)
	}
}
```

Note: `tea.Msg` channels are untyped `chan tea.Msg`, matching `NewModel`'s existing signature (`eventsCh chan tea.Msg, jobsCh chan transfer.Job, ...` — check the real signature in `app.go`; `jobsCh` is actually `chan transfer.Job`, not `chan tea.Msg`. Fix the test to match:

```go
	m := NewModel(make(chan tea.Msg, 1), make(chan transfer.Job, 1), testLogger())
```

applied to all three test functions above, with `"github.com/bvanhorn/exfil/internal/transfer"` added to the imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestNewModel -v`
Expected: FAIL with a compile error (`Model` has no field `loc`/`primaryColorHex`/`secondaryColorHex`)

- [ ] **Step 3: Modify `internal/ui/app.go`**

Add the import (in the existing `import (...)` block):

```go
	"github.com/bvanhorn/exfil/internal/i18n"
```

Add fields to `Model` (after `theme Theme` in the struct, currently line 65):

```go
	theme              Theme
	loc                *i18n.Localizer
	primaryColorHex    string
	secondaryColorHex  string
```

Replace the theme-construction block in `NewModel` (currently):

```go
	theme := NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor))
```

with:

```go
	cfg, err := config.Load()
	if err != nil {
		logger.Printf("failed to load hosts.yaml for lingo/theme settings: %v", err)
		cfg = &config.Config{}
	}

	lingo := cfg.Lingo
	if lingo == "" {
		lingo = "plain"
	}
	loc := i18n.NewLocalizer(lingo)

	primaryColorHex := cfg.PrimaryColor
	if primaryColorHex == "" {
		primaryColorHex = DefaultPrimaryColor
	}
	primaryColor, err := parseHexColor(primaryColorHex)
	if err != nil {
		logger.Printf("invalid primary_color %q in hosts.yaml, using default: %v", primaryColorHex, err)
		primaryColorHex = DefaultPrimaryColor
		primaryColor = lipgloss.Color(DefaultPrimaryColor)
	}

	secondaryColorHex := cfg.SecondaryColor
	if secondaryColorHex == "" {
		secondaryColorHex = DefaultSecondaryColor
	}
	secondaryColor, err := parseHexColor(secondaryColorHex)
	if err != nil {
		logger.Printf("invalid secondary_color %q in hosts.yaml, using default: %v", secondaryColorHex, err)
		secondaryColorHex = DefaultSecondaryColor
		secondaryColor = lipgloss.Color(DefaultSecondaryColor)
	}

	theme := NewTheme(primaryColor, secondaryColor)
```

Add `loc`, `primaryColorHex`, `secondaryColorHex` to the `Model{...}` literal in `NewModel` (alongside the existing `theme: theme,` line):

```go
		theme:              theme,
		loc:                loc,
		primaryColorHex:    primaryColorHex,
		secondaryColorHex:  secondaryColorHex,
```

Note: `NewModel` already declares `hostPicker := NewHostPickerPane(theme)` etc. *before* this block in the current file — reorder so the `cfg, err := config.Load()` block above runs *before* those pane constructors are called, since `theme` must be fully constructed first. Move the whole block (from `cfg, err := config.Load()` through `theme := NewTheme(...)`) to just after `localFS := fsys.LocalFS{}` / `home, _ := localFS.Home()` and before `sp := spinner.New()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./internal/ui/... -v`
Expected: PASS (all `TestNewModel*` tests, plus all pre-existing tests still passing)

- [ ] **Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "Wire config-driven lingo pack and theme colors into Model"
```

---

### Task 6: BrowserPane — theme as a View() parameter

**Files:**
- Modify: `internal/ui/browser.go`
- Modify: `internal/ui/browser_test.go`
- Modify: `internal/ui/app.go` (call sites)

**Interfaces:**
- Produces: `(*BrowserPane).View(theme Theme) string` (was `View() string`, no longer stores `theme`)
- Consumes: none new

**Why:** `BrowserPane` currently stores `theme Theme` at construction (`NewBrowserPane(title, fs, theme)`), so once the Settings screen (Task 12) saves new colors, the already-constructed `m.localPane`/`m.remotePane` would keep rendering with their stale original copy forever — `Model` never re-constructs them after startup. Passing `theme` into `View()` each render means it always reflects whatever `m.theme` currently is.

- [ ] **Step 1: Update `internal/ui/browser_test.go` call sites**

Both test functions currently call `NewBrowserPane("test", fsys.LocalFS{}, NewTheme())`. Update to:

```go
b := NewBrowserPane("test", fsys.LocalFS{})
```

(applies in both `TestBrowserPaneBack` and `TestBrowserPaneEnsureVisible`; neither test calls `.View()`, so no other change is needed in this file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestBrowserPane -v`
Expected: FAIL with a compile error (`NewBrowserPane` still takes 3 args)

- [ ] **Step 3: Modify `internal/ui/browser.go`**

Remove `theme Theme` from the struct (currently line 21) and from `NewBrowserPane`'s signature/body:

```go
type BrowserPane struct {
	Title     string
	FS        fsys.FileSystem
	Cwd       string
	Entries   []fsys.Entry
	Cursor    int
	Focus     bool
	Selected  map[string]bool
	Width     int
	Height    int
	scrollTop int
}

func NewBrowserPane(title string, fs fsys.FileSystem) *BrowserPane {
	return &BrowserPane{
		Title:    title,
		FS:       fs,
		Cwd:      "/",
		Selected: make(map[string]bool),
	}
}
```

Change `View()`'s signature and internal references from `b.theme.X` to `theme.X`:

```go
func (b *BrowserPane) View(theme Theme) string {
	titleStyle := theme.PaneTitle
	borderStyle := theme.PaneBorder

	if b.Focus {
		titleStyle = theme.PaneTitleFocus
		borderStyle = theme.PaneBorderFocus
	}

	titleWithPath := titleStyle.Render(fmt.Sprintf(" %s:%s ", b.Title, b.Cwd))

	lines := []string{titleWithPath}

	// -2 for the border's top/bottom lines, -1 for the title line above.
	contentHeight := b.Height - 3
	if contentHeight < 0 {
		contentHeight = 0
	}

	rowsRendered := 0
	for i := b.scrollTop; i < len(b.Entries) && i < b.scrollTop+contentHeight; i++ {
		e := b.Entries[i]
		isCursor := i == b.Cursor && b.Focus
		isSelected := b.Selected[e.Name]

		cursorMark := " "
		if isCursor {
			cursorMark = "►"
		}
		selectMark := " "
		if isSelected {
			selectMark = "☑"
		}
		marker := cursorMark + selectMark + " "

		style := theme.BrowserFile
		if e.IsDir {
			style = theme.BrowserDir
		}
		if isSelected {
			style = theme.BrowserSelected
		}

		line := marker + style.Render(e.Name)
		if e.IsDir {
			line += "/"
		}

		lines = append(lines, line)
		rowsRendered++
	}

	for ; rowsRendered < contentHeight; rowsRendered++ {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	bordered := borderStyle.Width(b.Width).Render(content)
	return bordered
}
```

(All other `BrowserPane` methods — `SetFocus`, `Refresh`, `SetEntries`, `Up`, `Down`, `ensureVisible`, `Enter`, `Back`, `ToggleSelect`, `GetSelectedFiles`, `CurrentFile` — are unchanged; they never used `theme`.)

- [ ] **Step 4: Update call sites in `internal/ui/app.go`**

Find (in `NewModel`):

```go
		localPane:  NewBrowserPane("local", localFS, theme),
		remotePane: NewBrowserPane("remote", fsys.LocalFS{}, theme),
```

Replace with:

```go
		localPane:  NewBrowserPane("local", localFS),
		remotePane: NewBrowserPane("remote", fsys.LocalFS{}),
```

Find (in `View()`):

```go
	localView := m.localPane.View()
	remoteView := m.remotePane.View()
```

Replace with:

```go
	localView := m.localPane.View(m.theme)
	remoteView := m.remotePane.View(m.theme)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ui/browser.go internal/ui/browser_test.go internal/ui/app.go
git commit -m "BrowserPane: take theme as a View() parameter instead of storing it"
```

---

### Task 7: QueuePane — theme/loc as View() parameters, themed status words

**Files:**
- Modify: `internal/ui/queuepane.go`
- Modify: `internal/ui/queuepane_test.go`
- Modify: `internal/ui/app.go` (call sites)

**Interfaces:**
- Produces: `(*QueuePane).View(theme Theme, loc *i18n.Localizer) string`
- Consumes: `i18n.Localizer.T` (Task 1)

**Note:** The current queue pane shows a fixed icon+letter badge per status (`"⏳ Q"`, `"▶ ▶"`, `"✓ ✓"`, `"✗ ✗"`) rather than a text word. Since the design calls for full-immersion terminology and the spec's `transfer_status_*` catalog entries are words (queued/staged/pending, etc.), this task changes the badge from a fixed icon to the localized status word itself (still colored by the existing per-status style), which is what actually makes the lingo pack visible in the queue.

- [ ] **Step 1: Update `internal/ui/queuepane_test.go` call sites**

Both existing tests construct `NewQueuePane(NewTheme())` and call `q.View()`. Update:

```go
q := NewQueuePane()
```

and

```go
view := q.View(NewTheme(lipgloss.Color(DefaultPrimaryColor), lipgloss.Color(DefaultSecondaryColor)), i18n.NewLocalizer("plain"))
```

Add imports to the top of the file:

```go
import (
	"strings"
	"testing"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)
```

(applies to both `TestQueuePaneViewCapsHeight` and `TestQueuePaneViewEmptyFillsHeight`)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestQueuePane -v`
Expected: FAIL with a compile error (`NewQueuePane` still takes 1 arg; `View` still takes 0)

- [ ] **Step 3: Modify `internal/ui/queuepane.go`**

Remove `theme Theme` from the struct and constructor:

```go
type QueuePane struct {
	Transfers []Transfer
	Height    int
	Width     int
}

func NewQueuePane() *QueuePane {
	return &QueuePane{
		Transfers: []Transfer{},
	}
}
```

Add the `i18n` import:

```go
import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/bubbles/progress"
)
```

Update `View()` and `renderTransfer()`:

```go
func (q *QueuePane) View(theme Theme, loc *i18n.Localizer) string {
	title := theme.QueueTitle.Render(loc.T("screen_title_queue"))
	border := theme.QueueBorder

	maxRows := q.Height - 3
	if maxRows < 1 {
		maxRows = 1
	}

	transfers := q.Transfers
	if len(transfers) > maxRows {
		transfers = transfers[len(transfers)-maxRows:]
	}

	lines := []string{title}
	rowsRendered := 0

	if len(transfers) == 0 {
		lines = append(lines, loc.T("queue_empty"))
		rowsRendered++
	} else {
		for _, t := range transfers {
			lines = append(lines, q.renderTransfer(t, theme, loc))
			rowsRendered++
		}
	}

	for ; rowsRendered < maxRows; rowsRendered++ {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return border.Width(q.Width).Render(content)
}

func (q *QueuePane) renderTransfer(t Transfer, theme Theme, loc *i18n.Localizer) string {
	var statusKey string
	statusStyle := theme.TransferQueued

	switch t.Status {
	case StatusQueued:
		statusKey = "transfer_status_queued"
		statusStyle = theme.TransferQueued
	case StatusRunning:
		statusKey = "transfer_status_running"
		statusStyle = theme.TransferRunning
	case StatusDone:
		statusKey = "transfer_status_done"
		statusStyle = theme.TransferDone
	case StatusError:
		statusKey = "transfer_status_error"
		statusStyle = theme.TransferError
	}

	nameWidth := 20
	if len(t.Filename) > nameWidth {
		nameWidth = len(t.Filename)
	}

	name := fmt.Sprintf("%-"+fmt.Sprint(nameWidth)+"s", t.Filename)
	status := statusStyle.Render(fmt.Sprintf("%-10s", loc.T(statusKey)))

	var progressView string
	if t.Total > 0 {
		pct := float64(t.Done) / float64(t.Total)
		prog := progress.New(progress.WithScaledGradient("#ff00ff", "#00ffff"))
		progressView = prog.ViewAs(pct)
	} else {
		progressView = "      "
	}

	sizeStr := fmt.Sprintf("%d/%d", t.Done, t.Total)
	if len(sizeStr) < 15 {
		sizeStr = fmt.Sprintf("%-15s", sizeStr)
	}

	line := fmt.Sprintf("%s %s %s %s %s", status, name, progressView, sizeStr, t.Speed)

	if t.Error != "" {
		line = theme.TransferError.Render(line + " (" + t.Error + ")")
	}

	return line
}
```

(`AddTransfer`, `UpdateTransfer` are unchanged.)

- [ ] **Step 4: Update call sites in `internal/ui/app.go`**

Find:

```go
		queuePane:  NewQueuePane(theme),
```

Replace with:

```go
		queuePane:  NewQueuePane(),
```

Find:

```go
	queueView := m.queuePane.View()
```

Replace with:

```go
	queueView := m.queuePane.View(m.theme, m.loc)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ui/queuepane.go internal/ui/queuepane_test.go internal/ui/app.go
git commit -m "QueuePane: theme/loc as View() parameters, themed status words"
```

---

### Task 8: HostPickerPane — theme/loc as parameters

**Files:**
- Modify: `internal/ui/hostpicker.go`
- Modify: `internal/ui/app.go` (call sites)

**Interfaces:**
- Produces: `(*HostPickerPane).View(theme Theme, loc *i18n.Localizer) string`

- [ ] **Step 1: Modify `internal/ui/hostpicker.go`**

```go
package ui

import (
	"strings"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
)

type HostPickerPane struct {
	Hosts  []config.Host
	Cursor int
	Focus  bool
	Width  int
	Height int
}

func NewHostPickerPane() *HostPickerPane {
	return &HostPickerPane{
		Hosts: []config.Host{},
	}
}

func (hp *HostPickerPane) Load() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	hp.Hosts = cfg.Hosts
	return nil
}

func (hp *HostPickerPane) Up() {
	if hp.Cursor > 0 {
		hp.Cursor--
	}
}

func (hp *HostPickerPane) Down() {
	if hp.Cursor < len(hp.Hosts)-1 {
		hp.Cursor++
	}
}

func (hp *HostPickerPane) CurrentHost() *config.Host {
	if hp.Cursor < 0 || hp.Cursor >= len(hp.Hosts) {
		return nil
	}
	return &hp.Hosts[hp.Cursor]
}

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

- [ ] **Step 2: Update call sites in `internal/ui/app.go`**

Find:

```go
	hostPicker := NewHostPickerPane(theme)
```

Replace with:

```go
	hostPicker := NewHostPickerPane()
```

Find:

```go
		content = m.hostPicker.View()
```

Replace with:

```go
		content = m.hostPicker.View(m.theme, m.loc)
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: All PASS (no existing test directly exercises `HostPickerPane`, so this step is a build/regression check rather than a new assertion)

- [ ] **Step 4: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/app.go
git commit -m "HostPickerPane: theme/loc as View() parameters, themed text"
```

---

### Task 9: HostFormPane — theme/loc as parameters, themed labels and errors

**Files:**
- Modify: `internal/ui/hostform.go`
- Modify: `internal/ui/hostform_test.go`
- Modify: `internal/ui/app.go` (call sites)

**Interfaces:**
- Produces: `NewHostFormPane() *HostFormPane` (was `NewHostFormPane(theme Theme)`), `(*HostFormPane).View(theme Theme, loc *i18n.Localizer) string`

- [ ] **Step 1: Update `internal/ui/hostform_test.go`**

`newTestForm()` currently does `return NewHostFormPane(NewTheme())`. Change to:

```go
func newTestForm() *HostFormPane {
	return NewHostFormPane()
}
```

None of the existing tests call `.View()`, so no other change is needed in this file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestBuildHost -v`
Expected: FAIL with a compile error (`NewHostFormPane` still takes 1 arg)

- [ ] **Step 3: Modify `internal/ui/hostform.go`**

Add the `i18n` import:

```go
import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)
```

Remove `theme Theme` from the struct and constructor:

```go
type HostFormPane struct {
	inputs      []textinput.Model
	focused     hostFormField
	editingName string
	isEditing   bool
	errMsg      string
	Width       int
	Height      int
}

func NewHostFormPane() *HostFormPane {
	labels := map[hostFormField]string{
		fieldName:       "Name",
		fieldHostname:   "Hostname",
		fieldPort:       "Port",
		fieldUser:       "User",
		fieldRemotePath: "Remote Path",
	}

	inputs := make([]textinput.Model, fieldCount)
	for f := hostFormField(0); f < fieldCount; f++ {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = labels[f]
		ti.CharLimit = 256
		inputs[f] = ti
	}

	return &HostFormPane{
		inputs: inputs,
	}
}
```

(The `labels` map here is only used for `textinput.Placeholder` — a faint gray hint shown when a field is empty, not the visible label rendered in `View()`. Leave these as plain English; they're a minor UX nicety, not primary UI text, so they're out of scope for lingo theming.)

`buildHost()`'s error messages need `loc` — change its signature and every call site:

```go
func (hf *HostFormPane) buildHost(loc *i18n.Localizer) (config.Host, error) {
	name := strings.TrimSpace(hf.inputs[fieldName].Value())
	hostname := strings.TrimSpace(hf.inputs[fieldHostname].Value())
	user := strings.TrimSpace(hf.inputs[fieldUser].Value())
	portStr := strings.TrimSpace(hf.inputs[fieldPort].Value())
	remotePath := strings.TrimSpace(hf.inputs[fieldRemotePath].Value())

	if name == "" {
		return config.Host{}, fmt.Errorf("%s", loc.T("err_name_required"))
	}
	if hostname == "" {
		return config.Host{}, fmt.Errorf("%s", loc.T("err_hostname_required"))
	}
	if user == "" {
		return config.Host{}, fmt.Errorf("%s", loc.T("err_user_required"))
	}

	port := config.DefaultPort()
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 || p > 65535 {
			return config.Host{}, fmt.Errorf("%s", loc.T("err_port_invalid"))
		}
		port = p
	}

	return config.Host{
		Name:       name,
		Hostname:   hostname,
		Port:       port,
		User:       user,
		RemotePath: remotePath,
	}, nil
}

func (hf *HostFormPane) Save(loc *i18n.Localizer) (config.Host, error) {
	host, err := hf.buildHost(loc)
	if err != nil {
		hf.errMsg = err.Error()
		return config.Host{}, err
	}

	cfg, err := config.Load()
	if err != nil {
		hf.errMsg = loc.T("err_config_load", err)
		return config.Host{}, err
	}

	replaced := false
	if hf.IsEditing() {
		for i := range cfg.Hosts {
			if cfg.Hosts[i].Name == hf.editingName {
				cfg.Hosts[i] = host
				replaced = true
				break
			}
		}
	}
	if !replaced {
		cfg.Hosts = append(cfg.Hosts, host)
	}

	if err := cfg.Save(); err != nil {
		hf.errMsg = loc.T("err_config_save", err)
		return config.Host{}, err
	}

	hf.errMsg = ""
	return host, nil
}
```

Update `View()`:

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

	for f := hostFormField(0); f < fieldCount; f++ {
		labelStyle := theme.BrowserFile
		if f == hf.focused {
			labelStyle = theme.BrowserDir
		}
		label := labelStyle.Render(fmt.Sprintf("%-12s", loc.T(labelKeys[f])))
		lines = append(lines, label+" "+hf.inputs[f].View())
	}

	if hf.errMsg != "" {
		lines = append(lines, "", theme.StatusError.Render(loc.T("error_prefix")+hf.errMsg))
	}

	return strings.Join(lines, "\n")
}
```

(`ResetForAdd`, `ResetForEdit`, `IsEditing`, `refreshFocus`, `NextField`, `PrevField`, `HandleKey` are unchanged.)

- [ ] **Step 4: Update call sites in `internal/ui/app.go`**

Find:

```go
	hostForm := NewHostFormPane(theme)
```

Replace with:

```go
	hostForm := NewHostFormPane()
```

Find (in `handleHostFormKey`):

```go
	case "enter":
		host, err := m.hostForm.Save()
```

Replace with:

```go
	case "enter":
		host, err := m.hostForm.Save(m.loc)
```

Find (in `View()`):

```go
		content = m.hostForm.View()
```

Replace with:

```go
		content = m.hostForm.View(m.theme, m.loc)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ui/hostform.go internal/ui/hostform_test.go internal/ui/app.go
git commit -m "HostFormPane: theme/loc as parameters, themed labels and errors"
```

---

### Task 10: AboutPane — theme/loc as parameters, themed tagline/labels

**Files:**
- Modify: `internal/ui/about.go`
- Modify: `internal/ui/app.go` (call sites)

**Interfaces:**
- Produces: `NewAboutPane() *AboutPane` (was `NewAboutPane(theme Theme)`), `(*AboutPane).View(theme Theme, loc *i18n.Localizer) string`

- [ ] **Step 1: Modify `internal/ui/about.go`**

Add the `i18n` import and remove the stored `theme` field:

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/bvanhorn/exfil/internal/version"
	"github.com/charmbracelet/lipgloss"
)

// logo is a "bigmono12"-style ASCII rendering of "exfil" (via
// `toilet -f bigmono12`), colored at render time with a gradient instead of
// baking in ANSI codes here.
const logo = `                                  ██
                       ▒████      ██     ████
                       █████      ██     ████
                       ██                  ██
  ░████▒   ███  ███  ███████    ████       ██
 ░██████▒   ██▒▒██   ███████    ████       ██
 ██▒  ▒██   ▒████▒     ██         ██       ██
 ████████    ████      ██         ██       ██
 ████████    ▒██▒      ██         ██       ██
 ██          ████      ██         ██       ██
 ███░  ▒█   ▒████▒     ██         ██       ██▒
 ░███████   ██▒▒██     ██      ████████    █████
  ░█████▒  ███  ███    ██      ████████    ░████  `

const (
	logoFrom = "#00E5FF"
	logoTo   = "#B341F5"
)

type AboutPane struct {
	Width  int
	Height int
}

func NewAboutPane() *AboutPane {
	return &AboutPane{}
}

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
	lines := []string{
		gradientLogo(logo, logoFrom, logoTo),
		"",
		theme.BrowserFile.Render(loc.T("about_tagline")),
		"",
		theme.BrowserDir.Render(fmt.Sprintf("%-10s", loc.T("about_label_version"))) + version.Version,
		theme.BrowserDir.Render(fmt.Sprintf("%-10s", loc.T("about_label_license"))) + "MIT",
		theme.BrowserDir.Render(fmt.Sprintf("%-10s", loc.T("about_label_source"))) + "github.com/brucevanhorn2/exfil",
		"",
		theme.PaneTitle.Render(loc.T("about_close_hint")),
	}

	content := strings.Join(lines, "\n")
	return theme.PaneBorderFocus.Width(a.Width).Height(a.Height).Render(content)
}
```

- [ ] **Step 2: Update call sites in `internal/ui/app.go`**

Find:

```go
	aboutPane := NewAboutPane(theme)
```

Replace with:

```go
	aboutPane := NewAboutPane()
```

Find:

```go
		content = m.aboutPane.View()
```

Replace with:

```go
		content = m.aboutPane.View(m.theme, m.loc)
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ui/about.go internal/ui/app.go
git commit -m "AboutPane: theme/loc as parameters, themed tagline/labels"
```

---

### Task 11: SettingsPane

**Files:**
- Create: `internal/ui/settings.go`
- Create: `internal/ui/settings_test.go`

**Interfaces:**
- Consumes: `i18n.Packs`, `i18n.Localizer` (Task 1), `parseHexColor` (Task 4)
- Produces: `NewSettingsPane() *SettingsPane`, `(*SettingsPane).ResetFromConfig(lingo, primaryHex, secondaryHex string)`, `.NextField()`, `.PrevField()`, `.Focused() settingsField`, `.CyclePack(delta int)`, `.CurrentPack() string`, `.HandleKey(msg tea.KeyMsg) tea.Cmd`, `.PreviewColors(fallbackPrimary, fallbackSecondary string) (string, string)`, `.Validate() error`, `.PrimaryValue() string`, `.SecondaryValue() string`, `.View(theme Theme, loc *i18n.Localizer) string`

- [ ] **Step 1: Write the failing test**

```go
// internal/ui/settings_test.go
package ui

import (
	"testing"

	"github.com/bvanhorn/exfil/internal/i18n"
)

func TestSettingsPaneResetFromConfigFindsPackIndex(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("keyboardcowboy", "#39FF14", "#3A3A4A")
	if s.CurrentPack() != "keyboardcowboy" {
		t.Errorf("CurrentPack() = %q, want %q", s.CurrentPack(), "keyboardcowboy")
	}
	if s.PrimaryValue() != "#39FF14" {
		t.Errorf("PrimaryValue() = %q, want %q", s.PrimaryValue(), "#39FF14")
	}
}

func TestSettingsPaneResetFromConfigUnknownPackDefaultsToFirst(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("nonexistent", "#39FF14", "#3A3A4A")
	if s.CurrentPack() != i18n.Packs()[0] {
		t.Errorf("expected fallback to first pack, got %q", s.CurrentPack())
	}
}

func TestSettingsPaneCyclePackWrapsAround(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")

	packs := i18n.Packs()
	s.CyclePack(-1) // from index 0, should wrap to the last pack
	if s.CurrentPack() != packs[len(packs)-1] {
		t.Errorf("CyclePack(-1) from first pack = %q, want %q (wrap to last)", s.CurrentPack(), packs[len(packs)-1])
	}

	s.CyclePack(1) // back to first
	if s.CurrentPack() != packs[0] {
		t.Errorf("CyclePack(1) = %q, want %q", s.CurrentPack(), packs[0])
	}
}

func TestSettingsPaneFieldNavigationWraps(t *testing.T) {
	s := NewSettingsPane()
	if s.Focused() != settingsFieldLingo {
		t.Fatalf("expected initial focus on Lingo row, got %v", s.Focused())
	}
	s.NextField()
	if s.Focused() != settingsFieldPrimary {
		t.Errorf("expected focus on Primary after one NextField, got %v", s.Focused())
	}
	s.NextField()
	if s.Focused() != settingsFieldSecondary {
		t.Errorf("expected focus on Secondary after two NextField, got %v", s.Focused())
	}
	s.NextField()
	if s.Focused() != settingsFieldLingo {
		t.Errorf("expected wrap back to Lingo after three NextField, got %v", s.Focused())
	}
	s.PrevField()
	if s.Focused() != settingsFieldSecondary {
		t.Errorf("expected wrap to Secondary after PrevField from Lingo, got %v", s.Focused())
	}
}

func TestSettingsPaneValidateRejectsInvalidHex(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "not-a-color", "#6E6E6E")
	if err := s.Validate(); err == nil {
		t.Error("expected Validate() to reject an invalid primary color, got nil")
	}
}

func TestSettingsPaneValidateAcceptsValidHex(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")
	if err := s.Validate(); err != nil {
		t.Errorf("expected Validate() to accept valid hex colors, got: %v", err)
	}
}

func TestSettingsPanePreviewColorsFallsBackOnIncompleteInput(t *testing.T) {
	s := NewSettingsPane()
	s.ResetFromConfig("plain", "#B341F5", "#6E6E6E")
	s.primaryInput.SetValue("#B3") // incomplete, mid-typing

	primary, secondary := s.PreviewColors("#000000", "#111111")
	if primary != "#000000" {
		t.Errorf("expected incomplete input to hold fallback %q, got %q", "#000000", primary)
	}
	if secondary != "#6E6E6E" {
		t.Errorf("expected valid secondary input %q to be used, got %q", "#6E6E6E", secondary)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestSettingsPane -v`
Expected: FAIL — package doesn't have `settings.go` yet (compile error, `NewSettingsPane` undefined)

- [ ] **Step 3: Write `internal/ui/settings.go`**

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/bvanhorn/exfil/internal/i18n"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// settingsField indexes the rows in the Settings screen, in Tab order.
type settingsField int

const (
	settingsFieldLingo settingsField = iota
	settingsFieldPrimary
	settingsFieldSecondary
	settingsFieldCount
)

// SettingsPane is the dedicated Settings screen: lingo pack (arrow-cycled)
// and primary/secondary theme colors (free-form hex text entry). Kept
// separate from AboutPane (read-only) for single responsibility.
type SettingsPane struct {
	packIndex      int
	focused        settingsField
	primaryInput   textinput.Model
	secondaryInput textinput.Model
	errMsg         string
	Width          int
	Height         int
}

func NewSettingsPane() *SettingsPane {
	primary := textinput.New()
	primary.Prompt = ""
	primary.CharLimit = 7

	secondary := textinput.New()
	secondary.Prompt = ""
	secondary.CharLimit = 7

	return &SettingsPane{
		primaryInput:   primary,
		secondaryInput: secondary,
	}
}

// ResetFromConfig populates the pane from the model's current settings.
// Called every time the Settings screen is opened, so it never shows stale
// edits left over from a previously cancelled visit.
func (s *SettingsPane) ResetFromConfig(lingo, primaryHex, secondaryHex string) {
	s.packIndex = 0
	for i, p := range i18n.Packs() {
		if p == lingo {
			s.packIndex = i
			break
		}
	}
	s.primaryInput.SetValue(primaryHex)
	s.secondaryInput.SetValue(secondaryHex)
	s.errMsg = ""
	s.focused = settingsFieldLingo
	s.refreshFocus()
}

func (s *SettingsPane) refreshFocus() {
	if s.focused == settingsFieldPrimary {
		s.primaryInput.Focus()
	} else {
		s.primaryInput.Blur()
	}
	if s.focused == settingsFieldSecondary {
		s.secondaryInput.Focus()
	} else {
		s.secondaryInput.Blur()
	}
}

// Focused reports which row currently has focus.
func (s *SettingsPane) Focused() settingsField {
	return s.focused
}

// NextField moves focus to the next row, wrapping around.
func (s *SettingsPane) NextField() {
	s.focused = (s.focused + 1) % settingsFieldCount
	s.refreshFocus()
}

// PrevField moves focus to the previous row, wrapping around.
func (s *SettingsPane) PrevField() {
	s.focused = (s.focused - 1 + settingsFieldCount) % settingsFieldCount
	s.refreshFocus()
}

// CyclePack moves the Lingo Pack selection by delta (+1 or -1), wrapping
// around. Only meaningful when the Lingo Pack row is focused — the caller
// (Model.handleSettingsKey) is responsible for only calling this when
// Focused() == settingsFieldLingo.
func (s *SettingsPane) CyclePack(delta int) {
	packs := i18n.Packs()
	n := len(packs)
	s.packIndex = ((s.packIndex+delta)%n + n) % n
}

// CurrentPack returns the currently-selected (not yet necessarily saved)
// lingo pack name.
func (s *SettingsPane) CurrentPack() string {
	return i18n.Packs()[s.packIndex]
}

// HandleKey forwards a key message to whichever color textinput has focus.
// A no-op if the Lingo Pack row is focused (it has no textinput).
func (s *SettingsPane) HandleKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	switch s.focused {
	case settingsFieldPrimary:
		s.primaryInput, cmd = s.primaryInput.Update(msg)
	case settingsFieldSecondary:
		s.secondaryInput, cmd = s.secondaryInput.Update(msg)
	}
	return cmd
}

// PreviewColors returns the two color rows' current text if each is
// syntactically valid hex, else the corresponding fallback — so a live
// preview can hold the last valid color instead of erroring/flickering
// while the user is mid-edit.
func (s *SettingsPane) PreviewColors(fallbackPrimary, fallbackSecondary string) (string, string) {
	primary := fallbackPrimary
	if _, err := parseHexColor(s.primaryInput.Value()); err == nil {
		primary = s.primaryInput.Value()
	}
	secondary := fallbackSecondary
	if _, err := parseHexColor(s.secondaryInput.Value()); err == nil {
		secondary = s.secondaryInput.Value()
	}
	return primary, secondary
}

// Validate checks both color fields are valid hex. On failure, errMsg is
// set (surfaced by View()) and the same error is returned so the caller
// knows not to save/close.
func (s *SettingsPane) Validate() error {
	if _, err := parseHexColor(s.primaryInput.Value()); err != nil {
		s.errMsg = fmt.Sprintf("primary color: %v", err)
		return err
	}
	if _, err := parseHexColor(s.secondaryInput.Value()); err != nil {
		s.errMsg = fmt.Sprintf("secondary color: %v", err)
		return err
	}
	s.errMsg = ""
	return nil
}

func (s *SettingsPane) PrimaryValue() string   { return s.primaryInput.Value() }
func (s *SettingsPane) SecondaryValue() string { return s.secondaryInput.Value() }

func (s *SettingsPane) View(theme Theme, loc *i18n.Localizer) string {
	title := theme.PaneTitle.Render(loc.T("screen_title_settings"))

	rows := []string{title, ""}

	lingoStyle := theme.BrowserFile
	if s.focused == settingsFieldLingo {
		lingoStyle = theme.BrowserDir
	}
	rows = append(rows, lingoStyle.Render(fmt.Sprintf("%-16s", loc.T("settings_label_lingo")))+" ◄ "+s.CurrentPack()+" ►")

	primaryStyle := theme.BrowserFile
	if s.focused == settingsFieldPrimary {
		primaryStyle = theme.BrowserDir
	}
	rows = append(rows, primaryStyle.Render(fmt.Sprintf("%-16s", loc.T("settings_label_primary")))+" "+s.primaryInput.View())

	secondaryStyle := theme.BrowserFile
	if s.focused == settingsFieldSecondary {
		secondaryStyle = theme.BrowserDir
	}
	rows = append(rows, secondaryStyle.Render(fmt.Sprintf("%-16s", loc.T("settings_label_secondary")))+" "+s.secondaryInput.View())

	if s.errMsg != "" {
		rows = append(rows, "", theme.StatusError.Render(loc.T("error_prefix")+s.errMsg))
	}

	rows = append(rows, "", theme.PaneTitle.Render(loc.T("settings_hint")))

	return strings.Join(rows, "\n")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/... -run TestSettingsPane -v`
Expected: PASS (all 7 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/ui/settings.go internal/ui/settings_test.go
git commit -m "Add SettingsPane: lingo pack cycling, free-form hex colors"
```

---

### Task 12: Wire the Settings screen into Model

**Files:**
- Modify: `internal/ui/app.go`

**Interfaces:**
- Consumes: `NewSettingsPane`, `SettingsPane.*` (Task 11)
- Produces: `ScreenSettings`, `Model.settingsPane`, `Model.handleSettingsKey`

- [ ] **Step 1: Add the screen constant**

Find (currently line 23-28):

```go
const (
	ScreenBrowsing   Screen = "browsing"
	ScreenHostPicker Screen = "hostpicker"
	ScreenAddHost    Screen = "addhost"
	ScreenAbout      Screen = "about"
)
```

Replace with:

```go
const (
	ScreenBrowsing   Screen = "browsing"
	ScreenHostPicker Screen = "hostpicker"
	ScreenAddHost    Screen = "addhost"
	ScreenAbout      Screen = "about"
	ScreenSettings   Screen = "settings"
)
```

- [ ] **Step 2: Add the field and construction**

Add to the `Model` struct (alongside `aboutPane *AboutPane`):

```go
	aboutPane    *AboutPane
	settingsPane *SettingsPane
```

Add to `NewModel`, alongside `aboutPane := NewAboutPane()`:

```go
	aboutPane := NewAboutPane()
	settingsPane := NewSettingsPane()
```

Add to the `Model{...}` literal, alongside `aboutPane: aboutPane,`:

```go
		aboutPane:    aboutPane,
		settingsPane: settingsPane,
```

- [ ] **Step 3: Route the key and add the handler**

Find (in `Update()`):

```go
		if m.screen == ScreenAbout {
			return m.handleAboutKey(msg)
		}
		return m.handleBrowsingKey(msg)
```

Replace with:

```go
		if m.screen == ScreenAbout {
			return m.handleAboutKey(msg)
		}
		if m.screen == ScreenSettings {
			return m.handleSettingsKey(msg)
		}
		return m.handleBrowsingKey(msg)
```

Find (in `handleBrowsingKey`, the `case "?":` line):

```go
	case "?":
		m.screen = ScreenAbout
```

Replace with:

```go
	case "?":
		m.screen = ScreenAbout
	case "S":
		m.settingsPane.ResetFromConfig(m.loc.Pack(), m.primaryColorHex, m.secondaryColorHex)
		m.screen = ScreenSettings
```

Add a new handler function, right after `handleAboutKey`:

```go
// handleSettingsKey handles keys in the Settings screen.
func (m *Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Discard: rebuild the theme from the last persisted values.
		m.applyTheme(m.primaryColorHex, m.secondaryColorHex)
		m.screen = ScreenBrowsing
		return m, nil
	case "tab":
		m.settingsPane.NextField()
		return m, nil
	case "shift+tab":
		m.settingsPane.PrevField()
		return m, nil
	case "left":
		if m.settingsPane.Focused() == settingsFieldLingo {
			m.settingsPane.CyclePack(-1)
			return m, nil
		}
	case "right":
		if m.settingsPane.Focused() == settingsFieldLingo {
			m.settingsPane.CyclePack(1)
			return m, nil
		}
	case "enter":
		if err := m.settingsPane.Validate(); err != nil {
			// Error shown inline on the form; stay put.
			return m, nil
		}
		m.loc.SetPack(m.settingsPane.CurrentPack())
		m.primaryColorHex = m.settingsPane.PrimaryValue()
		m.secondaryColorHex = m.settingsPane.SecondaryValue()
		m.applyTheme(m.primaryColorHex, m.secondaryColorHex)

		cfg, err := config.Load()
		if err != nil {
			// A genuine parse failure (not "file missing") means we don't
			// know what's in hosts.yaml — saving here would overwrite it
			// with only the new lingo/theme fields and silently drop the
			// existing Hosts list. Abort instead, matching HostFormPane.Save().
			m.statusMsg = fmt.Sprintf("Error loading hosts.yaml, settings not saved: %v", err)
			m.screen = ScreenBrowsing
			return m, nil
		}
		cfg.Lingo = m.loc.Pack()
		cfg.PrimaryColor = m.primaryColorHex
		cfg.SecondaryColor = m.secondaryColorHex
		if err := cfg.Save(); err != nil {
			m.statusMsg = fmt.Sprintf("Error saving settings: %v", err)
		}
		m.screen = ScreenBrowsing
		return m, nil
	}
	// Any other key (character input) goes to whichever color textinput is
	// focused; a no-op if the Lingo Pack row is focused.
	cmd := m.settingsPane.HandleKey(msg)
	return m, cmd
}

// applyTheme rebuilds m.theme from the given hex colors. Both are assumed
// already-valid (either defaults, previously-persisted values, or freshly
// validated by SettingsPane.Validate), so parse errors here are unexpected
// and fall back to the package defaults rather than crash.
func (m *Model) applyTheme(primaryHex, secondaryHex string) {
	primary, err := parseHexColor(primaryHex)
	if err != nil {
		primary = lipgloss.Color(DefaultPrimaryColor)
	}
	secondary, err := parseHexColor(secondaryHex)
	if err != nil {
		secondary = lipgloss.Color(DefaultSecondaryColor)
	}
	m.theme = NewTheme(primary, secondary)
}
```

- [ ] **Step 4: Render the Settings screen**

Find (in `View()`):

```go
	if m.screen == ScreenHostPicker {
		content = m.hostPicker.View(m.theme, m.loc)
	} else if m.screen == ScreenAddHost {
		content = m.hostForm.View(m.theme, m.loc)
	} else if m.screen == ScreenAbout {
		content = m.aboutPane.View(m.theme, m.loc)
	}
```

Replace with:

```go
	if m.screen == ScreenHostPicker {
		content = m.hostPicker.View(m.theme, m.loc)
	} else if m.screen == ScreenAddHost {
		content = m.hostForm.View(m.theme, m.loc)
	} else if m.screen == ScreenAbout {
		content = m.aboutPane.View(m.theme, m.loc)
	} else if m.screen == ScreenSettings {
		previewPrimary, previewSecondary := m.settingsPane.PreviewColors(m.primaryColorHex, m.secondaryColorHex)
		primary, err := parseHexColor(previewPrimary)
		if err != nil {
			primary = lipgloss.Color(DefaultPrimaryColor)
		}
		secondary, err := parseHexColor(previewSecondary)
		if err != nil {
			secondary = lipgloss.Color(DefaultSecondaryColor)
		}
		previewTheme := NewTheme(primary, secondary)
		content = m.settingsPane.View(previewTheme, m.loc)
	}
```

(`previewPrimary`/`previewSecondary` are always syntactically valid hex — either the current textinput value or the fallback `m.primaryColorHex`/`m.secondaryColorHex`, both already-validated — so the `err != nil` branches here are unreachable in practice but kept for the same defensive reason as `applyTheme`.)

- [ ] **Step 5: Run the full build/test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/app.go
git commit -m "Wire the Settings screen into Model (key S, live color preview)"
```

**Post-implementation correction (found in review, applied as a follow-up
commit rather than amending):** Step 3's `"enter"` case as originally written
above fell back to a fresh `&config.Config{}` when `config.Load()` returned a
non-nil error, then saved that empty config — silently overwriting
`hosts.yaml` and destroying its `Hosts` list if the file existed but failed
to parse (a real error, not just "file missing", which `config.Load()`
already handles safely). The code above has been corrected in place to abort
and surface an error instead, matching `HostFormPane.Save()`'s existing
pattern. Add a regression test to `internal/ui/app_test.go` alongside the
other `NewModel`/settings tests: write a syntactically-invalid `hosts.yaml`
after model construction, invoke `handleSettingsKey` with `KeyEnter`, and
assert the file is unchanged and `m.statusMsg` reports the error.

---

### Task 13: Replace remaining hardcoded status/hint strings with loc.T()

**Files:**
- Modify: `internal/ui/app.go`

**Interfaces:**
- Consumes: `m.loc.T` (all prior tasks)

- [ ] **Step 1: Replace the initial status message**

Find (in `NewModel`'s `Model{...}` literal):

```go
		statusMsg:  "Ready.",
```

Replace with:

```go
		statusMsg:  loc.T("status_ready"),
```

- [ ] **Step 2: Replace connection status messages**

Find (in the `sshConnectedMsg` case of `Update()`):

```go
	case sshConnectedMsg:
		m.connecting = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Connection to %s failed: %v", msg.host.Name, msg.err)
			m.logger.Printf("ssh dial %s: %v", msg.host.Name, msg.err)
			return m, nil
		}
```

Replace with:

```go
	case sshConnectedMsg:
		m.connecting = false
		if msg.err != nil {
			m.statusMsg = m.loc.T("status_connection_failed", msg.host.Name, msg.err)
			m.logger.Printf("ssh dial %s: %v", msg.host.Name, msg.err)
			return m, nil
		}
```

Find:

```go
		m.statusMsg = fmt.Sprintf("Connected to %s@%s", msg.host.User, msg.host.Hostname)
```

Replace with:

```go
		m.statusMsg = m.loc.T("status_connected", msg.host.User, msg.host.Hostname)
```

- [ ] **Step 3: Replace the read-dir error message**

Find (in the `readDirMsg` case of `Update()`):

```go
	case readDirMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error reading dir: %v", msg.err)
		}
```

Replace with:

```go
	case readDirMsg:
		if msg.err != nil {
			m.statusMsg = m.loc.T("status_read_dir_error", msg.err)
		}
```

- [ ] **Step 4: Replace the hint bar and hosts-load-error messages**

Find (in `handleBrowsingKey`):

```go
	case "s":
		// Open the Site Manager overlay to pick a host to connect to.
		if err := m.hostPicker.Load(); err != nil {
			m.statusMsg = fmt.Sprintf("Error loading hosts: %v", err)
		}
		m.screen = ScreenHostPicker
```

Replace with:

```go
	case "s":
		// Open the Site Manager overlay to pick a host to connect to.
		if err := m.hostPicker.Load(); err != nil {
			m.statusMsg = m.loc.T("status_hosts_load_error", err)
		}
		m.screen = ScreenHostPicker
```

Find (in `View()`, the hints bar):

```go
	if m.screen == ScreenBrowsing {
		hintsBar := m.theme.StatusBar.Render(
			"[Tab] switch pane  [↑/↓] nav  [→] push→remote  [←] pull←local  [↵] enter  [⌫] back  [space] select  [s] hosts  [?] about  [q] quit",
		)
		footer = lipgloss.JoinVertical(lipgloss.Left, footer, hintsBar)
	}
```

Replace with:

```go
	if m.screen == ScreenBrowsing {
		hintsBar := m.theme.StatusBar.Render(m.loc.T("hint_bar"))
		footer = lipgloss.JoinVertical(lipgloss.Left, footer, hintsBar)
	}
```

- [ ] **Step 5: Replace "No host selected" and the reload-error message**

Find (in `handleHostPickerKey`, both occurrences of `"No host selected"`):

```go
	case "e":
		host := m.hostPicker.CurrentHost()
		if host == nil {
			m.statusMsg = "No host selected"
			return m, nil
		}
```

and

```go
	case "enter":
		host := m.hostPicker.CurrentHost()
		if host == nil {
			m.statusMsg = "No host selected"
			return m, nil
		}
```

Replace both with the same body but:

```go
			m.statusMsg = m.loc.T("status_no_host_selected")
```

Find:

```go
		m.statusMsg = fmt.Sprintf("Connecting to %s…", host.Name)
```

Replace with:

```go
		m.statusMsg = m.loc.T("status_connecting", host.Name)
```

- [ ] **Step 6: Replace the host-saved and hosts-reload-error messages**

Find (in `handleHostFormKey`):

```go
		if err := m.hostPicker.Load(); err != nil {
			m.statusMsg = fmt.Sprintf("Error reloading hosts: %v", err)
		}
		m.statusMsg = fmt.Sprintf("Saved host %q", host.Name)
```

Replace with:

```go
		if err := m.hostPicker.Load(); err != nil {
			m.statusMsg = m.loc.T("status_hosts_load_error", err)
		}
		m.statusMsg = m.loc.T("status_host_saved", host.Name)
```

- [ ] **Step 7: Replace "No files selected" and "Directories not supported"**

Find (in `enqueueCopyDirection`):

```go
		if len(files) == 0 {
			m.statusMsg = "No files selected"
			return nil
		}
```

Replace with:

```go
		if len(files) == 0 {
			m.statusMsg = m.loc.T("status_no_files_selected")
			return nil
		}
```

Find:

```go
			if entry.IsDir {
				m.statusMsg = "Directories not supported"
				continue
			}
```

Replace with:

```go
			if entry.IsDir {
				m.statusMsg = m.loc.T("status_dir_not_supported")
				continue
			}
```

- [ ] **Step 8: Verify no stray hardcoded status strings remain**

Run: `grep -n 'm\.statusMsg = "' internal/ui/app.go`
Expected: No output (every `m.statusMsg = "..."` literal has been replaced with `m.loc.T(...)`).

Run: `grep -n 'm\.statusMsg = fmt\.Sprintf' internal/ui/app.go`
Expected: No output.

- [ ] **Step 9: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS, `gofmt -l .` prints nothing.

- [ ] **Step 10: Commit**

```bash
git add internal/ui/app.go
git commit -m "Route remaining status/hint strings through the active lingo pack"
```

---

### Task 14: Manual smoke test and doc updates

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

**Interfaces:** none (docs + manual verification only)

- [ ] **Step 1: Build and run the full automated check suite**

Run: `make build && go vet ./... && go test ./... -v && gofmt -l .`
Expected: Build succeeds, all tests PASS across every package, `gofmt -l .` prints nothing.

- [ ] **Step 2: Manual smoke test (not automatable — run and observe)**

```bash
./exfil
```

Walk through:
1. Press `S` — Settings screen opens, showing "Lingo Pack: ◄ plain ►", "Primary Color: #B341F5", "Secondary Color: #6E6E6E".
2. Press `→` — pack cycles to `secretsquirrel`.
3. `Tab` to the Primary Color row, clear it, type `#39FF14` — the label/border colors elsewhere on the Settings screen should not change (live preview is scoped to the Settings screen's own rendering, per the design — the browsing screen isn't visible underneath), but no crash/garbled output should occur while typing an incomplete hex.
4. Press `Enter` — returns to the browsing screen; pane borders, selection highlight, and hint bar should now be in the new secretsquirrel wording and the new neon-green primary color.
5. Press `?` — About screen shows the `secretsquirrel` tagline ("covert data extraction unit") and relabeled fields ("Build:", "Clearance:", "Origin:").
6. Press `Esc`, then `S` again, then `Esc` immediately (no changes) — confirms cancel doesn't alter anything.
7. Quit (`q`), relaunch `./exfil` — confirms the pack and colors persisted in `~/.config/exfil/hosts.yaml`.

- [ ] **Step 3: Update `README.md`**

Add a new bullet to the "Status" list (after the About screen bullet):

```markdown
- ✅ Selectable lingo packs (plain/secretsquirrel/keyboardcowboy/corposlut) and free-form hex theme colors via the Settings screen (`S`)
```

Add to the Controls list:

```markdown
- **S** — Settings (lingo pack, theme colors)
```

- [ ] **Step 4: Update `CLAUDE.md`**

Add to the "Current Status" checklist:

```markdown
- ✅ Selectable lingo packs (`internal/i18n`) and free-form hex theme colors, via a dedicated Settings screen (`S`)
```

Add a new subsection under "Code Patterns & Guidelines":

```markdown
### Lingo packs (`internal/i18n`)

Every user-facing string goes through `loc.T("message_id", args...)` rather than being hardcoded. Four packs — `plain`, `secretsquirrel`, `keyboardcowboy`, `corposlut` — live as embedded YAML catalogs in `internal/i18n/locales/`. `Localizer.T` falls back to `plain` for any key missing from the active pack, then to the raw message ID if even `plain` doesn't have it. Panes don't store `Theme`/`Localizer` at construction — `View()` takes them as parameters — so a Settings-screen change re-themes the whole app immediately without reconstructing anything.
```

- [ ] **Step 5: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "Document lingo packs and Settings screen in README/CLAUDE.md"
```

---

## Self-Review

**Spec coverage:**
- i18n package + fallback behavior → Task 1
- Four lingo packs → Tasks 1, 2
- Config fields → Task 3
- Theme refactor (configurable primary/secondary, semantic colors fixed) → Task 4
- Localizer wired into Model → Task 5
- Every pane's theme/loc plumbing (the prerequisite for live re-theming) → Tasks 6–10
- SettingsPane (arrow-cycle pack, free-form hex, Tab/Shift+Tab, live preview, validation, save/cancel) → Task 11
- `S` key, screen routing, save/cancel semantics, `←`/`→` row-dependent behavior → Task 12
- Remaining hardcoded status/hint strings → Task 13
- Manual verification + docs → Task 14

**Placeholder scan:** No "TBD"/"TODO" in any task.

**Type consistency check:**
- `NewTheme(primary, secondary lipgloss.Color) Theme` — signature introduced in Task 4, used identically in Tasks 5, 12.
- `(*BrowserPane).View(theme Theme) string`, `(*QueuePane).View(theme Theme, loc *i18n.Localizer) string`, `(*HostPickerPane).View(theme Theme, loc *i18n.Localizer) string`, `(*HostFormPane).View(theme Theme, loc *i18n.Localizer) string`, `(*AboutPane).View(theme Theme, loc *i18n.Localizer) string`, `(*SettingsPane).View(theme Theme, loc *i18n.Localizer) string` — each introduced once (Tasks 6–11) and called with matching argument order in Task 12's `View()` edits.
- `HostFormPane.Save(loc *i18n.Localizer) (config.Host, error)` (Task 9) matches its call site `m.hostForm.Save(m.loc)` (Task 9, same task's app.go edit).
- `SettingsPane.Focused() settingsField` (Task 11) matches its use in Task 12's `handleSettingsKey` (`m.settingsPane.Focused() == settingsFieldLingo`).
- `Model.primaryColorHex`/`secondaryColorHex` (Task 5) are read/written consistently across Tasks 12–13 with no renaming.
