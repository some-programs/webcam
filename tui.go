package main

import (
	"fmt"
	"math"
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
		cam:     cam,
		help:    help.New(),
		config:  configFile,
		presets: configFile.CameraPresets(cam.Card),
		filter:  filter,
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
	// dragging is set while the mouse drags the selected control's bar.
	dragging bool
	filter   textinput.Model
	// width and height are the terminal size, or 0 before it is known.
	width, height int
	help          help.Model
	config        ConfigFile
	// presets are the presets of cam, part of config.
	presets map[string]ConfigPreset
	status  status
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

	case tea.MouseClickMsg:
		return m.mouseClick(msg.Mouse())

	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		if !m.dragging || mouse.Button != tea.MouseLeft {
			return m, nil
		}
		c, ok := m.current()
		if !ok {
			return m, nil
		}
		cmd := m.report(m.cam.SetControl(c.ID, m.barValue(c, mouse.X)), "")
		m.refreshControls()
		return m, cmd

	case tea.MouseReleaseMsg:
		m.dragging = false
		return m, nil

	case tea.MouseWheelMsg:
		return m.mouseWheel(msg.Mouse())

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
		name := strings.ToUpper(msg.String())
		preset, ok := m.presets[name]
		if !ok {
			return m.setStatus("no preset saved on "+name, true), true
		}
		return m.report(applyPreset(m.cam, preset), "loaded preset "+name), true
	case key.Matches(msg, keys.SavePreset):
		name := strings.ToUpper(strings.TrimPrefix(msg.String(), "alt+"))
		preset, err := newPreset(m.cam)
		if err != nil {
			return m.report(err, ""), true
		}
		m.presets[name] = preset
		if err := SaveConfigFile(m.config); err != nil {
			return m.report(err, ""), true
		}
		return m.report(nil, "saved preset "+name), true
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

// controlAt returns the control on screen row y, if any.
func (m Model) controlAt(y int) (Control, bool) {
	l := m.layout()
	i := y - l.listTop
	if i < 0 || (l.listHeight > 0 && i >= l.listHeight) {
		return Control{}, false
	}
	lines := m.listLines()
	i += m.offset
	if i >= len(lines) || !lines[i].isControl {
		return Control{}, false
	}
	return lines[i].control, true
}

// onBar reports whether screen column x is within the bars.
func (m Model) onBar(x int) bool {
	l := m.layout()
	return x >= l.barX && x < l.barX+l.barWidth
}

// barValue returns the value of integer control c that screen column x on
// its bar stands for, rounded to the control's step.
func (m Model) barValue(c Control, x int) int32 {
	l := m.layout()
	percent := 0.0
	if l.barWidth > 1 {
		percent = float64(x-l.barX) / float64(l.barWidth-1)
	}
	percent = min(1, max(0, percent))
	span := float64(c.Max) - float64(c.Min)
	step := float64(max(1, c.Step))
	value := float64(c.Min) + math.Round(percent*span/step)*step
	return int32(min(float64(c.Max), value))
}

// mouseClick selects the clicked control and, if the click is on its value,
// changes it: a click on a bar sets the value there and starts a drag, a
// click on a boolean toggles it, and a click on a menu steps to the previous
// item on the left arrow or the next item elsewhere.
func (m Model) mouseClick(mouse tea.Mouse) (Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	c, ok := m.controlAt(mouse.Y)
	if !ok {
		return m, nil
	}
	m.selected = c.ID
	if !m.onBar(mouse.X) {
		return m, nil
	}

	var value int32
	switch {
	case c.IsBoolean():
		value = c.ToggleBoolean()
	case c.IsMenu():
		value = c.menuStep(1)
		if mouse.X < m.layout().barX+2 {
			value = c.menuStep(-1)
		}
	default:
		value = m.barValue(c, mouse.X)
		m.dragging = true
	}
	cmd := m.report(m.cam.SetControl(c.ID, value), "")
	m.refreshControls()
	return m, cmd
}

// mouseWheel adjusts the control under the pointer, or moves the cursor if
// there is none.
func (m Model) mouseWheel(mouse tea.Mouse) (Model, tea.Cmd) {
	up := mouse.Button == tea.MouseWheelUp
	if !up && mouse.Button != tea.MouseWheelDown {
		return m, nil
	}
	c, ok := m.controlAt(mouse.Y)
	if !ok {
		if up {
			m.move(-1)
		} else {
			m.move(1)
		}
		return m, nil
	}
	m.selected = c.ID
	value := c.GetValueDecreasePercent()
	if up {
		value = c.GetValueIncreasePercent()
	}
	cmd := m.report(m.cam.SetControl(c.ID, value), "")
	m.refreshControls()
	return m, cmd
}
