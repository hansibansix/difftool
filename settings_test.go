package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	orig := cfg
	defer func() { cfg = orig }()
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	cfg = config{Theme: "nord", Intraline: false, IgnoreWs: true, TabWidth: 8, ShowIdentical: true}
	if err := saveConfigTo(path); err != nil {
		t.Fatal(err)
	}
	want := cfg
	cfg = defaultConfig()
	loadConfigFrom(path)
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v want %+v", cfg, want)
	}
}

func TestConfigValidation(t *testing.T) {
	orig := cfg
	defer func() { cfg = orig }()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg = config{Theme: "nope", TabWidth: 7}
	if err := saveConfigTo(path); err != nil {
		t.Fatal(err)
	}
	cfg = defaultConfig()
	loadConfigFrom(path)
	if cfg.Theme != "rose-pine" || cfg.TabWidth != 4 {
		t.Fatalf("invalid values must fall back: %+v", cfg)
	}
}

func TestNormalizeWs(t *testing.T) {
	got := normalizeWs([]string{"  a\t b ", "", "x"})
	want := []string{"a b", "", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestIgnoreWhitespaceDiff(t *testing.T) {
	orig := cfg
	defer func() { cfg = orig }()
	left := []string{"a  b", "c"}
	right := []string{"a b", "c"}
	cfg.IgnoreWs = false
	m := testModel(left, right)
	if len(m.nav) != 1 {
		t.Fatalf("ws difference should be a change: %+v", m.nav)
	}
	cfg.IgnoreWs = true
	m.recompute()
	if len(m.nav) != 0 {
		t.Fatalf("ws difference should vanish with IgnoreWs: %+v", m.nav)
	}
}

// every theme must define the colors the renderer relies on
func TestThemesComplete(t *testing.T) {
	for name, th := range themes {
		fields := map[string]string{
			"delBg": th.delBg, "insBg": th.insBg, "modBg": th.modBg,
			"modEmphBg": th.modEmphBg, "appliedBg": th.appliedBg,
			"appliedCurBg": th.appliedCurBg, "appliedFg": th.appliedFg,
			"selBg": th.selBg, "stMod": th.stMod, "stLeft": th.stLeft,
			"stRight": th.stRight, "stSame": th.stSame, "stApplied": th.stApplied,
			"muted": th.muted, "subtle": th.subtle, "accent": th.accent, "sep": th.sep,
			"delCurBg": th.delCurBg, "insCurBg": th.insCurBg, "modCurBg": th.modCurBg,
			"modEmphCurBg": th.modEmphCurBg, "voidBg": th.voidBg, "voidCurBg": th.voidCurBg,
		}
		for f, v := range fields {
			if v == "" {
				t.Errorf("theme %s: %s is empty", name, f)
			}
		}
	}
}

func TestMenuThemeCycle(t *testing.T) {
	orig := cfg
	defer func() { cfg = orig }()
	a := &app{}
	items := a.menuItems()
	start := cfg.Theme
	for range themes {
		items[0].change(1)
	}
	if cfg.Theme != start {
		t.Fatalf("full cycle should return to %q, got %q", start, cfg.Theme)
	}
	items[0].change(-1)
	items[0].change(1)
	if cfg.Theme != start {
		t.Fatalf("back and forth should return to %q, got %q", start, cfg.Theme)
	}
}

func TestIgnored(t *testing.T) {
	orig, origX := cfg, extraIgnores
	defer func() { cfg, extraIgnores = orig, origX }()
	cfg.UseIgnores = true
	cfg.IgnorePatterns = []string{"node_modules", "*.min.js", "amd/build"}
	extraIgnores = []string{"*.log"}
	cases := map[string]bool{
		"node_modules":       true,
		"sub/node_modules":   true, // basename match at any depth
		"main.min.js":        true,
		"amd/src/x.js":       false,
		"amd/build":          true,
		"debug.log":          true, // from -x
		"sub/deep/trace.log": true,
		"lib.php":            false,
		"amd/build.php":      false,
	}
	for rel, want := range cases {
		if got := ignored(rel); got != want {
			t.Errorf("ignored(%q) = %v, want %v", rel, got, want)
		}
	}
	cfg.UseIgnores = false
	if ignored("node_modules") {
		t.Error("UseIgnores=false must disable all patterns")
	}
}

func TestScanHonorsIgnores(t *testing.T) {
	orig := cfg
	defer func() { cfg = orig }()
	cfg.UseIgnores = true
	cfg.IgnorePatterns = []string{"node_modules", "*.min.js"}
	l, r := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(l, "a.php"), "x\n")
	writeTestFile(t, filepath.Join(l, "b.min.js"), "x\n")
	writeTestFile(t, filepath.Join(l, "node_modules", "dep.js"), "x\n")
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.entries) != 1 || d.entries[0].rel != "a.php" {
		t.Fatalf("ignored files must not be scanned: %+v", d.entries)
	}
}

func TestIgnoreEditor(t *testing.T) {
	orig := cfg
	defer func() { cfg = orig }()
	cfg.IgnorePatterns = []string{"node_modules"}
	a := &app{}
	a.addIgnore("  *.log ")
	a.addIgnore("*.log") // duplicate ignored
	a.addIgnore("   ")   // empty ignored
	if !reflect.DeepEqual(cfg.IgnorePatterns, []string{"node_modules", "*.log"}) || a.ignSel != 1 {
		t.Fatalf("add: %v sel=%d", cfg.IgnorePatterns, a.ignSel)
	}
	a.removeIgnore(0)
	if !reflect.DeepEqual(cfg.IgnorePatterns, []string{"*.log"}) || a.ignSel != 0 {
		t.Fatalf("remove: %v sel=%d", cfg.IgnorePatterns, a.ignSel)
	}
	a.removeIgnore(5) // out of range: no-op
	if len(cfg.IgnorePatterns) != 1 {
		t.Fatal("out-of-range remove must be a no-op")
	}
}
