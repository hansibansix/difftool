package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a temp git repository and returns its path plus a helper
// that runs git in it with a throwaway identity.
func initRepo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
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
	return repo, git
}

func TestGitTwoRefs(t *testing.T) {
	repo, git := initRepo(t)
	writeTestFile(t, filepath.Join(repo, "a.txt"), "1\n")
	writeTestFile(t, filepath.Join(repo, "gone.txt"), "x\n")
	git("add", ".")
	git("commit", "-q", "-m", "one")
	writeTestFile(t, filepath.Join(repo, "a.txt"), "2\n")
	writeTestFile(t, filepath.Join(repo, "new.txt"), "n\n")
	if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "two")
	writeTestFile(t, filepath.Join(repo, "a.txt"), "worktree noise\n") // must not show up

	d, tmp, err := newGitDirModel("HEAD~1..HEAD", repo, "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	want := map[string]dirStatus{"a.txt": stModified, "gone.txt": stOnlyLeft, "new.txt": stOnlyRight}
	if len(d.entries) != len(want) {
		t.Fatalf("entries: %+v", d.entries)
	}
	for _, e := range d.entries {
		if want[e.rel] != e.status {
			t.Errorf("%s: %v", e.rel, e.status)
		}
	}
	if !d.roLeft || !d.roRight || d.leftLabel != "HEAD~1" || d.rightLabel != "HEAD" {
		t.Fatalf("flags/labels: %+v", d)
	}
	b, _ := os.ReadFile(filepath.Join(d.rightRoot, "a.txt"))
	if string(b) != "2\n" {
		t.Fatalf("right side must be the B blob, got %q", b)
	}
	selectRel(t, d, "a.txt")
	d.copyEntry(true)
	d.copyEntry(false)
	if d.entries[0].status != stModified || !strings.Contains(d.status, "read-only") {
		t.Fatalf("copies must be refused: %v %q", d.entries[0].status, d.status)
	}
}

// TestGitDirModelPathspecThroughSymlink guards the case macOS hits for free:
// `git rev-parse --show-toplevel` reports the real path while the caller's cwd
// and pathspec still carry a symlinked prefix (/var -> /private/var there).
// Comparing the two unresolved makes an in-repo path look outside the repo.
func TestGitDirModelPathspecThroughSymlink(t *testing.T) {
	repo, git := initRepo(t)
	writeTestFile(t, filepath.Join(repo, "sub", "in.txt"), "a\n")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	writeTestFile(t, filepath.Join(repo, "sub", "in.txt"), "b\n")

	// Reach the same repo through a symlink, as macOS's temp dirs do.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	d, tmp, err := newGitDirModel("HEAD", link, filepath.Join(link, "sub"))
	if err != nil {
		t.Fatalf("pathspec through a symlink must resolve: %v", err)
	}
	defer os.RemoveAll(tmp)
	if len(d.entries) != 1 || d.entries[0].rel != filepath.Join("sub", "in.txt") {
		t.Fatalf("pathspec scope wrong through symlink: %+v", d.entries)
	}
}

// TestGitAppSingleFileSkipsTree: a pathspec naming one file goes straight to
// the file view — a tree pane listing exactly that file is pure overhead.
// Directory and whole-repo pathspecs keep the tree.
func TestGitAppSingleFileSkipsTree(t *testing.T) {
	repo, git := initRepo(t)
	writeTestFile(t, filepath.Join(repo, "sub", "in.txt"), "a\n")
	writeTestFile(t, filepath.Join(repo, "sub", "other.txt"), "a\n")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	writeTestFile(t, filepath.Join(repo, "sub", "in.txt"), "b\n")
	writeTestFile(t, filepath.Join(repo, "sub", "other.txt"), "b\n")

	t.Run("file pathspec", func(t *testing.T) {
		a, tmp, err := newGitApp("HEAD", repo, filepath.Join(repo, "sub", "in.txt"))
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmp)
		if a.dir != nil {
			t.Error("single-file pathspec must not build a tree pane")
		}
		if a.file == nil {
			t.Fatal("single-file pathspec must open the file view")
		}
		if a.split() {
			t.Error("split layout must be off without a tree")
		}
		if !a.file.roLeft {
			t.Error("ref side must stay read-only in the file view")
		}
	})

	t.Run("dir pathspec keeps the tree", func(t *testing.T) {
		a, tmp, err := newGitApp("HEAD", repo, filepath.Join(repo, "sub"))
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmp)
		if a.dir == nil {
			t.Error("directory pathspec must keep the tree pane")
		}
	})

	t.Run("no pathspec keeps the tree", func(t *testing.T) {
		a, tmp, err := newGitApp("HEAD", repo, "")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmp)
		if a.dir == nil {
			t.Error("repo-wide git mode must keep the tree pane")
		}
	})
}
