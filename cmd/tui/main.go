package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"flowscale/internal/client"
	"flowscale/internal/models"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	baseStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
	tabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1)
	activeTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Padding(0, 1).Bold(true)
)

type viewState int

const (
	viewExecutions viewState = iota
	viewWorkflows
	viewDLQ
	viewCompensation
	viewSchedules
	viewDetail
)

type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	Quit         key.Binding
	Cancel       key.Binding
	Retry        key.Binding
	Filter       key.Binding
	New          key.Binding
	Delete       key.Binding
	Enter        key.Binding
	Back         key.Binding
	ToggleDemo   key.Binding
	Generate     key.Binding
	Pause        key.Binding
	Resume       key.Binding
	BatchExecute key.Binding
}

var keys = keyMap{
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Cancel:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel")),
	Retry:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry")),
	Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	New:        key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
	Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	ToggleDemo: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "toggle demo API")),
	Generate:   key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "generate demos")),
	Pause:        key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause")),
	Resume:       key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "resume")),
	BatchExecute: key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "batch run (10x)")),
}

type stateKeyMap struct {
	keys  keyMap
	state viewState
}

func (s stateKeyMap) ShortHelp() []key.Binding {
	base := []key.Binding{s.keys.Up, s.keys.Down}
	switch s.state {
	case viewExecutions:
		base = append(base, s.keys.Enter, s.keys.Cancel, s.keys.Filter)
	case viewWorkflows:
		base = append(base, s.keys.Enter, s.keys.Generate, s.keys.BatchExecute)
	case viewDLQ, viewCompensation:
		base = append(base, s.keys.Enter, s.keys.Retry)
	case viewSchedules:
		base = append(base, s.keys.New, s.keys.Delete, s.keys.Pause, s.keys.Resume)
	case viewDetail:
		base = append(base, s.keys.Back)
	}
	base = append(base, s.keys.ToggleDemo, s.keys.Quit)
	return base
}
func (s stateKeyMap) FullHelp() [][]key.Binding {
	var rows [][]key.Binding
	keys := s.ShortHelp()
	chunkSize := 4
	for i := 0; i < len(keys); i += chunkSize {
		end := i + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		rows = append(rows, keys[i:end])
	}
	return rows
}

type model struct {
	client      *client.Client
	state       viewState
	table       table.Model
	viewport    viewport.Model
	filterInput textinput.Model
	help        help.Model
	
	// Filters
	filterStatus   string
	filterWorkflow string
	filterTime     string
	isFiltering    bool
	
	// Demo
	shipmentDown bool
	
	width  int
	height int
	
	err error
}

func initialModel(apiClient *client.Client) model {
	t := table.New(
		table.WithFocused(true),
		table.WithHeight(15),
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 38},
			{Title: "Workflow", Width: 38},
			{Title: "Status", Width: 15},
		}),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	ti := textinput.New()
	ti.Placeholder = "Status=X or Workflow=Y"
	ti.CharLimit = 50
	ti.Width = 40

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		PaddingRight(2)

	m := model{
		client:      apiClient,
		state:       viewExecutions,
		table:       t,
		viewport:    vp,
		filterInput: ti,
		help:        help.New(),
		filterTime:  "24h",
	}
	m.help.ShowAll = true
	m.setColumnsForState(viewExecutions)
	return m
}

type loadedMsg struct {
	state   viewState
	rows    []table.Row
	details string
	err     error
}

func (m *model) setColumnsForState(state viewState) {
	switch state {
	case viewExecutions:
		m.table.SetColumns([]table.Column{
			{Title: "ID", Width: 38},
			{Title: "Workflow", Width: 22},
			{Title: "Status", Width: 25},
		})
	case viewWorkflows:
		m.table.SetColumns([]table.Column{
			{Title: "ID", Width: 38},
			{Title: "Name", Width: 30},
			{Title: "Activities", Width: 20},
		})
	case viewDLQ:
		m.table.SetColumns([]table.Column{
			{Title: "ID", Width: 38},
			{Title: "Exec ID", Width: 38},
			{Title: "Activity", Width: 22},
			{Title: "Attempt", Width: 8},
		})
	case viewCompensation:
		m.table.SetColumns([]table.Column{
			{Title: "ID", Width: 38},
			{Title: "Workflow", Width: 22},
			{Title: "Activity", Width: 22},
			{Title: "Status", Width: 25},
		})
	case viewSchedules:
		m.table.SetColumns([]table.Column{
			{Title: "ID", Width: 38},
			{Title: "Workflow", Width: 22},
			{Title: "Status", Width: 25},
			{Title: "Next Run", Width: 15},
		})
	}
}

func (m model) loadData() tea.Cmd {
	state := m.state
	return func() tea.Msg {
		wfs, _ := m.client.ListWorkflows()
		wfMap := make(map[string]string)
		for _, w := range wfs {
			wfMap[w.ID] = w.Name
		}
		getWfName := func(id string) string {
			if name, ok := wfMap[id]; ok {
				return name
			}
			return id
		}

		colorStatus := func(st string) string {
			switch st {
			case "COMPLETED": return "✅ COMPLETED"
			case "ACTIVE": return "🟢 ACTIVE"
			case "FAILED": return "❌ FAILED"
			case "RUNNING": return "⏳ RUNNING"
			case "COMPENSATING": return "⚠️ COMPENSAT.."
			case "CANCELLED": return "🛑 CANCELLED"
			case "PAUSED": return "⏸️ PAUSED"
			}
			return st
		}

		switch state {
		case viewExecutions:
			execs, err := m.client.ListExecutions(m.filterStatus, m.filterWorkflow, m.filterTime)
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(execs))
			for i, ex := range execs {
				rows[i] = []string{ex.ID, getWfName(ex.WorkflowID), colorStatus(string(ex.Status))}
			}
			return loadedMsg{state: state, rows: rows}

		case viewWorkflows:
			wfs, err := m.client.ListWorkflows()
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(wfs))
			for i, w := range wfs {
				rows[i] = []string{w.ID, w.Name, fmt.Sprintf("%d acts", len(w.Activities))}
			}
			return loadedMsg{state: state, rows: rows}

		case viewDLQ:
			dlq, err := m.client.ListDLQ()
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(dlq))
			for i, act := range dlq {
				rows[i] = []string{act.ID, act.ExecutionID, act.ActivityName, fmt.Sprintf("%d", act.Attempt)}
			}
			return loadedMsg{state: state, rows: rows}

		case viewCompensation:
			// Filter executions to COMPENSATING and COMPENSATED
			execs, err := m.client.ListExecutions("COMPENSATING,COMPENSATED", "", "")
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(execs))
			for i, ex := range execs {
				rows[i] = []string{ex.ID, getWfName(ex.WorkflowID), colorStatus(string(ex.Status)), ex.CurrentActivity}
			}
			return loadedMsg{state: state, rows: rows}

		case viewSchedules:
			scheds, err := m.client.ListSchedules()
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(scheds))
			for i, s := range scheds {
				nr := "N/A"
				if !s.NextRunAt.IsZero() {
					nr = s.NextRunAt.Local().Format("15:04:05")
				}
				rows[i] = []string{s.ID, getWfName(s.WorkflowID), colorStatus(string(s.Status)), nr}
			}
			return loadedMsg{state: state, rows: rows}
		}
		return nil
	}
}

func (m model) loadDetail(id string) tea.Cmd {
	return func() tea.Msg {
		events, err := m.client.GetExecutionEvents(id)
		if err != nil {
			return loadedMsg{err: err}
		}
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Execution Events Timeline\n\n"))
		for _, ev := range events {
			ts := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(ev.Timestamp.Local().Format("15:04:05.000"))
			
			var evColor string
			switch string(ev.EventType) {
			case "WORKFLOW_STARTED", "ACTIVITY_STARTED":
				evColor = "33" // Blueish
			case "WORKFLOW_COMPLETED", "ACTIVITY_COMPLETED":
				evColor = "46" // Green
			case "WORKFLOW_FAILED", "ACTIVITY_FAILED":
				evColor = "196" // Red
			case "WORKFLOW_COMPENSATING", "COMPENSATION_STARTED", "COMPENSATION_COMPLETED", "COMPENSATION_FAILED":
				evColor = "214" // Orange/Yellow
			default:
				evColor = "36" // Default Cyan
			}
			evType := lipgloss.NewStyle().Foreground(lipgloss.Color(evColor)).Bold(true).Render(string(ev.EventType))
			
			b.WriteString(fmt.Sprintf("%s  %s\n", ts, evType))
			if len(ev.Payload) > 2 { // more than just "{}"
				b.WriteString(fmt.Sprintf("    %s\n", string(ev.Payload)))
			}
			b.WriteString("\n")
		}
		if len(events) == 0 {
			b.WriteString("No events found.")
		}
		return loadedMsg{state: viewDetail, details: b.String()}
	}
}

type shipmentStatusMsg struct {
	down bool
}

func (m model) loadShipmentStatus() tea.Cmd {
	return func() tea.Msg {
		down, err := m.client.GetShipmentStatus()
		if err != nil {
			return nil // ignore error for simplicity
		}
		return shipmentStatusMsg{down: down}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadData(), m.loadShipmentStatus())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 6

	case shipmentStatusMsg:
		m.shipmentDown = msg.down

	case loadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.state == viewDetail {
			m.viewport.SetContent(msg.details)
			m.viewport.GotoTop()
		} else {
			m.table.SetRows(msg.rows)
			m.table.SetCursor(0)
		}

	case tea.KeyMsg:
		if m.isFiltering {
			switch msg.String() {
			case "enter", "esc":
				m.isFiltering = false
				m.filterInput.Blur()
				// parse filter 
				val := m.filterInput.Value()
				if strings.HasPrefix(val, "status=") {
					m.filterStatus = strings.TrimPrefix(val, "status=")
					m.filterWorkflow = ""
				} else if strings.HasPrefix(val, "workflow=") {
					m.filterWorkflow = strings.TrimPrefix(val, "workflow=")
					m.filterStatus = ""
				} else {
					m.filterStatus = ""
					m.filterWorkflow = ""
				}
				cmds = append(cmds, m.loadData())
			default:
				m.filterInput, cmd = m.filterInput.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		if m.state == viewDetail {
			switch msg.String() {
			case "esc", "q":
				m.state = viewExecutions
				m.table.SetRows(nil)
				m.setColumnsForState(m.state)
				return m, m.loadData()
			default:
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "1", "2", "3", "4", "5":
			switch msg.String() {
			case "1": m.state = viewExecutions
			case "2": m.state = viewWorkflows
			case "3": m.state = viewDLQ
			case "4": m.state = viewCompensation
			case "5": m.state = viewSchedules
			}
			m.table.SetRows(nil)
			m.setColumnsForState(m.state)
			return m, m.loadData()
		case "/":
			if m.state == viewExecutions {
				m.isFiltering = true
				m.filterInput.Focus()
				return m, textinput.Blink
			}
		case "r":
			if row := m.table.SelectedRow(); row != nil {
				switch m.state {
				case viewDLQ:
					_ = m.client.RetryDLQ(row[0])
					return m, m.loadData()
				case viewCompensation:
					_ = m.client.RetryCompensation(row[0])
					return m, m.loadData()
				}
			}
		case "c":
			if m.state == viewExecutions {
				if row := m.table.SelectedRow(); row != nil {
					_ = m.client.CancelExecution(row[0])
					return m, m.loadData()
				}
			}
		case "d":
			if m.state == viewSchedules {
				if row := m.table.SelectedRow(); row != nil {
					_ = m.client.DeleteSchedule(row[0])
					return m, m.loadData()
				}
			}
		case "s":
			m.shipmentDown = !m.shipmentDown
			_ = m.client.SetShipmentStatus(m.shipmentDown)
			return m, nil
		case "g":
			if m.state == viewWorkflows {
				// Seed workflows via API
				req, _ := http.NewRequest(http.MethodPost, m.client.BaseURL+"/workflows/seed", nil)
				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					resp.Body.Close()
				}
				return m, m.loadData()
			}
		case "B":
			if m.state == viewWorkflows {
				if row := m.table.SelectedRow(); row != nil {
					// Batch execute 10 times
					for i := 0; i < 10; i++ {
						_, _ = m.client.StartWorkflow(row[0])
					}
					return m, m.loadData()
				}
			}
		case "p":
			if m.state == viewSchedules {
				if row := m.table.SelectedRow(); row != nil {
					_ = m.client.PauseSchedule(row[0])
					return m, m.loadData()
				}
			}
		case "m":
			if m.state == viewSchedules {
				if row := m.table.SelectedRow(); row != nil {
					_ = m.client.ResumeSchedule(row[0])
					return m, m.loadData()
				}
			}
		case "n":
			if m.state == viewSchedules {
				wfs, err := m.client.ListWorkflows()
				if err == nil {
					var targetID string
					for _, w := range wfs {
						if w.Name == "Data-Sync-Cron" {
							targetID = w.ID
							break
						}
					}
					if targetID != "" {
						_, _ = m.client.CreateSchedule(models.Schedule{
							WorkflowID: targetID,
							CronExpression: "* * * * *",
							ScheduleType: "recurring",
						})
						return m, m.loadData()
					}
				}
			}
		case "enter":
			if row := m.table.SelectedRow(); row != nil {
				switch m.state {
				case viewExecutions, viewCompensation:
					m.state = viewDetail
					m.viewport.SetContent("Loading details...")
					return m, m.loadDetail(row[0])
				case viewDLQ:
					m.state = viewDetail
					m.viewport.SetContent("Loading details...")
					return m, m.loadDetail(row[1])
				case viewWorkflows:
					_, _ = m.client.StartWorkflow(row[0])
					m.state = viewExecutions
					m.table.SetRows(nil)
					m.setColumnsForState(m.state)
					return m, m.loadData()
				}
			}
		}

		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	var b strings.Builder
	
	// Tabs
	tabs := []string{"1: Executions", "2: Workflows", "3: DLQ", "4: Compensations", "5: Schedules"}
	for i, t := range tabs {
		if int(m.state) == i {
			b.WriteString(activeTabStyle.Render(t))
		} else {
			b.WriteString(tabStyle.Render(t))
		}
		if i < len(tabs)-1 {
			b.WriteString(" | ")
		}
	}
	
	// Add Demo Status
	demoStatus := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("Shipment API: UP")
	if m.shipmentDown {
		demoStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Shipment API: DOWN")
	}
	// Calculate padding to push to right (roughly width - tabs length - status length)
	b.WriteString(fmt.Sprintf("    [%s]\n\n", demoStatus))

	if m.err != nil {
		b.WriteString(fmt.Sprintf("Error: %v\n", m.err))
		return b.String()
	}

	if m.state == viewDetail {
		b.WriteString(m.viewport.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc/q: back to list • up/down: scroll"))
		return b.String()
	}

	if m.isFiltering {
		b.WriteString("Filter: ")
		b.WriteString(m.filterInput.View())
		b.WriteString("\n\n")
	}

	b.WriteString(baseStyle.Render(m.table.View()))
	b.WriteString("\n")
	b.WriteString(m.help.View(stateKeyMap{keys: keys, state: m.state}))
	return b.String()
}

func main() {
	baseURL := os.Getenv("API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	apiClient := client.NewClient(baseURL)
	if err := apiClient.Login("admin", "admin"); err != nil {
		fmt.Printf("Failed to login to API (make sure engine is running and admin exists): %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel(apiClient), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
