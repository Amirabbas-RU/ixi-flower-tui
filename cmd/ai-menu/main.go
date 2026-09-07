package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type menuItem struct {
	name    string
	command string
	color   lipgloss.Color
}

type model struct {
	items      []menuItem
	selected   int
	quitting   bool
	launchDir  string
	width      int
	height     int
}

var (
	menuItems = []menuItem{
		{name: "Qwen", command: "qwen", color: lipgloss.Color("240")},           // Dark Gray
		{name: "Gemini CLI", command: "gemini", color: lipgloss.Color("240")},   // Dark Gray
		{name: "ACLI (rovodev run)", command: "acli rovodev run", color: lipgloss.Color("240")}, // Dark Gray
		{name: "Droid", command: "droid", color: lipgloss.Color("240")},         // Dark Gray
		{name: "Codex", command: "codex", color: lipgloss.Color("240")},        // Dark Gray
		{name: "Kimi", command: "kimi", color: lipgloss.Color("240")},           // Dark Gray
		{name: "Kilocode", command: "kilocode", color: lipgloss.Color("240")},  // Dark Gray
		{name: "Quit", command: "quit", color: lipgloss.Color("240")},          // Dark Gray
	}

	bannerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Align(lipgloss.Center)

	titleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Align(lipgloss.Center).
		MarginTop(1).
		MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("16")).
		Background(lipgloss.Color("255")).
		Bold(true).
		PaddingLeft(2).
		PaddingRight(2)

	normalStyle = lipgloss.NewStyle().
		PaddingLeft(2)

	hintStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Align(lipgloss.Center).
		MarginTop(1)

	footerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Align(lipgloss.Center).
		MarginTop(1)
)

func initialModel() model {
	launchDir, _ := os.Getwd()
	return model{
		items:     menuItems,
		selected:  0,
		launchDir: launchDir,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "Q":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			m.selected--
			if m.selected < 0 {
				m.selected = len(m.items) - 1
			}

		case "down", "j":
			m.selected++
			if m.selected >= len(m.items) {
				m.selected = 0
			}

		case "enter", " ":
			selectedItem := m.items[m.selected]
			if selectedItem.command == "quit" {
				m.quitting = true
				return m, tea.Quit
			}
			return m, runAgent(selectedItem.name, selectedItem.command, m.launchDir)
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return footerStyle.Render("Goodbye! 👋\n")
	}

	// Calculate vertical centering
	banner := `
 █████╗ ██╗     █████╗  ██████╗ ███████╗███╗   ██╗████████╗███████╗
██╔══██╗██║    ██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝██╔════╝
███████║██║    ███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║   ███████╗
██╔══██║██║    ██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║   ╚════██║
██║  ██║██║    ██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║   ███████║
╚═╝  ╚═╝╚═╝    ╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚══════╝`

	s := "\n\n"
	s += bannerStyle.Width(m.width).Render(banner) + "\n"
	s += hintStyle.Width(m.width).Render(">> Use ↑↓ arrows and Enter to select <<") + "\n\n"
	s += titleStyle.Width(m.width).Render("★ AI AGENTS MENU ★") + "\n\n"

	// Build menu items with proper centering
	maxLen := 0
	
	// First pass: find the longest line
	for _, item := range m.items {
		lineLen := len("▶ " + item.name)
		if lineLen > maxLen {
			maxLen = lineLen
		}
	}
	
	// Calculate left margin to center the block
	leftMargin := (m.width - maxLen) / 2
	if leftMargin < 0 {
		leftMargin = 0
	}
	
	// Build the menu with consistent left margin
	menuContent := ""
	for i, item := range m.items {
		if i == m.selected {
			cursor := "▶ "
			itemStyle := selectedStyle
			line := itemStyle.Render(cursor + item.name)
			menuContent += lipgloss.NewStyle().PaddingLeft(leftMargin).Render(line) + "\n"
		} else {
			itemStyle := normalStyle.Foreground(item.color)
			line := itemStyle.Render("  " + item.name)
			menuContent += lipgloss.NewStyle().PaddingLeft(leftMargin).Render(line) + "\n"
		}
	}

	s += menuContent

	return s
}

func runAgent(name, command, launchDir string) tea.Cmd {
	return func() tea.Msg {
		// Clear screen and show execution info
		fmt.Print("\033[H\033[2J")
		
		// Set proxy environment variables
		os.Setenv("HTTP_PROXY", "http://127.0.0.1:10808")
		os.Setenv("HTTPS_PROXY", "http://127.0.0.1:10808")
		
		fmt.Printf("\033[2mProxy set: HTTP_PROXY & HTTPS_PROXY = http://127.0.0.1:10808\033[0m\n")
		fmt.Printf("\033[2mWorking directory: %s\033[0m\n", launchDir)
		fmt.Printf("\n\033[36mLaunching %s...\033[0m\n\n", name)

		// Change to launch directory
		os.Chdir(launchDir)

		// Execute the command
		cmd := exec.Command("sh", "-c", command)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()

		// Wait for user to press enter
		fmt.Printf("\n\033[33mPress Enter to return to menu...\033[0m\n")
		fmt.Scanln()

		// Restart the program to show menu again
		newCmd := exec.Command(os.Args[0])
		newCmd.Stdin = os.Stdin
		newCmd.Stdout = os.Stdout
		newCmd.Stderr = os.Stderr
		newCmd.Run()

		return tea.Quit()
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
