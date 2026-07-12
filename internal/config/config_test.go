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
