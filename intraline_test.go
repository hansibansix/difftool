package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestChangedSpans(t *testing.T) {
	sa, sb := changedSpans([]rune("abcdef"), []rune("abXdef"))
	if !reflect.DeepEqual(sa, []span{{2, 3}}) || !reflect.DeepEqual(sb, []span{{2, 3}}) {
		t.Fatalf("got %v %v", sa, sb)
	}
	// pure insertion on the right
	sa, sb = changedSpans([]rune("abc"), []rune("abXXc"))
	if len(sa) != 0 || !reflect.DeepEqual(sb, []span{{2, 4}}) {
		t.Fatalf("got %v %v", sa, sb)
	}
	// identical
	sa, sb = changedSpans([]rune("same"), []rune("same"))
	if len(sa) != 0 || len(sb) != 0 {
		t.Fatalf("got %v %v", sa, sb)
	}
}

func TestClipAndStyle(t *testing.T) {
	z := lipgloss.NewStyle()
	if got := clipAndStyle("hello", nil, nil, 0, 8, true, z, z); got != "hello   " {
		t.Fatalf("pad: %q", got)
	}
	if got := clipAndStyle("hello world", nil, nil, 0, 5, true, z, z); got != "hell…" {
		t.Fatalf("right clip: %q", got)
	}
	if got := clipAndStyle("hello world", nil, nil, 6, 5, true, z, z); got != "…orld" {
		t.Fatalf("left clip: %q", got)
	}
	if got := clipAndStyle("hi", nil, nil, 10, 4, true, z, z); got != "    " {
		t.Fatalf("past content: %q", got)
	}
	if got := clipAndStyle("hello world!!", nil, nil, 3, 6, true, z, z); got != "…o wo…" {
		t.Fatalf("both clipped: %q", got)
	}
	// wrap mode: plain window, no clip markers
	if got := clipAndStyle("hello world!!", nil, nil, 6, 5, false, z, z); got != "world" {
		t.Fatalf("wrap piece: %q", got)
	}
	if got := clipAndStyle("hello world!!", nil, nil, 11, 5, false, z, z); got != "!!   " {
		t.Fatalf("wrap tail: %q", got)
	}
}

func TestGitDirModel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		all := append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)
		cmd := exec.Command("git", all...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	writeTestFile(t, filepath.Join(repo, "mod.txt"), "one\ntwo\n")
	writeTestFile(t, filepath.Join(repo, "del.txt"), "gone\n")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	writeTestFile(t, filepath.Join(repo, "mod.txt"), "one\nTWO\n")
	if err := os.Remove(filepath.Join(repo, "del.txt")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repo, "new.txt"), "fresh\n")

	d, tmp, err := newGitDirModel("HEAD", repo, "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	want := map[string]dirStatus{
		"mod.txt": stModified,
		"del.txt": stOnlyLeft,
		"new.txt": stOnlyRight,
	}
	if len(d.entries) != len(want) {
		t.Fatalf("entries: %+v", d.entries)
	}
	for _, e := range d.entries {
		if want[e.rel] != e.status {
			t.Errorf("%s: got %v want %v", e.rel, e.status, want[e.rel])
		}
	}
	if !d.roLeft || d.leftLabel != "HEAD" {
		t.Fatalf("git model flags wrong: %+v", d)
	}
	// materialized ref blob must match the committed content
	data, err := os.ReadFile(filepath.Join(tmp, "mod.txt"))
	if err != nil || string(data) != "one\ntwo\n" {
		t.Fatalf("ref blob: %v %q", err, data)
	}
	// copy toward the ref side must be refused
	selectRel(t, d, "new.txt")
	d.copyEntry(false)
	if _, err := os.Stat(filepath.Join(tmp, "new.txt")); err == nil {
		t.Fatal("copy into ref side must be refused")
	}
	if !strings.Contains(d.status, "read-only") {
		t.Fatalf("status: %q", d.status)
	}
}

func TestApplyLeftRefusedWhenReadOnly(t *testing.T) {
	m := testModel([]string{"a"}, []string{"b"})
	m.roLeft = true
	m.apply(false)
	if m.dirty() || len(m.applied) != 0 || m.left[0] != "a" {
		t.Fatal("apply into read-only left side must be refused")
	}
}

func TestGitDirModelPathspec(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		all := append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)
		cmd := exec.Command("git", all...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	writeTestFile(t, filepath.Join(repo, "top.txt"), "a\n")
	writeTestFile(t, filepath.Join(repo, "sub", "in.txt"), "a\n")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	writeTestFile(t, filepath.Join(repo, "top.txt"), "b\n")
	writeTestFile(t, filepath.Join(repo, "sub", "in.txt"), "b\n")
	writeTestFile(t, filepath.Join(repo, "sub", "new.txt"), "n\n")

	// scoped to sub/ (absolute path): top.txt must not appear
	d, tmp, err := newGitDirModel("HEAD", repo, filepath.Join(repo, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	rels := map[string]bool{}
	for _, e := range d.entries {
		rels[e.rel] = true
	}
	if len(rels) != 2 || !rels[filepath.Join("sub", "in.txt")] || !rels[filepath.Join("sub", "new.txt")] {
		t.Fatalf("pathspec scope wrong: %+v", d.entries)
	}

	// scoped to a single file
	d2, tmp2, err := newGitDirModel("HEAD", repo, filepath.Join(repo, "top.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp2)
	if len(d2.entries) != 1 || d2.entries[0].rel != "top.txt" {
		t.Fatalf("file pathspec wrong: %+v", d2.entries)
	}

	// path outside the repo must error
	if _, _, err := newGitDirModel("HEAD", repo, t.TempDir()); err == nil {
		t.Fatal("outside path must be rejected")
	}
}
