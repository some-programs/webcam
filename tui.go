package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// keyMap holds the main view's key bindings.
type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	Top        key.Binding
	Bottom     key.Binding
	Move       key.Binding // only for the help
	Filter     key.Binding
	Adjust     key.Binding // only for the help
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
	Up:         key.NewBinding(key.WithKeys("up", "k")),
	Down:       key.NewBinding(key.WithKeys("down", "j")),
	PageUp:     key.NewBinding(key.WithKeys("pgup")),
	PageDown:   key.NewBinding(key.WithKeys("pgdown")),
	Top:        key.NewBinding(key.WithKeys("home", "g")),
	Bottom:     key.NewBinding(key.WithKeys("end", "G")),
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

const (
	statusInfoDuration  = 3 * time.Second
	statusErrorDuration = 6 * time.Second
)

func NewModel(cam *Webcam, configFile ConfigFile) Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "filter controls"
	m := Model{
		cam:    cam,
		help:   help.New(),
		config: configFile,
		filter: filter,
	}
	m.refreshControls()
	if len(m.controls) > 0 {
		m.selected = m.controls[0].ID
	}
	return m
}

type Model struct {
	cam      *Webcam
	controls []Control
	// selected is the ID of the control under the cursor. If the filter
	// hides it, the first visible control is used instead.
	selected ControlID
	// offset is the first list line shown when the list does not fit.
	offset int
	filter textinput.Model
	// width and height are the terminal size, or 0 before it is known.
	width, height int
	help          help.Model
	config        ConfigFile
	status        status
}

// status is a message shown below the list until it expires.
type status struct {
	text  string
	isErr bool
	// id tells apart messages so that a timer only clears its own.
	id int
}

// clearStatusMsg clears the status if it is still the one with id.
type clearStatusMsg struct{ id int }

// setStatus shows text below the list for a while.
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

// visible returns the controls that match the filter.
func (m Model) visible() []Control {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		return m.controls
	}
	return slices.DeleteFunc(slices.Clone(m.controls), func(c Control) bool {
		return !strings.Contains(strings.ToLower(c.Name), q)
	})
}

// current returns the control under the cursor.
func (m Model) current() (Control, bool) {
	visible := m.visible()
	if i := m.currentIndex(visible); i >= 0 {
		return visible[i], true
	}
	return Control{}, false
}

// currentIndex returns the index of the control under the cursor in
// visible, or -1 if visible is empty.
func (m Model) currentIndex(visible []Control) int {
	if len(visible) == 0 {
		return -1
	}
	return max(0, slices.IndexFunc(visible, func(c Control) bool { return c.ID == m.selected }))
}

// move moves the cursor by delta controls, stopping at either end.
func (m *Model) move(delta int) {
	visible := m.visible()
	i := m.currentIndex(visible)
	if i < 0 {
		return
	}
	m.selected = visible[min(len(visible)-1, max(0, i+delta))].ID
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.update(msg)
	// Keep the cursor on a visible control when the filter hides it.
	if c, ok := m.current(); ok {
		m.selected = c.ID
	}
	m.scrollToCursor()
	return m, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.updateKey(msg)

	case clearStatusMsg:
		if msg.id == m.status.id {
			m.status = status{id: m.status.id}
		}
		return m, nil

	case TickMsg:
		m.refreshControls()
		return m, doTick()
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	return m, cmd
}

func (m Model) updateKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if m.filter.Focused() {
		switch msg.String() {
		case "enter":
			m.filter.Blur()
		case "esc":
			m.filter.Reset()
			m.filter.Blur()
		case "up":
			m.move(-1)
		case "down":
			m.move(1)
		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch {
	case msg.String() == "esc" && m.filter.Value() != "":
		m.filter.Reset()
		return m, nil
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	case key.Matches(msg, keys.Filter):
		return m, m.filter.Focus()
	case key.Matches(msg, keys.Up):
		m.move(-1)
		return m, nil
	case key.Matches(msg, keys.Down):
		m.move(1)
		return m, nil
	case key.Matches(msg, keys.PageUp):
		m.move(-max(1, m.layout().listHeight-1))
		return m, nil
	case key.Matches(msg, keys.PageDown):
		m.move(max(1, m.layout().listHeight-1))
		return m, nil
	case key.Matches(msg, keys.Top):
		m.move(-len(m.controls))
		return m, nil
	case key.Matches(msg, keys.Bottom):
		m.move(len(m.controls))
		return m, nil
	}

	cmd, handled := m.handleKey(msg)
	if handled {
		m.refreshControls()
	}
	return m, cmd
}

// handleKey runs the action bound to msg, if any.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	// Presets do not need a selected control.
	switch {
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

	c, ok := m.current()
	if !ok {
		return nil, false
	}
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
		return m.report(m.quickSet(c, msg.String()), ""), true
	case key.Matches(msg, keys.Reset):
		err := m.cam.SetControl(c.ID, c.Default)
		return m.report(err, fmt.Sprintf("%s reset to %s", c.Name, c.formatValue(c.Default))), true
	}
	return nil, false
}

// quickSet sets c from a digit key: "`" is 0%, "1" 10% and so on up to "0"
// for 100%. For menus it picks the item at that index, since menu values can
// have gaps; for booleans "`" is off and "1" on.
func (m *Model) quickSet(c Control, k string) error {
	var n int
	switch k {
	case "`":
		n = 0
	case "0":
		n = 10
	default:
		n, _ = strconv.Atoi(k)
	}
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
	m.controls = m.cam.getControls()
}

// scrollToCursor scrolls the list so that the cursor is shown.
func (m *Model) scrollToCursor() {
	l := m.layout()
	lines := m.listLines()
	cursor := slices.IndexFunc(lines, func(line listLine) bool {
		return line.isControl && line.control.ID == m.selected
	})
	if l.listHeight <= 0 || len(lines) <= l.listHeight {
		m.offset = 0
		return
	}
	m.offset = min(m.offset, len(lines)-l.listHeight)
	if cursor >= 0 {
		if cursor < m.offset {
			m.offset = cursor
			// Show the heading above the first control of a class.
			if cursor > 0 && !lines[cursor-1].isControl {
				m.offset--
			}
		}
		if cursor >= m.offset+l.listHeight {
			m.offset = cursor - l.listHeight + 1
		}
	}
	m.offset = max(0, m.offset)
}
