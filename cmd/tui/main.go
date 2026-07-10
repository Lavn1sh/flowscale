package main

import (
	"fmt"
	"os"

	"flowscale/internal/client"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	baseStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
)

type viewState int

const (
	viewExecutions viewState = iota
	viewWorkflows
	viewDLQ
	viewDetail
)

type model struct {
	client      *client.Client
	state       viewState
	table       table.Model
	execs       []table.Row
	detailText  string
	errorMsg    string
	loading     bool
}

func initialModel(apiClient *client.Client) model {
	columns := []table.Column{
		{Title: "ID", Width: 38},
		{Title: "Workflow ID", Width: 38},
		{Title: "Status", Width: 15},
		{Title: "Current Activity", Width: 20},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(15),
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

	return model{
		client: apiClient,
		state:  viewExecutions,
		table:  t,
	}
}

type loadedMsg struct {
	state   viewState
	rows    []table.Row
	details string
	err     error
}

func (m model) loadData(state viewState, id string) tea.Cmd {
	return func() tea.Msg {
		switch state {
		case viewExecutions:
			execs, err := m.client.ListExecutions()
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(execs))
			for i, ex := range execs {
				rows[i] = []string{ex.ID, ex.WorkflowID, string(ex.Status), ex.CurrentActivity}
			}
			return loadedMsg{state: state, rows: rows}

		case viewWorkflows:
			wfs, err := m.client.ListWorkflows()
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(wfs))
			for i, w := range wfs {
				rows[i] = []string{w.ID, w.Name, fmt.Sprintf("%d activities", len(w.Activities)), ""}
			}
			return loadedMsg{state: state, rows: rows}

		case viewDLQ:
			dlq, err := m.client.ListDLQ()
			if err != nil {
				return loadedMsg{err: err}
			}
			rows := make([]table.Row, len(dlq))
			for i, act := range dlq {
				rows[i] = []string{act.ID, act.ExecutionID, act.ActivityName, fmt.Sprintf("Attempt %d", act.Attempt)}
			}
			return loadedMsg{state: state, rows: rows}

		case viewDetail:
			events, err := m.client.GetExecutionEvents(id)
			if err != nil {
				return loadedMsg{err: err}
			}
			var details string
			for _, ev := range events {
				details += fmt.Sprintf("[%s] %s\n%s\n\n", ev.Timestamp.Format("15:04:05"), ev.EventType, string(ev.Payload))
			}
			if details == "" {
				details = "No events found."
			}
			return loadedMsg{state: state, details: details}
		}
		return nil
	}
}

func (m model) Init() tea.Cmd {
	return m.loadData(m.state, "")
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.state = viewExecutions
			m.table.SetColumns([]table.Column{
				{Title: "ID", Width: 38},
				{Title: "Workflow ID", Width: 38},
				{Title: "Status", Width: 15},
				{Title: "Current Activity", Width: 20},
			})
			return m, m.loadData(m.state, "")
		case "2":
			m.state = viewWorkflows
			m.table.SetColumns([]table.Column{
				{Title: "ID", Width: 38},
				{Title: "Name", Width: 30},
				{Title: "Activities", Width: 20},
				{Title: "", Width: 1},
			})
			return m, m.loadData(m.state, "")
		case "3":
			m.state = viewDLQ
			m.table.SetColumns([]table.Column{
				{Title: "ID", Width: 38},
				{Title: "Execution ID", Width: 38},
				{Title: "Activity", Width: 20},
				{Title: "Attempt", Width: 10},
			})
			return m, m.loadData(m.state, "")
		case "r":
			if m.state == viewExecutions {
				return m, m.loadData(m.state, "")
			} else if m.state == viewDLQ {
				row := m.table.SelectedRow()
				if row != nil {
					_ = m.client.RetryDLQ(row[0])
					return m, m.loadData(m.state, "")
				}
			}
		case "enter":
			if m.state == viewExecutions {
				row := m.table.SelectedRow()
				if row != nil {
					m.state = viewDetail
					return m, m.loadData(m.state, row[0])
				}
			} else if m.state == viewWorkflows {
				row := m.table.SelectedRow()
				if row != nil {
					m.client.StartWorkflow(row[0])
					// Hop back to executions
					m.state = viewExecutions
					m.table.SetColumns([]table.Column{
						{Title: "ID", Width: 38},
						{Title: "Workflow ID", Width: 38},
						{Title: "Status", Width: 15},
						{Title: "Current Activity", Width: 20},
					})
					return m, m.loadData(m.state, "")
				}
			}
		case "esc":
			if m.state == viewDetail {
				m.state = viewExecutions
				return m, m.loadData(m.state, "")
			}
		}

	case loadedMsg:
		if msg.err != nil {
			m.errorMsg = msg.err.Error()
		} else {
			m.errorMsg = ""
			if msg.state == viewDetail {
				m.detailText = msg.details
			} else {
				m.table.SetRows(msg.rows)
			}
		}
	}

	if m.state != viewDetail {
		m.table, cmd = m.table.Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	tabs := "Tabs: [1] Executions  [2] Workflows  [3] DLQ  |  [q] Quit"
	if m.state == viewExecutions {
		tabs = "Tabs: [*1*] Executions  [2] Workflows  [3] DLQ  |  [q] Quit"
	} else if m.state == viewWorkflows {
		tabs = "Tabs: [1] Executions  [*2*] Workflows  [3] DLQ  |  [q] Quit"
	} else if m.state == viewDLQ {
		tabs = "Tabs: [1] Executions  [2] Workflows  [*3*] DLQ  |  [q] Quit"
	}

	body := ""
	if m.errorMsg != "" {
		body = "Error: " + m.errorMsg
	} else if m.state == viewDetail {
		body = m.detailText
		tabs += "  |  [esc] Back"
	} else {
		body = baseStyle.Render(m.table.View())
		if m.state == viewExecutions {
			body += "\n" + helpStyle.Render("enter: view events  r: refresh")
		} else if m.state == viewWorkflows {
			body += "\n" + helpStyle.Render("enter: start workflow")
		} else if m.state == viewDLQ {
			body += "\n" + helpStyle.Render("r: retry activity")
		}
	}

	return fmt.Sprintf("\n%s\n\n%s\n", tabs, body)
}

func main() {
	apiClient := client.NewClient("http://localhost:8080")
	m := initialModel(apiClient)

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
