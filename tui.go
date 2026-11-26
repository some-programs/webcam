package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/evertras/bubble-table/table"
)

// keyMap holds the main view's key bindings. Moving the cursor and filtering
// are handled by the table; their bindings here are only for the help.
type keyMap struct {
	Move       key.Binding
	Filter     key.Binding
	Adjust     key.Binding
	Increase   key.Binding
	Decrease   key.Binding
	StepUp     key.Binding
	StepDown   key.Binding
	QuickSet   key.Binding
	Reset      key.Binding
	ResetAll   key.Binding
	LoadPreset key.Binding
	SavePreset key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func fKeys(prefix string) []string {
	var keys []string
	for i := 1; i <= 12; i++ {
		keys = append(keys, fmt.Sprintf("%sf%d", prefix, i))
	}
	return keys
}

var keys = keyMap{
	Move:       key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "select")),
	Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Adjust:     key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←/→", "adjust")),
	Increase:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "increase")),
	Decrease:   key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "decrease")),
	StepUp:     key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "step up")),
	StepDown:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "step down")),
	QuickSet:   key.NewBinding(key.WithKeys("`", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0"), key.WithHelp("`…0", "set 0-100% / menu item")),
	Reset:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset to default")),
	ResetAll:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reset all")),
	LoadPreset: key.NewBinding(key.WithKeys(fKeys("")...), key.WithHelp("F1-F12", "load preset")),
	SavePreset: key.NewBinding(key.WithKeys(fKeys("alt+")...), key.WithHelp("alt+F1-F12", "save preset")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more")),
	Quit:       key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Adjust, k.StepDown, k.StepUp, k.Reset, k.Filter, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Move, k.Filter, k.Help, k.Quit},
		{k.Increase, k.Decrease, k.StepUp, k.StepDown, k.QuickSet},
		{k.Reset, k.ResetAll, k.LoadPreset, k.SavePreset},
	}
}

// tableKeyMap is the table's default key map without the bindings that
// clash with ours.
func tableKeyMap() table.KeyMap {
	km := table.DefaultKeyMap()
	km.PageDown.SetKeys("pgdown")
	km.PageUp.SetKeys("pgup")
	km.RowSelectToggle.Unbind()
	return km
}

const (
	statusInfoDuration  = 3 * time.Second
	statusErrorDuration = 6 * time.Second
)

var (
	headerStyle      = lipgloss.NewStyle().Bold(true)
	faintStyle       = lipgloss.NewStyle().Faint(true)
	statusInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	statusErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true)
	presetStyle      = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5))
	highlightStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(5))
	errorHighlight   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(1))
)

func NewModel(cam *Webcam, configFile ConfigFile) Model {
	barWidth := 40
	controls := cam.getControls()
	var rows []table.Row
	for _, c := range controls {
		rows = append(rows, c.Row(barWidth))
	}

	return Model{
		table: table.New([]table.Column{
			table.NewColumn("name", "Name", 30).WithFiltered(true),
			table.NewColumn("bar", "", barWidth).WithStyle(lipgloss.NewStyle().Align(lipgloss.Center)),
			table.NewColumn("value", "Value", 7).WithStyle(lipgloss.NewStyle().Align(lipgloss.Left)),
			table.NewColumn("default", "Default", 7),
			table.NewColumn("min", "Min", 6),
			table.NewColumn("max", "Max", 6),
			table.NewColumn("state", "", 8),
		}).WithRows(rows).
			WithKeyMap(tableKeyMap()).
			HighlightStyle(highlightStyle).
			Filtered(true).
			Focused(true),
		cam:      cam,
		current:  controls[0],
		barWidth: barWidth,
		help:     help.New(),
		config:   configFile,
	}
}

type Model struct {
	table    table.Model
	current  Control
	cam      *Webcam
	barWidth int
	help     help.Model
	config   ConfigFile
	status   status
}

// status is a message shown below the table until it expires.
type status struct {
	text  string
	isErr bool
	// id tells apart messages so that a timer only clears its own.
	id int
}

// clearStatusMsg clears the status if it is still the one with id.
type clearStatusMsg struct{ id int }

// setStatus shows text below the table for a while.
func (m *Model) setStatus(text string, isErr bool) tea.Cmd {
	id := m.status.id + 1
	m.status = status{text: text, isErr: isErr, id: id}
	d := statusInfoDuration
	if isErr {
		d = statusErrorDuration
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return clearStatusMsg{id: id} })
}

// report shows err, or the info message if err is nil and info is not empty.
// If both are empty it clears an error left by an earlier action.
func (m *Model) report(err error, info string) tea.Cmd {
	switch {
	case err != nil:
		return m.setStatus(err.Error(), true)
	case info != "":
		return m.setStatus(info, false)
	case m.status.isErr:
		m.status = status{id: m.status.id}
	}
	return nil
}

func (m Model) Init() tea.Cmd {
	return doTick()
}

type TickMsg time.Time

func doTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m Model) View() tea.View {
	header := headerStyle.Render(m.cam.Card) +
		faintStyle.Render(fmt.Sprintf(" · %s · %s", m.cam.Path, m.cam.Driver))
	tableView := m.table.View()
	width := lipgloss.Width(tableView)

	var statusText string
	switch {
	case m.status.isErr:
		statusText = statusErrorStyle.Render(m.status.text)
	default:
		statusText = statusInfoStyle.Render(m.status.text)
	}
	presets := m.presetsView()
	gap := max(1, width-lipgloss.Width(statusText)-lipgloss.Width(presets))
	statusLine := statusText + strings.Repeat(" ", gap) + presets

	m.help.SetWidth(width)
	k := keys
	if m.help.ShowAll {
		k.Help.SetHelp("?", "less")
	}
	view := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		tableView,
		statusLine,
		m.help.View(k),
	)
	return tea.NewView(lipgloss.NewStyle().MarginLeft(1).Render(view))
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit
		case m.table.GetIsFilterInputFocused():
			// Everything typed goes to the filter.
			return m.updateTable(msg)
		case msg.String() == "esc" && m.table.GetIsFilterActive():
			// Let the table clear the filter.
			return m.updateTable(msg)
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		}
		if cmd, handled := m.handleKey(msg); handled {
			cmds = append(cmds, cmd)
			m.refreshControls()
			m.updateHighlightStyle()
			return m, tea.Batch(cmds...)
		}
	}

	switch msg := msg.(type) {
	case Control:
		m.current = msg
		return m, nil
	case clearStatusMsg:
		if msg.id == m.status.id {
			m.status = status{id: m.status.id}
			m.updateHighlightStyle()
		}
		return m, nil
	case TickMsg:
		m.refreshControls()
		return m, doTick()
	}

	return m.updateTable(msg)
}

// updateTable passes msg to the table and follows the cursor.
func (m Model) updateTable(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	cmds := []tea.Cmd{cmd}
	for _, e := range m.table.GetLastUpdateUserEvents() {
		if _, ok := e.(table.UserEventHighlightedIndexChanged); ok {
			selected := m.table.HighlightedRow().Data["control"].(Control)
			cmds = append(cmds, func() tea.Msg { return selected })
		}
	}
	return m, tea.Batch(cmds...)
}

// handleKey runs the action bound to msg, if any.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	c := m.current
	switch {
	case key.Matches(msg, keys.Increase):
		return m.report(m.cam.SetControl(c.ID, c.GetValueIncreasePercent()), ""), true
	case key.Matches(msg, keys.Decrease):
		return m.report(m.cam.SetControl(c.ID, c.GetValueDecreasePercent()), ""), true
	case key.Matches(msg, keys.StepUp):
		return m.report(m.cam.SetControl(c.ID, c.GetValueIncreaseStep()), ""), true
	case key.Matches(msg, keys.StepDown):
		return m.report(m.cam.SetControl(c.ID, c.GetValueDecreseStep()), ""), true
	case key.Matches(msg, keys.QuickSet):
		return m.report(m.quickSet(msg.String()), ""), true
	case key.Matches(msg, keys.Reset):
		err := m.cam.SetControl(c.ID, c.Default)
		return m.report(err, fmt.Sprintf("%s reset to %s", c.Name, c.formatValue(c.Default))), true
	case key.Matches(msg, keys.ResetAll):
		err := applyPreset(m.cam, defaultPreset(m.cam))
		return m.report(err, "all controls reset to defaults"), true
	case key.Matches(msg, keys.LoadPreset):
		id := strings.TrimPrefix(msg.String(), "f")
		preset, ok := m.config.Presets[id]
		if !ok {
			return m.setStatus(fmt.Sprintf("no preset saved on F%s", id), true), true
		}
		return m.report(applyPreset(m.cam, preset), "loaded preset F"+id), true
	case key.Matches(msg, keys.SavePreset):
		id := strings.TrimPrefix(msg.String(), "alt+f")
		preset, err := newPreset(m.cam)
		if err != nil {
			return m.report(err, ""), true
		}
		m.config.Presets[id] = preset
		if err := SaveConfigFile(m.config); err != nil {
			return m.report(err, ""), true
		}
		return m.report(nil, "saved preset F"+id), true
	}
	return nil, false
}

// quickSet sets the current control from a digit key: "`" is 0%, "1" 10%
// and so on up to "0" for 100%. For menus it picks the item at that index,
// since menu values can have gaps; for booleans "`" is off and "1" on.
func (m *Model) quickSet(k string) error {
	var n int
	switch k {
	case "`":
		n = 0
	case "0":
		n = 10
	default:
		n, _ = strconv.Atoi(k)
	}
	c := m.current
	switch {
	case c.IsMenu():
		value, ok := c.MenuItemValue(n)
		if !ok {
			return fmt.Errorf("%s has no item %d", c.Name, n)
		}
		return m.cam.SetControl(c.ID, value)
	case c.IsBoolean():
		return m.cam.SetControl(c.ID, int32(n))
	default:
		return m.cam.SetControl(c.ID, controlValue(c, float64(n)/10))
	}
}

// refreshControls reads all controls from the camera.
func (m *Model) refreshControls() {
	var rows []table.Row
	for _, c := range m.cam.getControls() {
		if c.ID == m.current.ID {
			m.current = c
		}
		rows = append(rows, c.Row(m.barWidth))
	}
	m.table = m.table.WithRows(rows)
}

// updateHighlightStyle colors the cursor red while an error is shown.
func (m *Model) updateHighlightStyle() {
	if m.status.isErr {
		m.table = m.table.HighlightStyle(errorHighlight)
	} else {
		m.table = m.table.HighlightStyle(highlightStyle)
	}
}
