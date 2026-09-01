package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
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
