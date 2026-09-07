package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Clock-specific messages
type (
	tickMsg time.Time
	startMsg struct{}
	pauseMsg struct{}
	resetMsg struct{}
)

type clockModel struct {
	hoursInput    textinput.Model
	minutesInput  textinput.Model
	secondsInput  textinput.Model
	countdown     time.Duration
	running       bool
	focusIndex    int
	width         int
	height        int
	lastError     error
	beep          bool // To indicate countdown finished
	initialCountdown time.Duration
}

func initialClockModel(width, height int) clockModel {
	hours := textinput.New()
	hours.Placeholder = "00"
	hours.CharLimit = 2
	hours.Width = 4
	hours.Prompt = "H: "
	hours.Validate = func(s string) error {
		if s == "" {
			return nil
		}
		if _, err := strconv.Atoi(s); err != nil {
			return fmt.Errorf("not a number")
		}
		return nil
	}

	minutes := textinput.New()
	minutes.Placeholder = "00"
	minutes.CharLimit = 2
	minutes.Width = 4
	minutes.Prompt = "M: "
	minutes.Validate = func(s string) error {
		if s == "" {
			return nil
		}
		val, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("not a number")
		}
		if val < 0 || val > 59 {
			return fmt.Errorf("0-59")
		}
		return nil
	}

	seconds := textinput.New()
	seconds.Placeholder = "00"
	seconds.CharLimit = 2
	seconds.Width = 4
	seconds.Prompt = "S: "
	seconds.Validate = func(s string) error {
		if s == "" {
			return nil
		}
		val, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("not a number")
		}
		if val < 0 || val > 59 {
			return fmt.Errorf("0-59")
		}
		return nil
	}

	hours.Focus()

	return clockModel{
		hoursInput:    hours,
		minutesInput:  minutes,
		secondsInput:  seconds,
		countdown:     0,
		running:       false,
		focusIndex:    0,
		width:         width,
		height:        height,
		beep:          false,
		initialCountdown: 0,
	}
}

func (m clockModel) Init() tea.Cmd {
	return m.hoursInput.Focus()
}

func (m clockModel) Update(msg tea.Msg) (clockModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.beep && msg.String() == "enter" {
			// Acknowledge beep and reset
			m.beep = false
			m.countdown = 0
			m.hoursInput.SetValue("")
			m.minutesInput.SetValue("")
			m.secondsInput.SetValue("")
			m.initialCountdown = 0
			return m, nil
		}

		switch msg.String() {
		case "tab", "shift+tab", "right", "left":
			s := msg.String()

			// Did a field lose focus?
			if s == "tab" || s == "right" {
				m.focusIndex++
			} else {
				m.focusIndex--
			}

			if m.focusIndex > 2 {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = 2
			}

			cmds = append(cmds, m.updateFocus())

		case "enter":
			if !m.running {
				// Try to start countdown
				hours, _ := strconv.Atoi(m.hoursInput.Value())
				minutes, _ := strconv.Atoi(m.minutesInput.Value())
				seconds, _ := strconv.Atoi(m.secondsInput.Value())

				totalDuration := time.Duration(hours)*time.Hour +
					time.Duration(minutes)*time.Minute +
					time.Duration(seconds)*time.Second

				if totalDuration > 0 {
					m.countdown = totalDuration
					m.initialCountdown = totalDuration
					m.running = true
					m.beep = false
					cmds = append(cmds, tickCmd())
					m.lastError = nil
				} else {
					m.lastError = fmt.Errorf("set a time > 0")
				}
			} else {
				// If running, "enter" can be used to acknowledge beep (handled above) or act as pause/resume
				cmds = append(cmds, pauseMsgCmd())
			}

		case " ": // Space to pause/resume
			if m.countdown > 0 {
				if m.running {
					cmds = append(cmds, pauseMsgCmd())
				} else {
					cmds = append(cmds, startMsgCmd())
				}
			} else {
				m.lastError = fmt.Errorf("set a time to start")
			}
		case "r": // r to reset
			cmds = append(cmds, resetMsgCmd())

		}

	case tickMsg:
		if m.running {
			m.countdown -= time.Second
			if m.countdown <= 0 {
				m.countdown = 0
				m.running = false
				m.beep = true
				// Potentially add a beep sound or visual alert here
				return m, nil
			}
			cmds = append(cmds, tickCmd())
		}
	case startMsg:
		if m.countdown > 0 && !m.running {
			m.running = true
			m.beep = false
			cmds = append(cmds, tickCmd())
		}
	case pauseMsg:
		m.running = false
	case resetMsg:
		m.countdown = m.initialCountdown
		m.running = false
		m.beep = false
		if m.initialCountdown == 0 {
			m.hoursInput.SetValue("")
			m.minutesInput.SetValue("")
			m.secondsInput.SetValue("")
		}
	}

	// Update individual text inputs
	oldHours := m.hoursInput.Value()
	oldMinutes := m.minutesInput.Value()
	oldSeconds := m.secondsInput.Value()

	m.hoursInput, cmd = m.hoursInput.Update(msg)
	cmds = append(cmds, cmd)
	m.minutesInput, cmd = m.minutesInput.Update(msg)
	cmds = append(cmds, cmd)
	m.secondsInput, cmd = m.secondsInput.Update(msg)
	cmds = append(cmds, cmd)

	// If countdown is running, don't allow editing inputs
	if m.running {
		if m.hoursInput.Value() != oldHours || m.minutesInput.Value() != oldMinutes || m.secondsInput.Value() != oldSeconds {
			m.lastError = fmt.Errorf("cannot edit while timer is running")
			// Revert input values
			m.hoursInput.SetValue(oldHours)
			m.minutesInput.SetValue(oldMinutes)
			m.secondsInput.SetValue(oldSeconds)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *clockModel) updateFocus() tea.Cmd {
	var cmds []tea.Cmd
	inputs := []*textinput.Model{&m.hoursInput, &m.minutesInput, &m.secondsInput}

	for i := 0; i < len(inputs); i++ {
		if i == m.focusIndex {
			cmds = append(cmds, (*inputs[i]).Focus())
		} else {
			inputs[i].Blur()
		}
	}
	return tea.Batch(cmds...)
}

func (m clockModel) View() string {
	inputStyle := lipgloss.NewStyle().Padding(0, 1)

	// Countdown display
	hours := int(m.countdown.Hours())
	minutes := int(m.countdown.Minutes()) % 60
	seconds := int(m.countdown.Seconds()) % 60

	countdownDisplay := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

	countdownTitle := "Countdown"
	if m.beep {
		countdownTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("TIME UP!")
		countdownDisplay = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render(countdownDisplay)
	} else if m.running {
		countdownTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("Running")
	} else if m.countdown > 0 {
		countdownTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("Paused")
	} else {
		countdownTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Set Time")
	}

	countdownView := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Render(countdownTitle),
		lipgloss.NewStyle().Bold(true).SetString(countdownDisplay).String(),
	)

	// Input fields
	inputsView := lipgloss.JoinHorizontal(lipgloss.Top,
		inputStyle.Render(m.hoursInput.View()),
		inputStyle.Render(m.minutesInput.View()),
		inputStyle.Render(m.secondsInput.View()),
	)

	// Buttons
	startBtn := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Render("Start (Enter)")
	pauseBtn := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Render("Pause (Space)")
	resetBtn := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Render("Reset (r)")

	var actionButtons string
	if m.beep {
		actionButtons = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("Press Enter to clear alert!")
	} else if m.running {
		actionButtons = lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(startBtn), // Grayed out
			lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(pauseBtn),
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(resetBtn),
		)
	} else if m.countdown > 0 {
		// Paused state
		actionButtons = lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(startBtn),
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(pauseBtn), // Grayed out
			lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(resetBtn),
		)
	} else {
		// Ready to start
		actionButtons = lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(startBtn),
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(pauseBtn),
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(resetBtn),
		)
	}


	// Error display
	errorView := ""
	if m.lastError != nil {
		errorView = errorStyle.Render("ERROR: " + m.lastError.Error())
	}

	// Main clock view layout
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center,
			errorView,
			inputsView,
			actionButtons,
			countdownView,
		),
	)
}

// Commands for the main Update loop to send to the clock
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func startMsgCmd() tea.Cmd {
	return func() tea.Msg {
		return startMsg{}
	}
}

func pauseMsgCmd() tea.Cmd {
	return func() tea.Msg {
		return pauseMsg{}
	}
}

func resetMsgCmd() tea.Cmd {
	return func() tea.Msg {
		return resetMsg{}
	}
}
