package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type theme struct {
	text, muted, subtle, accent, sep string
	barBg                            string // header/footer bar
	delBg, delCurBg                  string
	insBg, insCurBg                  string
	modBg, modCurBg                  string
	modEmphBg, modEmphCurBg          string
	voidBg, voidCurBg                string
	appliedBg, appliedCurBg          string
	appliedFg                        string
	selBg                            string
	stMod, stLeft, stRight, stSame   string
	stApplied                        string
	chromaStyle                      string // chroma syntax style; empty = no highlighting
}

var themes = map[string]theme{
	"rose-pine": {
		chromaStyle: "rose-pine",
		text:        "#e0def4", muted: "#6e6a86", subtle: "#908caa", accent: "#f6c177", sep: "#403d52",
		barBg: "#1f1d2e",
		delBg: "#392636", delCurBg: "#57344e",
		insBg: "#23372f", insCurBg: "#2f4f42",
		modBg: "#1f3641", modCurBg: "#2b4a5a",
		modEmphBg: "#38607c", modEmphCurBg: "#48799c",
		voidBg: "#1f1d2e", voidCurBg: "#26233a",
		appliedBg: "#2a273f", appliedCurBg: "#393552", appliedFg: "#c4a7e7",
		selBg: "#26233a",
		stMod: "#f6c177", stLeft: "#eb6f92", stRight: "#9ccfd8", stSame: "#6e6a86",
		stApplied: "#c4a7e7",
	},
	"nord": {
		chromaStyle: "nord",
		text:        "#eceff4", muted: "#616e88", subtle: "#81a1c1", accent: "#ebcb8b", sep: "#434c5e",
		barBg: "#3b4252",
		delBg: "#4c3540", delCurBg: "#6a4450",
		insBg: "#404c40", insCurBg: "#526347",
		modBg: "#3d4c60", modCurBg: "#4c617a",
		modEmphBg: "#5b7492", modEmphCurBg: "#6d88ab",
		voidBg: "#333a47", voidCurBg: "#3b4252",
		appliedBg: "#443c4f", appliedCurBg: "#554962", appliedFg: "#b48ead",
		selBg: "#434c5e",
		stMod: "#ebcb8b", stLeft: "#bf616a", stRight: "#a3be8c", stSame: "#616e88",
		stApplied: "#b48ead",
	},
	"catppuccin": {
		chromaStyle: "catppuccin-mocha",
		text:        "#cdd6f4", muted: "#6c7086", subtle: "#a6adc8", accent: "#f9e2af", sep: "#45475a",
		barBg: "#181825",
		delBg: "#46293a", delCurBg: "#613a50",
		insBg: "#2e3c30", insCurBg: "#3f5440",
		modBg: "#2c3a55", modCurBg: "#3b4d70",
		modEmphBg: "#4c6493", modEmphCurBg: "#5d78b0",
		voidBg: "#262637", voidCurBg: "#313244",
		appliedBg: "#35304a", appliedCurBg: "#453e60", appliedFg: "#cba6f7",
		selBg: "#313244",
		stMod: "#f9e2af", stLeft: "#f38ba8", stRight: "#a6e3a1", stSame: "#6c7086",
		stApplied: "#cba6f7",
	},
	"dracula": {
		chromaStyle: "dracula",
		text:        "#f8f8f2", muted: "#6272a4", subtle: "#9ea8c7", accent: "#ffb86c", sep: "#44475a",
		barBg: "#21222c",
		delBg: "#4b3038", delCurBg: "#6b3c44",
		insBg: "#2e4438", insCurBg: "#3c5c48",
		modBg: "#363152", modCurBg: "#484169",
		modEmphBg: "#5c5388", modEmphCurBg: "#6f65a3",
		voidBg: "#23242e", voidCurBg: "#2c2e3c",
		appliedBg: "#2a3d47", appliedCurBg: "#37525f", appliedFg: "#8be9fd",
		selBg: "#44475a",
		stMod: "#ffb86c", stLeft: "#ff5555", stRight: "#50fa7b", stSame: "#6272a4",
		stApplied: "#8be9fd",
	},
	// JetBrains Darcula, diff colors close to the IDE's
	"darcula": {
		chromaStyle: "native",
		text:        "#a9b7c6", muted: "#606366", subtle: "#808080", accent: "#ffc66d", sep: "#3c3f41",
		barBg: "#3c3f41",
		delBg: "#4c3333", delCurBg: "#5e3f3f",
		insBg: "#294436", insCurBg: "#375c48",
		modBg: "#385570", modCurBg: "#43668a",
		modEmphBg: "#537ba3", modEmphCurBg: "#6290bd",
		voidBg: "#313335", voidCurBg: "#3c3f41",
		appliedBg: "#413a4a", appliedCurBg: "#524a5e", appliedFg: "#9876aa",
		selBg: "#354b5c",
		stMod: "#6897bb", stLeft: "#cc666b", stRight: "#629755", stSame: "#606366",
		stApplied: "#9876aa",
	},
	"gruvbox": {
		chromaStyle: "gruvbox",
		text:        "#ebdbb2", muted: "#928374", subtle: "#a89984", accent: "#fabd2f", sep: "#504945",
		barBg: "#3c3836",
		delBg: "#4a2e2a", delCurBg: "#663a32",
		insBg: "#3c3f22", insCurBg: "#545a24",
		modBg: "#35424a", modCurBg: "#45565f",
		modEmphBg: "#547181", modEmphCurBg: "#64879a",
		voidBg: "#32302f", voidCurBg: "#3c3836",
		appliedBg: "#453439", appliedCurBg: "#59424a", appliedFg: "#d3869b",
		selBg: "#504945",
		stMod: "#fabd2f", stLeft: "#fb4934", stRight: "#b8bb26", stSame: "#928374",
		stApplied: "#d3869b",
	},
	"tokyonight": {
		chromaStyle: "tokyonight-night",
		text:        "#c0caf5", muted: "#565f89", subtle: "#737aa2", accent: "#e0af68", sep: "#3b4261",
		barBg: "#16161e",
		delBg: "#3a2432", delCurBg: "#542f44",
		insBg: "#283b26", insCurBg: "#354f30",
		modBg: "#24325e", modCurBg: "#304279",
		modEmphBg: "#41579e", modEmphCurBg: "#4f68bd",
		voidBg: "#1f2030", voidCurBg: "#292a3d",
		appliedBg: "#2e2a48", appliedCurBg: "#3d3860", appliedFg: "#bb9af7",
		selBg: "#2e3350",
		stMod: "#e0af68", stLeft: "#f7768e", stRight: "#9ece6a", stSame: "#565f89",
		stApplied: "#bb9af7",
	},
	// 256-color fallback for terminals without truecolor
	"ansi": {
		muted: "241", subtle: "245", accent: "214", sep: "238",
		delBg: "52", delCurBg: "88",
		insBg: "22", insCurBg: "28",
		modBg: "17", modCurBg: "24",
		modEmphBg: "25", modEmphCurBg: "31",
		voidBg: "235", voidCurBg: "237",
		appliedBg: "23", appliedCurBg: "30", appliedFg: "80",
		selBg: "237",
		stMod: "179", stLeft: "167", stRight: "71", stSame: "241",
		stApplied: "141",
	},
}

var (
	th theme

	styleHeaderText, styleHeaderDim, styleHeaderSep, styleBar, styleDirty lipgloss.Style
	styleGutter, styleSep, styleMark                                      lipgloss.Style
	styleFooterText, styleFooterKey, styleStatus                          lipgloss.Style
	styleDel, styleIns, styleMod, styleVoid                               lipgloss.Style
	styleApplied, styleAppliedCur, styleAppliedMark                       lipgloss.Style
	styleStApplied                                                        lipgloss.Style
	styleModEmph, styleModEmphCur                                         lipgloss.Style
	styleDelCur, styleInsCur, styleModCur, styleVoidCur                   lipgloss.Style
	styleStModified, styleStOnlyLeft, styleStOnlyRight, styleStSame       lipgloss.Style
	styleGroup                                                            lipgloss.Style
	styleSelected                                                         lipgloss.Style
)

func initStyles(t theme) {
	th = t
	fgbg := func(fg, bg string) lipgloss.Style {
		s := lipgloss.NewStyle()
		if fg != "" {
			s = s.Foreground(lipgloss.Color(fg))
		}
		if bg != "" {
			s = s.Background(lipgloss.Color(bg))
		}
		return s
	}
	styleHeaderText = fgbg(t.text, t.barBg).Bold(true)
	styleHeaderDim = fgbg(t.subtle, t.barBg)
	styleHeaderSep = fgbg(t.sep, t.barBg)
	styleBar = fgbg("", t.barBg)
	styleDirty = fgbg(t.accent, t.barBg).Bold(true)
	styleGutter = fgbg(t.muted, "")
	styleSep = fgbg(t.sep, "")
	styleMark = fgbg(t.accent, "").Bold(true)
	styleFooterText = fgbg(t.subtle, t.barBg)
	styleFooterKey = fgbg(t.text, t.barBg).Bold(true)
	styleStatus = fgbg(t.accent, t.barBg).Bold(true)
	styleDel = fgbg("", t.delBg)
	styleIns = fgbg("", t.insBg)
	styleMod = fgbg("", t.modBg)
	styleVoid = fgbg(t.sep, t.voidBg)
	styleDelCur = fgbg("", t.delCurBg)
	styleInsCur = fgbg("", t.insCurBg)
	styleModCur = fgbg("", t.modCurBg)
	styleModEmph = fgbg("", t.modEmphBg)
	styleModEmphCur = fgbg("", t.modEmphCurBg)
	styleVoidCur = fgbg(t.sep, t.voidCurBg)
	styleApplied = fgbg("", t.appliedBg)
	styleAppliedCur = fgbg("", t.appliedCurBg)
	styleAppliedMark = fgbg(t.appliedFg, "").Bold(true)
	styleStModified = fgbg(t.stMod, "")
	styleStOnlyLeft = fgbg(t.stLeft, "")
	styleStOnlyRight = fgbg(t.stRight, "")
	styleStSame = fgbg(t.stSame, "")
	styleStApplied = fgbg(t.stApplied, "")
	styleSelected = fgbg(t.text, t.selBg)
	styleGroup = fgbg(t.subtle, "").Bold(true)
}

// barPad right-pads a styled line to width w with the bar background.
func barPad(s string, w int) string {
	return s + styleBar.Render(strings.Repeat(" ", max(0, w-lipgloss.Width(s))))
}

// footerBar renders the bottom bar: optional status, optional info,
// then key hints.
func footerBar(w int, status, info string, keys [][2]string) string {
	dot := styleFooterText.Render(" · ")
	line := styleBar.Render(" ")
	if status != "" {
		line += styleStatus.Render(status) + dot
	}
	if info != "" {
		line += styleFooterText.Render(info) + dot
	}
	gap := styleFooterText.Render("  ")
	for i, k := range keys {
		hint := styleFooterKey.Render(k[0]) + styleFooterText.Render(" "+k[1])
		if i > 0 {
			hint = gap + hint
		}
		if lipgloss.Width(line)+lipgloss.Width(hint) > w {
			break // whole hints only; the full list lives in the help overlay
		}
		line += hint
	}
	// clip: the bar must never exceed its pane, or side-by-side layout breaks
	return barPad(ansi.Truncate(line, w, ""), w)
}

// padCell right-pads a styled cell to width w (measuring visible width).
func padCell(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}
