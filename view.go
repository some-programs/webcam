package main

import (
	"cmp"
	"fmt"
	"image/color"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	headerStyle      = lipgloss.NewStyle().Bold(true)
	headingStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4))
	faintStyle       = lipgloss.NewStyle().Faint(true)
	statusInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	statusErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true)
	presetStyle      = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5))
	highlightStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(5))
	errorHighlight   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(1))
)

// Widths of the fixed parts of a list line.
const (
	marginWidth = 1 // left and right of everything
	cursorWidth = 2
	gapWidth    = 2
	valueWidth  = 7
	stateWidth  = 8

	defaultWidth = 100 // used until the terminal size is known
	maxNameWidth = 30
	minBarWidth  = 10
	maxBarWidth  = 64
)

// layout holds the positions and sizes of the parts of the view.
type layout struct {
	width     int // content width, without margins
	nameWidth int
	barWidth  int
	// barX is the screen column where bars start.
	barX int
	// listTop is the screen row of the first list line.
	listTop int
	// listHeight is how many list lines fit, or 0 if the terminal height
	// is not known.
	listHeight int
}

func (m Model) layout() layout {
	var l layout
	l.width = defaultWidth
	if m.width > 0 {
		l.width = max(1, m.width-2*marginWidth)
	}
	for _, c := range m.controls {
		l.nameWidth = max(l.nameWidth, ansi.StringWidth(c.Name))
	}
	l.nameWidth = min(maxNameWidth, l.nameWidth)
	fixed := cursorWidth + l.nameWidth + gapWidth + gapWidth + valueWidth + 1 + stateWidth
	l.barWidth = min(maxBarWidth, max(minBarWidth, l.width-fixed))
	// Keep headings and the status line as wide as the list.
	l.width = min(l.width, fixed+l.barWidth)
	l.barX = marginWidth + cursorWidth + l.nameWidth + gapWidth

	// Header, the filter if shown and the line above the list.
	l.listTop = 1 + m.filterHeight() + 1
	if m.height > 0 {
		// The line below the list, the details, the status line and
		// the help.
		below := 1 + 1 + 1 + lipgloss.Height(m.helpView(l.width))
		l.listHeight = max(1, m.height-l.listTop-below)
	}
	return l
}

func (m Model) filterHeight() int {
	if m.filter.Focused() || m.filter.Value() != "" {
		return 1
	}
	return 0
}

// listLine is one line of the control list: a control, or the heading of
// a control class.
type listLine struct {
	isControl bool
	control   Control
	heading   string
}

// listLines returns the visible controls grouped by class.
func (m Model) listLines() []listLine {
	var lines []listLine
	class := uint32(0)
	for i, c := range m.visible() {
		if i == 0 || controlClass(c.ID) != class {
			class = controlClass(c.ID)
			lines = append(lines, listLine{heading: m.cam.ClassName(class)})
		}
		lines = append(lines, listLine{isControl: true, control: c})
	}
	return lines
}

func (m Model) View() tea.View {
	l := m.layout()
	var lines []string

	lines = append(lines, headerStyle.Render(m.cam.Card)+
		faintStyle.Render(fmt.Sprintf(" · %s · %s", m.cam.Path, m.cam.Driver)))

	if m.filterHeight() > 0 {
		lines = append(lines, m.filter.View()+
			faintStyle.Render(fmt.Sprintf("  %d/%d", len(m.visible()), len(m.controls))))
	}

	list := m.listLines()
	start, end := m.offset, len(list)
	if l.listHeight > 0 {
		end = min(end, start+l.listHeight)
	}
	lines = append(lines, moreLine("↑", start))
	current, hasCurrent := m.current()
	for _, line := range list[start:end] {
		if !line.isControl {
			lines = append(lines, headingLine(line.heading, l.width))
			continue
		}
		lines = append(lines, m.controlLine(line.control, hasCurrent && line.control.ID == current.ID, l))
	}
	if len(list) == 0 {
		lines = append(lines, faintStyle.Render("  no controls match the filter"))
	}
	lines = append(lines, moreLine("↓", len(list)-end))

	if hasCurrent {
		lines = append(lines, m.detailLine(current))
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusLine(l.width))
	lines = append(lines, m.helpView(l.width))

	margin := strings.Repeat(" ", marginWidth)
	var b strings.Builder
	for i, line := range strings.Split(strings.Join(lines, "\n"), "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(margin)
		b.WriteString(ansi.Truncate(line, l.width, "…"))
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// moreLine says how many list lines are hidden in direction arrow.
func moreLine(arrow string, n int) string {
	if n <= 0 {
		return ""
	}
	return faintStyle.Render(fmt.Sprintf("  %s %d more", arrow, n))
}

// headingLine renders the heading of a control class.
func headingLine(name string, width int) string {
	prefix := "── " + name + " "
	return headingStyle.Render(prefix) +
		faintStyle.Render(strings.Repeat("─", max(0, width-ansi.StringWidth(prefix))))
}

// controlLine renders c as a list line.
func (m Model) controlLine(c Control, selected bool, l layout) string {
	style := lipgloss.NewStyle()
	if c.state() != "" {
		style = style.Faint(true)
	}

	cursor := strings.Repeat(" ", cursorWidth)
	nameStyle := style
	if selected {
		cursor = "▸ "
		if m.status.isErr {
			nameStyle = style.Inherit(errorHighlight)
		} else {
			nameStyle = style.Inherit(highlightStyle)
		}
	}

	var bar, value string
	switch {
	case c.IsBoolean():
		bar = booleanView(c.Value != c.Min, style)
	case c.IsMenu():
		bar = menuView(c, style)
	default:
		fill := barFillColor
		if selected {
			fill = barSelectedColor
		}
		bar = barView(l.barWidth, c.Percent(), c.percentOf(c.Default), fill, style)
		value = strconv.Itoa(int(c.Value))
	}
	bar += strings.Repeat(" ", max(0, l.barWidth-lipgloss.Width(bar)))

	gap := strings.Repeat(" ", gapWidth)
	return nameStyle.Render(cursor+padRight(c.Name, l.nameWidth)) +
		style.Render(gap) + bar +
		style.Render(gap+padLeft(value, valueWidth)+" "+c.state())
}

var (
	barFillColor     = lipgloss.ANSIColor(6)
	barSelectedColor = lipgloss.ANSIColor(5)
	barTrackColor    = lipgloss.ANSIColor(8)
	barMarkerColor   = lipgloss.ANSIColor(15)
	onColor          = lipgloss.ANSIColor(2)
)

// barEighths are the block characters for 1/8 to 7/8 of a cell.
var barEighths = []rune("▏▎▍▌▋▊▉")

// barView renders a bar width cells wide filled to percent in fill color on
// a track, with a marker at defaultPercent. Every cell is rendered with base
// as well, so that inactive controls look faint.
func barView(width int, percent, defaultPercent float64, fill color.Color, base lipgloss.Style) string {
	if math.IsNaN(percent) {
		percent = 0
	}
	eighths := min(width*8, max(0, int(math.Round(percent*float64(width)*8))))
	full, part := eighths/8, eighths%8
	marker := -1
	if !math.IsNaN(defaultPercent) && defaultPercent >= 0 && defaultPercent <= 1 {
		marker = min(width-1, int(float64(width)*defaultPercent))
	}

	filled := base.Foreground(fill).Background(barTrackColor)
	track := base.Background(barTrackColor)
	var b strings.Builder
	for i := range width {
		switch {
		case i == marker && i < full:
			b.WriteString(base.Foreground(barMarkerColor).Background(fill).Render("│"))
		case i == marker:
			b.WriteString(track.Foreground(barMarkerColor).Render("│"))
		case i < full:
			b.WriteString(filled.Render("█"))
		case i == full && part > 0:
			b.WriteString(filled.Render(string(barEighths[part-1])))
		default:
			b.WriteString(track.Render(" "))
		}
	}
	return b.String()
}

// booleanView renders a boolean control's state.
func booleanView(on bool, base lipgloss.Style) string {
	if on {
		return base.Foreground(onColor).Render("● on")
	}
	return base.Faint(true).Render("○ off")
}

// menuView renders a menu control's current item between arrows that show
// whether there are items before and after it.
func menuView(c Control, base lipgloss.Style) string {
	arrow := func(s string, ok bool) string {
		if !ok {
			return " "
		}
		return base.Faint(true).Render(s)
	}
	return arrow("‹", c.menuStep(-1) != c.Value) + " " +
		base.Bold(true).Render(c.MenuName(c.Value)) + " " +
		arrow("›", c.menuStep(1) != c.Value)
}

// detailLine describes c in more detail than its list line.
func (m Model) detailLine(c Control) string {
	var parts []string
	switch {
	case c.IsMenu():
		parts = append(parts,
			fmt.Sprintf("%s (%d)", c.MenuName(c.Value), c.Value),
			"default "+c.MenuName(c.Default))
		var items []string
		for _, item := range c.Menu {
			items = append(items, fmt.Sprintf("%d %s", item.Value, item.Name))
		}
		parts = append(parts, "items: "+strings.Join(items, ", "))
	case c.IsBoolean():
		parts = append(parts, c.formatValue(c.Value), "default "+c.formatValue(c.Default))
	default:
		parts = append(parts,
			strconv.Itoa(int(c.Value)),
			fmt.Sprintf("default %d", c.Default),
			fmt.Sprintf("range %d‥%d", c.Min, c.Max))
		if c.Step > 1 {
			parts = append(parts, fmt.Sprintf("step %d", c.Step))
		}
	}
	switch {
	case c.IsReadOnly():
		parts = append(parts, "read-only")
	case c.IsInactive():
		parts = append(parts, "inactive while an automatic mode is on")
	}
	return headerStyle.Render(c.Name) + faintStyle.Render(": ") +
		strings.Join(parts, faintStyle.Render(" · "))
}

// statusLine shows the status message on the left and the presets on the
// right.
func (m Model) statusLine(width int) string {
	style := statusInfoStyle
	if m.status.isErr {
		style = statusErrorStyle
	}
	text := style.Render(m.status.text)
	presets := m.presetsView()
	gap := max(1, width-lipgloss.Width(text)-lipgloss.Width(presets))
	return text + strings.Repeat(" ", gap) + presets
}

// presetsView lists the function keys that hold a preset.
func (m Model) presetsView() string {
	ids := slices.SortedFunc(maps.Keys(m.config.Presets), func(a, b string) int {
		ai, _ := strconv.Atoi(a)
		bi, _ := strconv.Atoi(b)
		return cmp.Or(cmp.Compare(ai, bi), cmp.Compare(a, b))
	})
	if len(ids) == 0 {
		return faintStyle.Render("no presets")
	}
	for i, id := range ids {
		ids[i] = "F" + id
	}
	return faintStyle.Render("presets ") + presetStyle.Render(strings.Join(ids, " "))
}

func (m Model) helpView(width int) string {
	h := m.help
	h.SetWidth(width)
	k := keys
	if h.ShowAll {
		k.Help.SetHelp("?", "less")
	}
	return h.View(k)
}

// padRight truncates or pads s with spaces to width cells.
func padRight(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

// padLeft truncates or pads s with leading spaces to width cells.
func padLeft(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	return strings.Repeat(" ", width-ansi.StringWidth(s)) + s
}
