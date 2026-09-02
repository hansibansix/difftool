package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeymapLookup(t *testing.T) {
	keys = defaultKeys()
	if keys.file.action("]") != "next" || keys.file.action("ctrl+d") != "half-down" || keys.file.action("Z") != "" {
		t.Fatal("default lookups")
	}
	if hint(keys.file, "next", "prev") != "n/p" || keyLabel(keys.file, "apply-right") != "l right >" {
		t.Fatalf("hint %q label %q", hint(keys.file, "next", "prev"), keyLabel(keys.file, "apply-right"))
	}
}

func TestApplyKeysOverridesAndWarns(t *testing.T) {
	defer func() { keys = defaultKeys() }()
	warn := applyKeys(map[string]map[string][]string{
		"file":    {"next": {"ctrl+n"}, "bogus": {"x"}},
		"dir":     {"quit": {"Q"}},
		"nowhere": {"a": {"b"}},
	})
	if keys.file.action("ctrl+n") != "next" || keys.file.action("n") != "" {
		t.Fatalf("override not applied: %v", keys.file.all("next"))
	}
	if keys.dir.action("Q") != "quit" || keys.dir.action("q") != "" {
		t.Fatal("dir override not applied")
	}
	if len(warn) != 2 || !strings.Contains(warn[0], `"bogus"`) || !strings.Contains(warn[1], `"nowhere"`) {
		t.Fatalf("warnings = %v", warn)
	}
	// a key bound in two actions resolves to the first in default order
	applyKeys(map[string]map[string][]string{"file": {"undo": {"n"}}})
	if keys.file.action("n") != "next" {
		t.Fatalf("n should stay 'next' (earlier binding wins): %q", keys.file.action("n"))
	}
}

func TestRemappedKeyDrivesModel(t *testing.T) {
	defer func() { keys = defaultKeys() }()
	applyKeys(map[string]map[string][]string{"file": {"apply-right": {"y"}, "next": {"ctrl+n"}}})
	m := testModel([]string{"a", "b"}, []string{"A", "B"})
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}) // no longer bound
	if m.right[0] != "A" {
		t.Fatal("unbound key must do nothing")
	}
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if m.right[0] != "a" {
		t.Fatalf("remapped apply: %v", m.right)
	}
	if !strings.Contains(m.view(true), "y") {
		t.Fatal("footer hint must show the remapped key")
	}
}

func TestKeysJSONRoundTrip(t *testing.T) {
	keys = defaultKeys()
	var out struct {
		Keys map[string]map[string][]string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(keysJSON()), &out); err != nil {
		t.Fatal(err)
	}
	if warn := applyKeys(out.Keys); len(warn) != 0 || keys.file.action("n") != "next" {
		t.Fatalf("round trip: %v", warn)
	}
}

func TestHelpShowsBindings(t *testing.T) {
	defer func() { keys = defaultKeys() }()
	applyKeys(map[string]map[string][]string{"file": {"save": {"ctrl+s"}}})
	initStyles(themes["ansi"])
	a := &app{w: 120, h: 60}
	if v := a.helpView(); !strings.Contains(v, "ctrl+s") {
		t.Fatal("help must render the configured key")
	}
}
