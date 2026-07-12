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
