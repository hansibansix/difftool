package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type row struct {
	l, r int // line index per side, -1 = filler
	ci   int // index into model.chunks
}

type snapshot struct {
	left, right []string
	cur         int
	applied     []appliedRegion
	side        int // merge mode: which input was shown on the left
}

// appliedRegion remembers a chunk that was applied this session: both sides
// are now equal in [l0,l1)/[r0,r1); toRight tells where the content came from.
type appliedRegion struct {
	l0, l1, r0, r1 int
	toRight        bool
	orig           []string // replaced target-side lines, for reset
}

// navTarget is a position n/p can jump to: a pending change chunk or an
// already applied region.
type navTarget struct {
	ci  int // chunk index, -1 for applied regions
	ai  int // applied region index, -1 for change chunks
	row int // first row, for ordering and scrolling
}

type model struct {
	leftPath, rightPath string
	left, right         []string
	leftNL, rightNL     bool

	chunks []chunk
	rows   []row
	nav    []navTarget

	cur  int // index into nav
	top  int // first visible row
	hOff int // horizontal scroll column
	w, h int

	undo    []snapshot
	applied []appliedRegion
	// content as last read from / written to disk; a side is dirty when its
	// slice header differs (lines are never mutated in place, only replaced)
	savedL, savedR []string

	leftFgs, rightFgs [][]fgSpan // syntax highlighting, per line

	search      string
	searchInput bool
	matches     []int // row indices containing the search term
	matchIdx    int
	pendingAll  bool // 'a' pressed, waiting for the direction key

	// visual mode: a row range inside the current chunk for partial apply
	visual        bool
	vAnchor, vCur int
	status        string
	quitConfirm   bool
	roLeft        bool        // left side is a git ref: no apply ◀
	roRight       bool        // right side is a git ref: no apply ▶
	merge         *mergeState // set in 3-way merge mode
	// display names for the header; differ from the paths in git mode
	leftName, rightName string
}

func newModel(leftPath, rightPath string) (*model, error) {
	left, leftNL, err := readLines(leftPath)
	if err != nil {
		return nil, err
	}
	right, rightNL, err := readLines(rightPath)
	if err != nil {
		return nil, err
	}
	m := &model{
		leftPath: leftPath, rightPath: rightPath,
		leftName: leftPath, rightName: rightPath,
		left: left, right: right,
		leftNL: leftNL, rightNL: rightNL,
	}
	m.savedL, m.savedR = left, right
	m.recompute()
	if len(m.nav) == 0 {
		m.status = "files are identical"
	}
	return m, nil
}

func readLines(path string) ([]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// dir mode diffs a file against a not-yet-existing counterpart
		if os.IsNotExist(err) {
			return []string{}, false, nil
		}
		return nil, false, err
	}
	s := string(data)
	if s == "" {
		return []string{}, false, nil
	}
	nl := strings.HasSuffix(s, "\n")
	if nl {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n"), nl, nil
}

func writeLines(path string, lines []string, nl bool) error {
	s := strings.Join(lines, "\n")
	if nl && len(lines) > 0 {
		s += "\n"
	}
	return writeFileMkdir(path, []byte(s), 0o644)
}

func (m *model) recompute() {
	la, ra := m.left, m.right
	if cfg.IgnoreWs {
		la, ra = normalizeWs(m.left), normalizeWs(m.right)
	}
	if m.merge != nil {
		ra = maskConflicts(ra)
	}
	m.chunks = diffChunks(la, ra)
	m.rows = m.rows[:0]
	m.nav = m.nav[:0]
	for ci, c := range m.chunks {
		if c.kind == kindChange {
			m.nav = append(m.nav, navTarget{ci, -1, len(m.rows)})
		}
		n := max(c.l1-c.l0, c.r1-c.r0)
		for i := 0; i < n; i++ {
			r := row{-1, -1, ci}
			if i < c.l1-c.l0 {
				r.l = c.l0 + i
			}
			if i < c.r1-c.r0 {
				r.r = c.r0 + i
			}
			m.rows = append(m.rows, r)
		}
	}
	rowByL := make(map[int]int)
	for i, r := range m.rows {
		if r.l >= 0 {
			if _, ok := rowByL[r.l]; !ok {
				rowByL[r.l] = i
			}
		}
	}
	for ai, a := range m.applied {
		if a.l0 < a.l1 {
			m.nav = append(m.nav, navTarget{-1, ai, rowByL[a.l0]})
		}
	}
	sort.Slice(m.nav, func(i, j int) bool { return m.nav[i].row < m.nav[j].row })
	m.cur = clamp(m.cur, 0, max(0, len(m.nav)-1))
	m.leftFgs = highlightLines(m.leftPath, expandAll(m.left))
	m.rightFgs = highlightLines(m.rightPath, expandAll(m.right))
	if m.search != "" {
		m.computeMatches()
	}
	m.clampScroll()
}

// applyAll applies every pending change in one direction, undoable in one step.
func (m *model) applyAll(toRight bool) {
	if !m.canApply(toRight) {
		return
	}
	pending := 0
	for _, c := range m.chunks {
		if c.kind == kindChange {
			pending++
		}
	}
	if pending == 0 {
		m.status = "no pending changes"
		return
	}
	m.pushUndo()
	// back to front: applying a chunk only shifts lines after it
	for ci := len(m.chunks) - 1; ci >= 0; ci-- {
		if m.chunks[ci].kind == kindChange {
			m.applyChunk(m.chunks[ci], toRight)
		}
	}
	m.recompute()
	m.status = fmt.Sprintf("applied %s %d hunks", arrowOf(toRight), pending)
}

// resetAll resets every applied hunk, undoable in one step.
func (m *model) resetAll() {
	if len(m.applied) == 0 {
		m.status = "nothing to reset"
		return
	}
	m.pushUndo()
	n := len(m.applied)
	// back to front by position: resetting a region only shifts regions after it
	for len(m.applied) > 0 {
		last := 0
		for i, a := range m.applied {
			if a.l0 > m.applied[last].l0 {
				last = i
			}
		}
		m.resetRegion(last)
	}
	m.recompute()
	m.status = fmt.Sprintf("↺ reset %d hunks", n)
}

func (m *model) computeMatches() {
	m.matches = m.matches[:0]
	m.matchIdx = -1
	q := strings.ToLower(m.search)
	if q == "" {
		return
	}
	for i, r := range m.rows {
		if (r.l >= 0 && strings.Contains(strings.ToLower(m.left[r.l]), q)) ||
			(r.r >= 0 && strings.Contains(strings.ToLower(m.right[r.r]), q)) {
			m.matches = append(m.matches, i)
		}
	}
}

func (m *model) gotoMatch(delta int) {
	if len(m.matches) == 0 {
		m.status = "no matches"
		return
	}
	if m.matchIdx < 0 {
		m.matchIdx = 0
		for i, row := range m.matches {
			if row >= m.top {
				m.matchIdx = i
				break
			}
		}
	} else {
		m.matchIdx = (m.matchIdx + delta + len(m.matches)) % len(m.matches)
	}
	m.top = clamp(m.matches[m.matchIdx]-m.bodyH()/3, 0, m.maxTop())
}

func (m *model) bodyH() int  { return max(1, m.h-2) }
func (m *model) maxTop() int { return max(0, len(m.rows)-m.bodyH()) }

func (m *model) clampScroll() { m.top = clamp(m.top, 0, m.maxTop()) }

func (m *model) scrollToCur() {
	if len(m.nav) == 0 {
		return
	}
	m.top = clamp(m.nav[m.cur].row-m.bodyH()/3, 0, m.maxTop())
}

func (m *model) apply(toRight bool) {
	if len(m.nav) == 0 {
		m.status = "no changes to apply"
		return
	}
	if m.nav[m.cur].ci < 0 {
		m.status = "chunk already applied"
		return
	}
	if !m.canApply(toRight) {
		return
	}
	m.pushUndo()
	m.applyChunk(m.chunks[m.nav[m.cur].ci], toRight)
	m.status = "applied " + arrowOf(toRight)
	// deliberately no scrollToCur: the view stays put after an apply
	m.recompute()
}

// applyChunk copies chunk c onto the other side and records the applied
// region; callers snapshot and recompute.
func (m *model) applyChunk(c chunk, toRight bool) {
	if toRight {
		if len(m.right) == 0 {
			m.rightNL = m.leftNL
		}
		orig := append([]string(nil), m.right[c.r0:c.r1]...)
		m.right = splice(m.right, c.r0, c.r1, m.left[c.l0:c.l1])
		m.shiftApplied(true, c.r1, (c.l1-c.l0)-(c.r1-c.r0), -1)
		m.applied = append(m.applied, appliedRegion{c.l0, c.l1, c.r0, c.r0 + (c.l1 - c.l0), true, orig})
		return
	}
	if len(m.left) == 0 {
		m.leftNL = m.rightNL
	}
	orig := append([]string(nil), m.left[c.l0:c.l1]...)
	m.left = splice(m.left, c.l0, c.l1, m.right[c.r0:c.r1])
	m.shiftApplied(false, c.l1, (c.r1-c.r0)-(c.l1-c.l0), -1)
	m.applied = append(m.applied, appliedRegion{c.l0, c.l0 + (c.r1 - c.r0), c.r0, c.r1, false, orig})
}

// resetApplied restores the applied hunk under the cursor to its pre-apply
// state; it becomes a pending change again.
func (m *model) resetApplied() {
	if len(m.nav) == 0 || m.nav[m.cur].ai < 0 {
		m.status = "not on an applied chunk"
		return
	}
	m.pushUndo()
	m.resetRegion(m.nav[m.cur].ai)
	m.status = "↺ reset"
	m.recompute()
}

// resetRegion puts the pre-apply content back for applied region ai and
// drops the region; callers snapshot and recompute.
func (m *model) resetRegion(ai int) {
	a := m.applied[ai]
	if a.toRight {
		m.right = splice(m.right, a.r0, a.r1, a.orig)
		m.shiftApplied(true, a.r1, len(a.orig)-(a.r1-a.r0), ai)
	} else {
		m.left = splice(m.left, a.l0, a.l1, a.orig)
		m.shiftApplied(false, a.l1, len(a.orig)-(a.l1-a.l0), ai)
	}
	m.applied = append(m.applied[:ai], m.applied[ai+1:]...)
}

// chunkRows returns the first and last row index of the current chunk.
func (m *model) chunkRows() (int, int) {
	t := m.nav[m.cur]
	c := m.chunks[t.ci]
	return t.row, t.row + max(c.l1-c.l0, c.r1-c.r0) - 1
}

// applySelection applies only the visually selected rows of the current
// chunk: the rows map to a sub-range on each side, which is applied like a
// chunk of its own; the rest stays pending.
func (m *model) applySelection(toRight bool) {
	if !m.canApply(toRight) {
		return
	}
	c := m.chunks[m.nav[m.cur].ci]
	first, _ := m.chunkRows()
	a, b := min(m.vAnchor, m.vCur)-first, max(m.vAnchor, m.vCur)-first+1
	nL, nR := c.l1-c.l0, c.r1-c.r0
	sub := chunk{kindChange, c.l0 + min(a, nL), c.l0 + min(b, nL), c.r0 + min(a, nR), c.r0 + min(b, nR)}
	m.visual = false
	if sub.l0 == sub.l1 && sub.r0 == sub.r1 {
		m.status = "nothing to apply in the selection"
		return
	}
	m.pushUndo()
	m.applyChunk(sub, toRight)
	m.status = fmt.Sprintf("applied %s %d lines", arrowOf(toRight), b-a)
	m.recompute()
}

// shiftApplied moves applied regions starting at or after `from` on one
// side by delta lines (skip excludes the region being edited).
func (m *model) shiftApplied(right bool, from, delta, skip int) {
	for i := range m.applied {
		a := &m.applied[i]
		if i == skip {
			continue
		}
		if right && a.r0 >= from {
			a.r0 += delta
			a.r1 += delta
		}
		if !right && a.l0 >= from {
			a.l0 += delta
			a.l1 += delta
		}
	}
}

func (m *model) pushUndo() {
	side := 0
	if m.merge != nil {
		side = m.merge.idx
	}
	m.undo = append(m.undo, snapshot{m.left, m.right, m.cur, append([]appliedRegion(nil), m.applied...), side})
}

func (m *model) leftDirty() bool  { return !sameLines(m.left, m.savedL) }
func (m *model) rightDirty() bool { return !sameLines(m.right, m.savedR) }
func (m *model) dirty() bool      { return m.leftDirty() || m.rightDirty() }

// canApply checks the target side is writable, setting the status otherwise.
func (m *model) canApply(toRight bool) bool {
	if toRight && m.roRight {
		m.status = "right side is read-only (git ref)"
		return false
	}
	if !toRight && m.roLeft {
		m.status = "left side is read-only (git ref)"
		return false
	}
	return true
}

// sameLines reports whether two line slices are the same slice (identity,
// not content): every edit produces a fresh slice, so identity == unchanged.
func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

func arrowOf(toRight bool) string {
	if toRight {
		return "▶"
	}
	return "◀"
}

// appliedAt returns the applied-region index a row lies in (-1 if none)
// and the direction the region was applied in.
func (m *model) appliedAt(r row) (int, bool) {
	if r.l < 0 {
		return -1, false
	}
	for i, a := range m.applied {
		if r.l >= a.l0 && r.l < a.l1 {
			return i, a.toRight
		}
	}
	return -1, false
}

func (m *model) undoLast() {
	if len(m.undo) == 0 {
		m.status = "nothing to undo"
		return
	}
	s := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	if m.merge != nil && s.side != m.merge.idx {
		m.setSide(s.side)
	}
	m.left, m.right, m.cur = s.left, s.right, s.cur
	m.applied = s.applied
	m.recompute()
	m.scrollToCur()
	m.status = "↺ undone"
}

func (m *model) save() {
	var saved []string
	if m.leftDirty() {
		if err := writeLines(m.leftPath, m.left, m.leftNL); err != nil {
			m.status = "error: " + err.Error()
			return
		}
		m.savedL = m.left
		saved = append(saved, m.leftPath)
	}
	if m.rightDirty() {
		if err := writeLines(m.rightPath, m.right, m.rightNL); err != nil {
			m.status = "error: " + err.Error()
			return
		}
		m.savedR = m.right
		saved = append(saved, m.rightPath)
	}
	if len(saved) == 0 {
		m.status = "nothing to save"
	} else {
		m.status = "✓ saved " + strings.Join(saved, ", ")
	}
}

func (m *model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clampScroll()
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.top -= 3
		case tea.MouseButtonWheelDown:
			m.top += 3
		case tea.MouseButtonWheelLeft:
			m.scrollH(-8)
		case tea.MouseButtonWheelRight:
			m.scrollH(8)
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress {
				m.selectRow(m.rowAtLine(msg.Y - 1))
			}
		}
		m.clampScroll()
	case tea.KeyMsg:
		key := msg.String()
		if m.searchInput {
			switch key {
			case "enter":
				m.searchInput = false
				m.computeMatches()
				m.gotoMatch(1)
			case "esc", "ctrl+c":
				m.searchInput = false
				m.search = ""
				m.matches = nil
			default:
				m.search = editText(m.search, msg)
			}
			return nil
		}
		if m.visual {
			first, last := m.chunkRows()
			switch key {
			case "j", "down":
				m.vCur = min(m.vCur+1, last)
			case "k", "up":
				m.vCur = max(m.vCur-1, first)
			case "l", "right", ">":
				m.applySelection(true)
			case "h", "left", "<":
				m.applySelection(false)
			case "esc", "v", "q":
				m.visual = false
				m.status = "selection cancelled"
			}
			m.top = clamp(m.top, max(0, m.vCur-m.bodyH()+1), min(m.vCur, m.maxTop()))
			return nil
		}
		if m.pendingAll {
			m.pendingAll = false
			switch key {
			case "l", "right", ">":
				m.applyAll(true)
			case "h", "left", "<":
				m.applyAll(false)
			default:
				m.status = "apply all cancelled"
			}
			return nil
		}
		if key != "q" && key != "esc" && key != "ctrl+c" {
			m.quitConfirm = false
			m.status = ""
		}
		switch key {
		case "q", "esc", "ctrl+c":
			if key == "esc" && m.search != "" {
				m.search = ""
				m.matches = nil
				m.status = "search cleared"
				return nil
			}
			if m.dirty() && !m.quitConfirm {
				m.quitConfirm = true
				m.status = "unsaved changes — q again to discard, s to save"
				return nil
			}
			if m.merge != nil && m.conflicts() > 0 && !m.quitConfirm {
				m.quitConfirm = true
				m.status = fmt.Sprintf("%d conflicts remain — q again to quit anyway", m.conflicts())
				return nil
			}
			if key == "ctrl+c" {
				return tea.Quit
			}
			return func() tea.Msg { return closeFileMsg{} }
		case "j", "down":
			m.top++
		case "k", "up":
			m.top--
		case "ctrl+d":
			m.top += m.bodyH() / 2
		case "ctrl+u":
			m.top -= m.bodyH() / 2
		case "g":
			m.top = 0
		case "G":
			m.top = m.maxTop()
		case "i":
			cfg.Intraline = !cfg.Intraline
			m.status = "intraline highlight " + onOff(cfg.Intraline)
		case "w":
			cfg.Wrap = !cfg.Wrap
			saveConfig()
			m.status = "line wrap " + onOff(cfg.Wrap)
		case "H":
			m.scrollH(-8)
		case "L":
			m.scrollH(8)
		case "n", "]":
			if key == "n" && m.search != "" {
				m.gotoMatch(1)
				break
			}
			if m.cur < len(m.nav)-1 {
				m.cur++
			}
			m.scrollToCur()
		case "N":
			if m.search != "" {
				m.gotoMatch(-1)
			}
		case "/":
			m.searchInput = true
			m.search = ""
			m.matches = nil
		case "a":
			m.pendingAll = true
			m.status = "apply all: l ▶ · h ◀ · esc cancel"
		case "X":
			m.resetAll()
		case "1", "2", "3":
			if m.merge != nil {
				m.switchSide(int(key[0] - '1'))
			}
		case "v":
			if len(m.nav) == 0 || m.nav[m.cur].ci < 0 {
				m.status = "select lines within a pending change"
				break
			}
			m.visual = true
			m.vAnchor, m.vCur = m.nav[m.cur].row, m.nav[m.cur].row
			m.status = "visual: j/k extend · l ▶ h ◀ apply lines · esc cancel"
		case "J", "K":
			d := 1
			if key == "K" {
				d = -1
			}
			return func() tea.Msg { return switchFileMsg{d} }
		case "p", "[":
			if m.cur > 0 {
				m.cur--
			}
			m.scrollToCur()
		case "l", "right", ">":
			m.apply(true)
		case "h", "left", "<":
			m.apply(false)
		case "x":
			m.resetApplied()
		case "u":
			m.undoLast()
		case "s":
			m.save()
		}
		m.clampScroll()
	}
	return nil
}

func (m *model) view(focused bool) string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	paneW, gutW, textW := m.geometry()
	var b strings.Builder

	hs := headerStyles(focused)
	head := hs.mark(focused) +
		pathCell(displayPath(m.leftName), paneW, m.leftDirty(), hs) +
		hs.sep.Render("│") +
		pathCell(displayPath(m.rightName), paneW, m.rightDirty(), hs)
	b.WriteString(barPadWith(head, m.w, hs.bar) + "\n")

	curCi, curAi := -1, -1
	if len(m.nav) > 0 {
		curCi, curAi = m.nav[m.cur].ci, m.nav[m.cur].ai
	}
	pad := strings.Repeat(" ", max(0, m.w-1-(2+2*paneW)))
	lines := 0
	for i := m.top; lines < m.bodyH(); i++ {
		sb := m.scrollbar(lines)
		if i >= len(m.rows) {
			b.WriteString(strings.Repeat(" ", max(0, m.w-1)) + sb + "\n")
			lines++
			continue
		}
		r := m.rows[i]
		ai, toRight := m.appliedAt(r)
		isCur := (curCi >= 0 && r.ci == curCi) || (curAi >= 0 && ai == curAi)
		if m.visual { // highlight only the selected rows
			isCur = i >= min(m.vAnchor, m.vCur) && i <= max(m.vAnchor, m.vCur)
		}
		mark := " "
		if isCur {
			mark = styleMark.Render("▌")
		}
		if m.matchIdx >= 0 && m.matchIdx < len(m.matches) && i == m.matches[m.matchIdx] {
			mark = styleMark.Render("▸")
		}
		sep := styleSep.Render("│")
		if ai >= 0 {
			if toRight {
				sep = styleAppliedMark.Render("▶")
			} else {
				sep = styleAppliedMark.Render("◀")
			}
		}
		var sa, sb2 []span
		if c := m.chunks[r.ci]; c.kind == kindChange && c.l1 > c.l0 && c.r1 > c.r0 &&
			r.l >= 0 && r.r >= 0 && cfg.Intraline {
			sa, sb2 = changedSpans([]rune(expandTabs(m.left[r.l])), []rune(expandTabs(m.right[r.r])))
		}
		var hlL, hlR []span
		if m.search != "" {
			if r.l >= 0 {
				hlL = searchSpans(expandTabs(m.left[r.l]), m.search)
			}
			if r.r >= 0 {
				hlR = searchSpans(expandTabs(m.right[r.r]), m.search)
			}
		}
		pieces := 1
		if cfg.Wrap {
			pieces = max(m.wrapCount(r.l, m.left, textW), m.wrapCount(r.r, m.right, textW))
		}
		for k := 0; k < pieces && lines < m.bodyH(); k++ {
			if k > 0 { // continuation lines keep the chunk marker, not the match marker
				mark = " "
				if isCur {
					mark = styleMark.Render("▌")
				}
				sb = m.scrollbar(lines)
			}
			b.WriteString(mark +
				m.renderSide(r, true, paneW, gutW, isCur, sa, hlL, k) +
				sep +
				m.renderSide(r, false, paneW, gutW, isCur, sb2, hlR, k) + pad + sb + "\n")
			lines++
		}
	}

	info := ""
	if len(m.nav) > 0 {
		info = fmt.Sprintf("change %d/%d", m.cur+1, len(m.nav))
	}
	if m.hOff > 0 && !cfg.Wrap {
		if info != "" {
			info += " · "
		}
		info += fmt.Sprintf("⇆ %d", m.hOff)
	}
	if m.search != "" && len(m.matches) > 0 && m.matchIdx >= 0 {
		info += fmt.Sprintf(" · /%s %d/%d", m.search, m.matchIdx+1, len(m.matches))
	}
	if m.visual {
		info += fmt.Sprintf(" · visual %d lines", max(m.vAnchor, m.vCur)-min(m.vAnchor, m.vCur)+1)
	}
	if m.merge != nil {
		info += fmt.Sprintf(" · %d conflicts · left 1·2·3: %s", m.conflicts(), m.merge.sides[m.merge.idx].name)
	}
	status := m.status
	if m.searchInput {
		status = "/" + m.search + "▏"
	}
	b.WriteString(footerBar(m.w, status, info, [][2]string{
		{"n/p", "change"}, {"h·l", "◀ apply ▶"}, {"a", "all"}, {"x", "reset"},
		{"u", "undo"}, {"s", "save"}, {"/", "search"}, {"?", "help"}, {"q", "quit"},
	}))
	return b.String()
}

// scrollbar returns the right-edge cell for body line bi: colored marks
// where changes and applied hunks live in the file, plus a viewport thumb.
func (m *model) scrollbar(bi int) string {
	bodyH := m.bodyH()
	n := len(m.rows)
	if n == 0 {
		return " "
	}
	seg0, seg1 := bi, bi+1
	if n > bodyH {
		seg0 = bi * n / bodyH
		seg1 = max(seg0+1, (bi+1)*n/bodyH)
	}
	if seg0 >= n {
		return " "
	}
	seg1 = min(seg1, n)
	inThumb := n > bodyH && seg0 < m.top+bodyH && seg1 > m.top
	fg := ""
	for i := seg0; i < seg1 && fg == ""; i++ {
		r := m.rows[i]
		c := m.chunks[r.ci]
		if c.kind == kindChange {
			switch {
			case c.l1 > c.l0 && c.r1 > c.r0:
				fg = th.stMod
			case c.l1 > c.l0:
				fg = th.stLeft
			default:
				fg = th.stRight
			}
		} else if ai, _ := m.appliedAt(r); ai >= 0 {
			fg = th.stApplied
		}
	}
	st := lipgloss.NewStyle()
	if inThumb {
		st = st.Background(lipgloss.Color(th.selBg))
	}
	if fg != "" {
		return st.Foreground(lipgloss.Color(fg)).Render("▐")
	}
	return st.Render(" ")
}

// geometry returns the pane width, gutter width and text width of a side.
func (m *model) geometry() (paneW, gutW, textW int) {
	paneW = max(10, (m.w-3)/2)
	gutW = len(fmt.Sprint(max(len(m.left), len(m.right), 1)))
	return paneW, gutW, max(1, paneW-gutW-2)
}

// rowAtLine maps a body screen line to a row index (-1 if none), taking
// wrapped rows into account.
func (m *model) rowAtLine(y int) int {
	if y < 0 {
		return -1
	}
	if !cfg.Wrap {
		if r := m.top + y; r < len(m.rows) {
			return r
		}
		return -1
	}
	_, _, textW := m.geometry()
	lines := 0
	for i := m.top; i < len(m.rows); i++ {
		r := m.rows[i]
		n := max(m.wrapCount(r.l, m.left, textW), m.wrapCount(r.r, m.right, textW))
		if y < lines+n {
			return i
		}
		lines += n
	}
	return -1
}

// selectRow moves the cursor to the change or applied hunk containing row.
func (m *model) selectRow(row int) {
	if row < 0 || row >= len(m.rows) {
		return
	}
	r := m.rows[row]
	ai, _ := m.appliedAt(r)
	for i, t := range m.nav {
		if (t.ci >= 0 && t.ci == r.ci) || (t.ai >= 0 && t.ai == ai) {
			m.cur = i
			return
		}
	}
}

// wrapCount is the number of screen lines a wrapped line needs (1 for none).
func (m *model) wrapCount(idx int, lines []string, textW int) int {
	if idx < 0 {
		return 1
	}
	return max(1, (runewidth.StringWidth(expandTabs(lines[idx]))+textW-1)/textW)
}

// renderSide draws one side of a row; spans are the intraline changes of
// this side (computed once per row by the caller). piece selects the
// screen line of a wrapped row (0 = first).
func (m *model) renderSide(r row, isLeft bool, paneW, gutW int, cur bool, spans, hl []span, piece int) string {
	textW := max(1, paneW-gutW-2)
	idx := r.r
	lines := m.right
	if isLeft {
		idx, lines = r.l, m.left
	}
	c := m.chunks[r.ci]
	if idx < 0 {
		st := styleVoid
		if cur {
			st = styleVoidCur
		}
		return strings.Repeat(" ", gutW+1) + st.Render(strings.Repeat("╱", textW)) + " "
	}
	txt := expandTabs(lines[idx])
	gutSt := styleGutter

	base := lipgloss.NewStyle()
	emph := base
	switch {
	case c.kind == kindEqual:
		if ai, _ := m.appliedAt(r); ai >= 0 {
			base = styleApplied
			gutSt = styleStApplied
			if cur {
				base = styleAppliedCur
			}
		}
	case c.l1 > c.l0 && c.r1 > c.r0:
		base, emph = styleMod, styleModEmph
		gutSt = styleStModified
		if cur {
			base, emph = styleModCur, styleModEmphCur
		}
	case isLeft:
		base = styleDel
		gutSt = styleStOnlyLeft
		if cur {
			base = styleDelCur
		}
	default:
		base = styleIns
		gutSt = styleStOnlyRight
		if cur {
			base = styleInsCur
		}
	}
	var fgs []fgSpan
	if isLeft {
		if idx < len(m.leftFgs) {
			fgs = m.leftFgs[idx]
		}
	} else if idx < len(m.rightFgs) {
		fgs = m.rightFgs[idx]
	}
	gut := gutSt.Render(fmt.Sprintf("%*d ", gutW, idx+1))
	off, marks := m.hOff, true
	if cfg.Wrap {
		off, marks = piece*textW, false
	}
	if piece > 0 {
		gut = strings.Repeat(" ", gutW+1)
	}
	return gut + clipAndStyle(txt, spans, fgs, hl, off, textW, marks, base, emph) + " "
}

// searchSpans returns the rune ranges of s that match q case-insensitively.
func searchSpans(s, q string) []span {
	ls, lq := strings.ToLower(s), strings.ToLower(q)
	if lq == "" || utf8.RuneCountInString(ls) != utf8.RuneCountInString(s) {
		return nil
	}
	var out []span
	qr := utf8.RuneCountInString(lq)
	for from := 0; ; {
		i := strings.Index(ls[from:], lq)
		if i < 0 {
			return out
		}
		start := utf8.RuneCountInString(ls[:from+i])
		out = append(out, span{start, start + qr})
		from += i + len(lq)
	}
}

func expandAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = expandTabs(l)
	}
	return out
}

// expandTabs prepares a line for display: tabs become spaces and control
// characters become their Unicode control pictures (␍ for CR, …) — emitted
// raw, a CR would move the cursor to column 0 and garble the whole row.
func expandTabs(s string) string {
	s = strings.ReplaceAll(s, "\t", strings.Repeat(" ", cfg.TabWidth))
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 0x7f:
			b.WriteRune('␡')
		case r < 0x20:
			b.WriteRune(rune(0x2400 + r))
		case r >= 0x80 && r < 0xa0:
			b.WriteRune('�')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isControl(r rune) bool { return r < 0x20 || (r >= 0x7f && r < 0xa0) }

// clipAndStyle renders s from display column off into exactly width w,
// styling runes inside spans with emph and the rest with base; clipped
// edges show a dim "…".
func clipAndStyle(s string, spans []span, fgs []fgSpan, hl []span, off, w int, marks bool, base, emph lipgloss.Style) string {
	runes := []rune(s)
	total := runewidth.StringWidth(s)
	if off > 0 && off >= total {
		return base.Render(strings.Repeat(" ", w))
	}
	leftClip := marks && off > 0
	startCol := off
	used := 0
	if leftClip {
		startCol++
		used = 1
	}
	inSpans := func(sps []span, i int) bool {
		for _, sp := range sps {
			if i >= sp.a && i < sp.b {
				return true
			}
		}
		return false
	}
	fgAt := func(i int) string {
		for _, f := range fgs {
			if i >= f.a && i < f.b {
				return f.fg
			}
		}
		return ""
	}
	type cell struct {
		r    rune
		emph bool
		hl   bool
		fg   string
	}
	var vis []cell
	col := 0
	clippedR := false
	for i, rn := range runes {
		rw := runewidth.RuneWidth(rn)
		if col+rw <= startCol {
			col += rw
			continue
		}
		if used+rw > w {
			clippedR = true
			break
		}
		vis = append(vis, cell{rn, inSpans(spans, i), inSpans(hl, i), fgAt(i)})
		used += rw
		col += rw
	}
	clippedR = clippedR && marks
	if clippedR {
		for used > w-1 && len(vis) > 0 {
			used -= runewidth.RuneWidth(vis[len(vis)-1].r)
			vis = vis[:len(vis)-1]
		}
	}
	var b strings.Builder
	if leftClip {
		b.WriteString(styleGutter.Render("…"))
	}
	var seg []rune
	segEmph, segHl := false, false
	segFg := ""
	flush := func() {
		if len(seg) == 0 {
			return
		}
		st := base
		switch {
		case segHl:
			st = styleSearch
		case segEmph:
			st = emph
		}
		if segFg != "" && !segHl {
			st = st.Foreground(lipgloss.Color(segFg))
		}
		b.WriteString(st.Render(string(seg)))
		seg = seg[:0]
	}
	for _, cl := range vis {
		if cl.emph != segEmph || cl.fg != segFg || cl.hl != segHl {
			flush()
			segEmph, segFg, segHl = cl.emph, cl.fg, cl.hl
		}
		seg = append(seg, cl.r)
	}
	flush()
	if clippedR {
		b.WriteString(styleGutter.Render("…"))
		used++
	}
	if used < w {
		b.WriteString(base.Render(strings.Repeat(" ", w-used)))
	}
	return b.String()
}

// pathCell renders a file path with dim directory and bold basename,
// left-truncated to fit, plus a dirty marker.
func pathCell(p string, w int, dirty bool, hs hdrStyles) string {
	avail := max(4, w-1)
	if dirty {
		avail -= 2
	}
	dir, base := filepath.Split(shortenPath(p, avail))
	s := hs.dim.Render(dir) + hs.text.Render(base)
	if dirty {
		s += hs.bar.Render(" ") + hs.dirty.Render("*")
	}
	return barPadWith(s, w, hs.bar)
}

// distinctTails strips the leading path components a and b have in common,
// so both start at the first component that differs (at least the last
// component is always kept).
func distinctTails(a, b string) (string, string) {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	i := 0
	for i < len(as)-1 && i < len(bs)-1 && as[i] == bs[i] {
		i++
	}
	return strings.Join(as[i:], "/"), strings.Join(bs[i:], "/")
}

// shortenPath fits p into w cells by dropping middle components
// ("a/b/c/d.php" → "a/…/d.php"), so the distinguishing head and the file
// name survive; falls back to left truncation.
func shortenPath(p string, w int) string {
	if runewidth.StringWidth(p) <= w {
		return p
	}
	parts := strings.Split(p, "/")
	for len(parts) > 2 {
		// drop the component after the head: keeps the head and as much of
		// the tail (the file's parent dirs) as fits
		parts = append(parts[:1], parts[2:]...)
		cand := parts[0] + "/…/" + strings.Join(parts[1:], "/")
		if runewidth.StringWidth(cand) <= w {
			return cand
		}
	}
	return truncLeft(p, w)
}

// truncLeft keeps the tail of s within w display cells, marking the cut with "…".
func truncLeft(s string, w int) string {
	if sw := runewidth.StringWidth(s); sw > w {
		return runewidth.TruncateLeft(s, sw-w+1, "…")
	}
	return s
}

var homeDir, _ = os.UserHomeDir()

// displayPath shortens the home directory to ~ for display.
func displayPath(p string) string {
	if homeDir != "" && strings.HasPrefix(p, homeDir+"/") {
		return "~" + p[len(homeDir):]
	}
	return p
}

const maxHOff = 4000

func (m *model) scrollH(delta int) {
	if cfg.Wrap {
		m.status = "line wrap is on — nothing to scroll (w toggles)"
		return
	}
	m.hOff = clamp(m.hOff+delta, 0, maxHOff)
}

// editText applies a key to a one-line text input: backspace drops the last
// rune, printable runes and space are appended.
func editText(s string, k tea.KeyMsg) string {
	switch k.Type {
	case tea.KeyBackspace:
		if rs := []rune(s); len(rs) > 0 {
			return string(rs[:len(rs)-1])
		}
	case tea.KeyRunes:
		return s + string(k.Runes)
	case tea.KeySpace:
		return s + " "
	}
	return s
}

// writeFileMkdir writes data to path, creating parent directories.
func writeFileMkdir(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
