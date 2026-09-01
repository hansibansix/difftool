package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// newGitDirModel compares the working tree of cwd's repository against ref,
// optionally limited to pathspec (a file or directory, relative or absolute).
// The ref-side blobs are materialized below a temp dir which is returned
// for the caller to remove.
func newGitDirModel(ref, cwd, pathspec string) (*dirModel, string, error) {
	root, err := gitCmd(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, "", err
	}
	spec := ""
	if pathspec != "" {
		abs := pathspec
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, pathspec)
		}
		rel, err := filepath.Rel(root, abs)
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
	diffArgs := []string{"-c", "core.quotepath=false", "diff", "--name-status", "--no-renames", ref, "--"}
	if spec != "" {
		diffArgs = append(diffArgs, spec)
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
		if st == "A" {
			continue // not in ref, nothing to materialize
		}
		blob, err := gitRaw(root, "show", ref+":"+rel)
		if err != nil {
			return fail(err)
		}
		dst := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(dst, blob, 0o644); err != nil {
			return fail(err)
		}
	}
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

	rightLabel := root
	if spec != "" {
		rightLabel = filepath.Join(root, spec)
	}
	d := &dirModel{
		leftRoot: tmp, rightRoot: root,
		leftLabel: ref, rightLabel: rightLabel,
		roLeft: true, showAll: cfg.ShowIdentical,
	}
	sorted := make([]string, 0, len(rels))
	for rel := range rels {
		sorted = append(sorted, rel)
	}
	sort.Slice(sorted, func(i, j int) bool {
		di, dj := filepath.Dir(sorted[i]), filepath.Dir(sorted[j])
		if di != dj {
			return di < dj
		}
		return sorted[i] < sorted[j]
	})
	for _, rel := range sorted {
		bits := 0
		if _, err := os.Stat(filepath.Join(tmp, rel)); err == nil {
			bits |= 1
		}
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			bits |= 2
		}
		d.entries = append(d.entries, dirEntry{rel, d.compare(rel, bits)})
	}
	d.rebuildList()
	if d.selected() == nil {
		d.status = "working tree matches " + ref
	}
	return d, tmp, nil
}
