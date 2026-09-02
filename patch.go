package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const patchContext = 3

// exportPatch writes the pending changes (or just chunk `only`, if ≥ 0) as a
// unified diff left → right to the clipboard, falling back to a file.
func (m *model) exportPatch(only int) {
	patch, n := m.unifiedDiff(only, patchContext)
	if n == 0 {
		m.status = "no pending changes to export"
		return
	}
	if tool, err := toClipboard(patch); err == nil {
		m.status = fmt.Sprintf("patch → clipboard via %s (%d hunks)", tool, n)
		return
	}
	const f = "difftool.patch"
	if err := os.WriteFile(f, []byte(patch), 0o644); err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.status = fmt.Sprintf("no clipboard tool — patch → ./%s (%d hunks)", f, n)
}

// unifiedDiff renders the pending (non-ignored) changes as a unified diff
// with ctx context lines and returns it with the hunk count. Changes closer
// than 2*ctx lines share a hunk; ignored changes never open a hunk but are
// emitted as changes when a hunk reaches them, so the patch stays applicable.
// Chunks alternate equal/change, so a change's neighbours are equal chunks
// and context never crosses into another change.
func (m *model) unifiedDiff(only, ctx int) (string, int) {
	var b strings.Builder
	name := m.patchName()
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", name, name)
	emit := func(prefix string, lines []string, from, to int, nl bool) {
		for i := from; i < to; i++ {
			b.WriteString(prefix + lines[i] + "\n")
			if i == len(lines)-1 && !nl {
				b.WriteString("\\ No newline at end of file\n")
			}
		}
	}
	hunks := 0
	for i := 0; i < len(m.chunks); i++ {
		if !m.isChange(i) || (only >= 0 && i != only) {
			continue
		}
		// extend the hunk over following changes within 2*ctx equal lines
		j := i
		for k := i + 1; k+1 < len(m.chunks) && only < 0; k += 2 {
			if eq := m.chunks[k]; eq.l1-eq.l0 > 2*ctx {
				break
			}
			j = k + 1
		}
		first, last := m.chunks[i], m.chunks[j]
		pre, post := 0, 0
		if i > 0 {
			pre = min(ctx, first.l0-m.chunks[i-1].l0)
		}
		if j+1 < len(m.chunks) {
			post = min(ctx, m.chunks[j+1].l1-last.l1)
		}
		l0, r0, l1, r1 := first.l0-pre, first.r0-pre, last.l1+post, last.r1+post
		fmt.Fprintf(&b, "@@ -%s +%s @@\n", hunkRange(l0, l1), hunkRange(r0, r1))
		emit(" ", m.left, l0, first.l0, m.leftNL)
		for k := i; k <= j; k++ {
			c := m.chunks[k]
			if c.kind == kindEqual {
				emit(" ", m.left, c.l0, c.l1, m.leftNL)
				continue
			}
			emit("-", m.left, c.l0, c.l1, m.leftNL)
			emit("+", m.right, c.r0, c.r1, m.rightNL)
		}
		emit(" ", m.left, last.l1, l1, m.leftNL)
		hunks++
		i = j
	}
	return b.String(), hunks
}

// hunkRange formats a half-open line range for a @@ header (1-based; an
// empty range names the line before it, as diff does).
func hunkRange(from, to int) string {
	if to == from {
		return fmt.Sprintf("%d,0", from)
	}
	return fmt.Sprintf("%d,%d", from+1, to-from)
}

// patchName is the a/ b/ path of the patch: the repo-relative name in git
// mode, otherwise the right file relative to the working directory.
func (m *model) patchName() string {
	if i := strings.IndexByte(m.leftName, ':'); i >= 0 && m.roLeft {
		return m.leftName[i+1:]
	}
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, m.rightPath); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return m.rightPath
}

var errNoClipboard = errors.New("no clipboard tool found")

// toClipboard pipes s into the first clipboard tool available and returns
// the tool's name.
func toClipboard(s string) (string, error) {
	for _, c := range [][]string{
		{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}, {"pbcopy"},
	} {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(s)
		if err := cmd.Run(); err != nil {
			continue // e.g. wl-copy without a Wayland display: try the next one
		}
		return c[0], nil
	}
	return "", errNoClipboard
}
