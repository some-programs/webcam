package main

import (
	_ "embed"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/evertras/bubble-table/table"
)

//go:generate go run github.com/dmarkham/enumer@v1.5.8 -type=ViewType -trimprefix ViewType
type ViewType int

const (
	ViewTypeMain ViewType = iota
	ViewTypeHelp
)

type View interface {
	Update(m Model, msg tea.Msg) (tea.Model, tea.Cmd)
	View(m Model) string
}

func NewModel(cam *Webcam, configFile ConfigFile) Model {

	barWidth := 50
	controls := cam.getControls()
	var rows []table.Row
	for _, c := range controls {
		rows = append(rows, c.Row(barWidth))
	}

	helpView, err := newHelpView()
	if err != nil {
		log.Fatal(err)
	}

	return Model{
		table: table.New([]table.Column{
			table.NewColumn("name", "Name", 30).WithFiltered(true),
			table.NewColumn("bar", "", barWidth).WithStyle(lipgloss.NewStyle().Align(lipgloss.Center)),
			table.NewColumn("value", "Value", 7).WithStyle(lipgloss.NewStyle().Align(lipgloss.Left)),
			table.NewColumn("min", "Min", 6),
			table.NewColumn("max", "Max", 6),
		}).WithRows(rows).
			HighlightStyle(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(5))).
			Filtered(true).
			// WithPageSize(10).
			// BorderRounded().
			// SelectableRows(true).
			Focused(true),
		cam:         cam,
		current:     controls[0],
		barWidth:    barWidth,
		help:        help.New(),
		config:      configFile,
		currentView: ViewTypeMain,
		mainView:    &MainView{},
		helpView:    helpView,
	}
}

type Model struct {
	table             table.Model
	current           Control
	cam               *Webcam
	lastSelectedEvent table.UserEventRowSelectToggled
	lastError         string
	barWidth          int
	help              help.Model
	config            ConfigFile
	quitting          bool
	mainView          *MainView
	helpView          *HelpView
	currentView       ViewType
}

func (m Model) Init() tea.Cmd {
	return doTick()
}

func (m Model) GetCurrentView() View {
	switch m.currentView {
	case ViewTypeHelp:
		return m.helpView
	default:
		return m.mainView
	}
}

func (m Model) View() string {
	return m.GetCurrentView().View(m)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.String()
		switch k {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "?":
			if m.currentView == ViewTypeMain {
				m.currentView = ViewTypeHelp
			} else {
				m.currentView = ViewTypeMain
			}

		}

	}
	return m.GetCurrentView().Update(m, msg)
}

type TickMsg time.Time

func doTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// MainView .
type MainView struct{}

func (v MainView) View(m Model) string {
	view := lipgloss.JoinVertical(
		lipgloss.Left,
		m.table.View(),
		m.lastError,
	)

	return lipgloss.NewStyle().MarginLeft(1).Render(view)

}

func (v MainView) Update(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	m.table, cmd = m.table.Update(msg)
	cmds = append(cmds, cmd)

	for _, e := range m.table.GetLastUpdateUserEvents() {
		switch e := e.(type) {
		case table.UserEventHighlightedIndexChanged:
			selected := m.table.HighlightedRow().Data["control"].(Control)
			cmds = append(cmds, func() tea.Msg {
				return selected
			})
		case table.UserEventRowSelectToggled:
			m.lastSelectedEvent = e
		}
	}

	var updateControls bool
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "u":
			updateControls = true

		case "`", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			var v int64
			var err error
			switch {
			case key == "`":
				v = 0
			case key == "0":
				v = 10
			default:
				v, err = strconv.ParseInt(key, 10, 32)
			}
			if err == nil {
				if m.current.IsMenu() || m.current.IsBoolean() {
					if err := m.cam.webcam.SetControl(m.current.ID, int32(v)); err != nil {
						m.lastError = err.Error()
					} else {
						m.lastError = ""
					}

				} else {
					percent := float64(v) / 10
					if err := m.cam.webcam.SetControl(m.current.ID, controlValue(m.current, percent)); err != nil {
						m.lastError = err.Error()
					} else {
						m.lastError = ""
					}
				}
				updateControls = true
			}

		case "left", "h":
			if err := m.cam.webcam.SetControl(m.current.ID, m.current.GetValueDecreasePercent()); err != nil {
				m.lastError = err.Error()
			} else {
				m.lastError = ""
			}
			updateControls = true

		case "right", "l":
			if err := m.cam.webcam.SetControl(m.current.ID, m.current.GetValueIncreasePercent()); err != nil {
				m.lastError = err.Error()
			} else {
				m.lastError = ""
			}
			updateControls = true

		case "y":
			if err := m.cam.webcam.SetControl(m.current.ID, m.current.GetValueDecreseStep()); err != nil {
				m.lastError = err.Error()
			} else {
				m.lastError = ""
			}
			updateControls = true

		case "o":
			if err := m.cam.webcam.SetControl(m.current.ID, m.current.GetValueIncreaseStep()); err != nil {
				m.lastError = err.Error()
			} else {
				m.lastError = ""
			}
			updateControls = true

		case "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12":
			presetID := strings.TrimPrefix(key, "f")

			preset, ok := m.config.Presets[presetID]
			if ok {
				err := applyPreset(m.cam, preset)
				if err != nil {
					m.lastError = err.Error()
				}
				updateControls = true
			}

		case "alt+f1", "alt+f2", "alt+f3", "alt+f4", "alt+f5", "alt+f6",
			"alt+f7", "alt+f8", "alt+f9", "alt+f10", "alt+f11", "alt+f12":
			presetID := strings.TrimPrefix(key, "alt+f")
			preset, err := newPreset(m.cam)
			if err != nil {
				m.lastError = err.Error()
			}
			m.config.Presets[presetID] = preset

		}
	case Control:
		m.current = msg
		m.lastError = ""
	case TickMsg:
		updateControls = true
		cmds = append(cmds, doTick())

	}

	if updateControls {
		controls := m.cam.getControls()
		var rows []table.Row
		for _, c := range controls {
			if c.ID == m.current.ID {
				m.current = c
			}
			rows = append(rows, c.Row(m.barWidth))
		}
		m.table = m.table.WithRows(rows)
	}

	if m.lastError != "" {
		m.table = m.table.HighlightStyle(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(1)))

	} else {
		m.table = m.table.HighlightStyle(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(5)))
	}

	return m, tea.Batch(cmds...)
}

//go:embed help.md
var helpText string

// MainView .
type HelpView struct {
	viewport viewport.Model
}

func (h HelpView) Update(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (v HelpView) View(m Model) string {
	return v.viewport.View()
}

func newHelpView() (*HelpView, error) {
	const width = 78

	vp := viewport.New(width, 20)
	vp.Style = lipgloss.NewStyle()

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	str, err := renderer.Render(helpText)
	if err != nil {
		return nil, err
	}

	vp.SetContent(str)

	return &HelpView{
		viewport: vp,
	}, nil

}
