package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func gitCmd(dir string, args ...string) (string, error) {
	out, err := gitRaw(dir, args...)
	return strings.TrimRight(string(out), "\n"), err
}

func gitRaw(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

// runGitMode builds and runs the git comparison, cleaning up the temp dir.
func runGitMode(ref, cwd, pathspec string) {
	d, tmp, err := newGitDirModel(ref, cwd, pathspec)
	if err != nil {
		fatal(err)
	}
	err = runProgram(&app{dir: d})
	os.RemoveAll(tmp)
	if err != nil {
		fatal(err)
	}
}

// resolveSymlinks expands symlinks in path so two paths naming the same file
// compare equal. `git rev-parse --show-toplevel` always reports the real path,
// while a caller's cwd/pathspec may still carry a symlinked prefix — on macOS
// every temp dir does (/var is a symlink to /private/var) — and comparing the
// two unresolved makes an in-repo path look like it sits outside the repo.
// Falls back to the parent's resolution, then to the input, so a path that
// doesn't exist (a deleted file) still gets its prefix normalized.
func resolveSymlinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	dir, base := filepath.Split(filepath.Clean(path))
	if dir != "" {
		if resolved, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
			return filepath.Join(resolved, base)
		}
	}
	return filepath.Clean(path)
}

// newGitDirModel compares the working tree of cwd's repository against ref,
// or two refs against each other when ref is "A..B" (both sides read-only),
// optionally limited to pathspec (a file or directory, relative or absolute).
// Ref-side blobs are materialized below a temp dir which is returned for the
// caller to remove.
func newGitDirModel(ref, cwd, pathspec string) (*dirModel, string, error) {
	root, err := gitCmd(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, "", err
	}
	refA, refB, twoRefs := strings.Cut(ref, "..")
	if !twoRefs {
		refA = ref
	}
	spec := ""
	if pathspec != "" {
		abs := pathspec
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, pathspec)
		}
		rel, err := filepath.Rel(resolveSymlinks(root), resolveSymlinks(abs))
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, "", fmt.Errorf("path %s is outside the repository %s", pathspec, root)
		}
		if rel != "." {
			spec = rel
		}
	}
	tmp, err := os.MkdirTemp("", "difftool-git-")
	if err != nil {
		return nil, "", err
	}
	fail := func(err error) (*dirModel, string, error) {
		os.RemoveAll(tmp)
		return nil, "", err
	}
	diffArgs := []string{"-c", "core.quotepath=false", "diff", "--name-status", "--no-renames", refA}
	if twoRefs {
		diffArgs = append(diffArgs, refB)
	}
	diffArgs = append(diffArgs, "--")
	if spec != "" {
		diffArgs = append(diffArgs, spec)
	}
	leftRoot, rightRoot := tmp, root
	if twoRefs {
		leftRoot, rightRoot = filepath.Join(tmp, "a"), filepath.Join(tmp, "b")
	}
	materialize := func(r, rel, dir string) error {
		blob, err := gitRaw(root, "show", r+":"+rel)
		if err != nil {
			return err
		}
		return writeFileMkdir(filepath.Join(dir, rel), blob, 0o644)
	}
	out, err := gitCmd(root, diffArgs...)
	if err != nil {
		return fail(err)
	}
	rels := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		st, rel, ok := strings.Cut(line, "\t")
		if !ok || ignored(rel) {
			continue
		}
		rels[rel] = true
		if st != "A" { // present in A
			if err := materialize(refA, rel, leftRoot); err != nil {
				return fail(err)
			}
		}
		if twoRefs && st != "D" { // present in B
			if err := materialize(refB, rel, rightRoot); err != nil {
				return fail(err)
			}
		}
	}
	if !twoRefs {
		unArgs := []string{"-c", "core.quotepath=false", "ls-files", "--others", "--exclude-standard", "--"}
		if spec != "" {
			unArgs = append(unArgs, spec)
		}
		untracked, err := gitCmd(root, unArgs...)
		if err != nil {
			return fail(err)
		}
		for _, rel := range strings.Split(untracked, "\n") {
			if rel != "" && !ignored(rel) {
				rels[rel] = true
			}
		}
	}

	rightLabel := root
	if spec != "" {
		rightLabel = filepath.Join(root, spec)
	}
	if twoRefs {
		rightLabel = refB
	}
	d := &dirModel{
		leftRoot: leftRoot, rightRoot: rightRoot,
		leftLabel: refA, rightLabel: rightLabel,
		roLeft: true, roRight: twoRefs,
	}
	sorted := make([]string, 0, len(rels))
	for rel := range rels {
		sorted = append(sorted, rel)
	}
	d.setEntries(sorted)
	if d.selected() == nil {
		d.status = "working tree matches " + ref
		if twoRefs {
			d.status = refA + " and " + refB + " are identical"
		}
	}
	return d, tmp, nil
}
