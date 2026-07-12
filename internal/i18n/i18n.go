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
// Other packs (secretsquirrel, keyboardcowboy, corposlut) are added in Task 2.
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
