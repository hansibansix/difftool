package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

func TestApplyAll(t *testing.T) {
	m := testModel(
		[]string{"a", "b", "c", "d", "e"},
		[]string{"a", "B", "c", "D", "E"},
	)
	m.applyAll(true)
	if !reflect.DeepEqual(m.left, m.right) {
		t.Fatalf("applyAll must sync sides: %v vs %v", m.left, m.right)
	}
	if !strings.Contains(m.status, "2 hunks") {
		t.Fatalf("status %q", m.status)
	}
	if len(m.undo) != 1 {
		t.Fatalf("applyAll must be one undo step, got %d", len(m.undo))
	}
	m.undoLast()
	if m.right[1] != "B" || len(m.applied) != 0 {
		t.Fatalf("undo must restore everything: %v", m.right)
	}
}

func TestResetAll(t *testing.T) {
	m := testModel([]string{"a", "b"}, []string{"A", "B"})
	m.applyAll(true)
	m.resetAll()
	if !reflect.DeepEqual(m.right, []string{"A", "B"}) || len(m.applied) != 0 {
		t.Fatalf("resetAll must restore the right side: %v %v", m.right, m.applied)
	}
	if len(m.nav) != 1 { // adjacent hunks merge back into one pending change
		t.Fatalf("hunks must be pending again: %+v", m.nav)
	}
}

func TestSearchMatches(t *testing.T) {
	m := testModel(
		[]string{"alpha", "bravo", "charlie"},
		[]string{"alpha", "BRAVO", "charlie"},
	)
	m.search = "bravo"
	m.computeMatches()
	if len(m.matches) != 1 || m.matches[0] != 1 {
		t.Fatalf("matches = %v", m.matches)
	}
	m.search = "AL" // case-insensitive
	m.computeMatches()
	if len(m.matches) != 1 || m.matches[0] != 0 {
		t.Fatalf("matches = %v", m.matches)
	}
	m.gotoMatch(1)
	if m.matchIdx != 0 {
		t.Fatalf("matchIdx = %d", m.matchIdx)
	}
}

func TestDirFilter(t *testing.T) {
	l, r := setupDirs(t)
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	d.filter = "onlyl"
	d.rebuildList()
	if n := fileRowCount(d); n != 1 {
		t.Fatalf("filter should leave 1 file, got %d", n)
	}
	d.filter = ""
	d.rebuildList()
	if n := fileRowCount(d); n != 3 {
		t.Fatalf("clearing filter should restore 3, got %d", n)
	}
}

// each theme's chroma style must exist in the registry (or be empty = off)
func TestChromaStylesExist(t *testing.T) {
	for name, th := range themes {
		if th.chromaStyle == "" {
			continue
		}
		if styles.Registry[th.chromaStyle] == nil {
			t.Errorf("theme %s: chroma style %q not in registry", name, th.chromaStyle)
		}
	}
}

func TestHighlightLines(t *testing.T) {
	origCfg, origTh := cfg, th
	defer func() { cfg, th = origCfg, origTh }()
	cfg.Syntax = true
	th.chromaStyle = "rose-pine"
	fgs := highlightLines("x.php", []string{"<?php", "function foo() {", "    return 42;", "}"})
	if fgs == nil || len(fgs) != 4 {
		t.Fatalf("expected spans for 4 lines, got %v", fgs)
	}
	found := false
	for _, s := range fgs[1] {
		if s.fg != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("function line should have colored spans")
	}
	// spans must be within line bounds and colors well-formed
	for i, line := range fgs {
		for _, s := range line {
			if s.a < 0 || s.b < s.a || !strings.HasPrefix(s.fg, "#") {
				t.Fatalf("line %d: bad span %+v", i, s)
			}
		}
	}
	cfg.Syntax = false
	if highlightLines("x.php", []string{"<?php"}) != nil {
		t.Fatal("Syntax=false must disable highlighting")
	}
}

func TestDisplayPath(t *testing.T) {
	orig := homeDir
	homeDir = "/home/tester"
	defer func() { homeDir = orig }()
	if got := displayPath("/home/tester/work/x.php"); got != "~/work/x.php" {
		t.Fatalf("got %q", got)
	}
	if got := displayPath("/srv/x.php"); got != "/srv/x.php" {
		t.Fatalf("got %q", got)
	}
	if got := displayPath("/home/testerx/y"); got != "/home/testerx/y" {
		t.Fatalf("prefix must match a path component: %q", got)
	}
}

func TestFooterBarFitsWidth(t *testing.T) {
	keys := [][2]string{{"n/p", "change"}, {"h·l", "◀ apply ▶"}, {"a", "all"}, {"q", "quit"}}
	for _, w := range []int{10, 20, 35, 80} {
		got := footerBar(w, "", "info", keys)
		if lipgloss.Width(got) != w {
			t.Fatalf("w=%d: rendered width %d", w, lipgloss.Width(got))
		}
	}
	// a partially fitting hint is dropped, not cut mid-word
	got := footerBar(22, "", "", keys)
	if !strings.Contains(got, "n/p") || strings.Contains(got, "h·l ◀ app") && !strings.Contains(got, "apply ▶") {
		t.Fatalf("hint was cut: %q", got)
	}
}

func TestRowAtLineAndSelectRow(t *testing.T) {
	orig := cfg
	defer func() { cfg = orig }()
	cfg.Wrap = false
	m := testModel(
		[]string{"a", "b", "c", "d"},
		[]string{"a", "B", "c", "D"},
	)
	m.w, m.h = 80, 20
	if got := m.rowAtLine(3); got != 3 {
		t.Fatalf("rowAtLine(3) = %d", got)
	}
	if got := m.rowAtLine(10); got != -1 {
		t.Fatalf("beyond content: %d", got)
	}
	m.selectRow(3) // row of d/D, the second change
	if m.cur != 1 {
		t.Fatalf("cur = %d", m.cur)
	}
	m.selectRow(0) // equal row: cursor unchanged
	if m.cur != 1 {
		t.Fatalf("equal row must not move the cursor, cur = %d", m.cur)
	}
}

func TestDistinctTails(t *testing.T) {
	l, r := distinctTails("~/ws/foo-prod/htdocs/auth/plugin", "~/ws/foo-test/htdocs/auth/plugin")
	if l != "foo-prod/htdocs/auth/plugin" || r != "foo-test/htdocs/auth/plugin" {
		t.Fatalf("got %q %q", l, r)
	}
	l, r = distinctTails("/a/x", "/a/y")
	if l != "x" || r != "y" {
		t.Fatalf("got %q %q", l, r)
	}
	l, r = distinctTails("/same/dir", "/same/dir") // at least the last component stays
	if l != "dir" || r != "dir" {
		t.Fatalf("got %q %q", l, r)
	}
}

func TestShortenPath(t *testing.T) {
	p := "foo-prod/htdocs/auth/plugin/auth.php"
	if got := shortenPath(p, 80); got != p {
		t.Fatalf("fits: %q", got)
	}
	if got := shortenPath(p, 24); got != "foo-prod/…/auth.php" {
		t.Fatalf("middle: %q", got)
	}
	if got := shortenPath(p, 30); got != "foo-prod/…/plugin/auth.php" {
		t.Fatalf("keep as much as fits: %q", got)
	}
	if got := shortenPath("a/verylongfilename.php", 8); got != "…ame.php" {
		t.Fatalf("fallback: %q", got)
	}
}
