package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v2"
)

const (
	dockerComposePath    = "/home/ixi_flower/Documents/server/docker-compose.yml"
	noteAutosaveDebounce = 500 * time.Millisecond
	htopRefreshInterval  = 2 * time.Second
	hubSearchDebounce    = 600 * time.Millisecond
)

type dockerServiceModel struct {
	name   string
	status string
}

type dockerService struct {
	title, status string
	responseTime  string
	address       string
}

func (d dockerService) Title() string { return d.title }
func (d dockerService) Description() string {
	if d.status == "online" {
		status := "online"
		if d.responseTime != "N/A" {
			status = fmt.Sprintf("online (%s)", d.responseTime)
		}
		if d.address != "" {
			return fmt.Sprintf("%s http://%s", status, d.address)
		}
		return status
	}
	return d.status
}
func (d dockerService) FilterValue() string { return d.title }

type dockerImage struct {
	title  string
	size   string
	created string
	id     string
}

func (d dockerImage) Title() string       { return d.title }
func (d dockerImage) Description() string { return fmt.Sprintf("%s  %s", d.size, d.created) }
func (d dockerImage) FilterValue() string { return d.title }

type dockerHubImage struct {
	title       string
	description string
	stars       int
	pulls       int64
	official    bool
}

func (d dockerHubImage) Title() string { return d.title }
func (d dockerHubImage) Description() string {
	off := ""
	if d.official {
		off = "[official] "
	}
	return fmt.Sprintf("★ %d  ⬇ %d  %s%s", d.stars, d.pulls, off, d.description)
}
func (d dockerHubImage) FilterValue() string { return d.title }

const logoText = `
██╗██╗  ██╗██╗        ███████╗██╗      ██████╗ ██╗    ██╗███████╗██████╗ 
██║╚██╗██╔╝██║        ██╔════╝██║     ██╔═══██╗██║    ██║██╔════╝██╔══██╗
██║ ╚███╔╝ ██║        █████╗  ██║     ██║   ██║██║ █╗ ██║█████╗  ██████╔╝
██║ ██╔██╗ ██║        ██╔══╝  ██║     ██║   ██║██║███╗██║██╔══╝  ██╔══██╗
██║██╔╝ ██╗██║███████╗██║     ███████╗╚██████╔╝╚███╔███╔╝███████╗██║  ██║
╚═╝╚╚═╝  ╚═╝╚═╝╚══════╝╚═╝     ╚══════╝ ╚═════╝  ╚══╝╚══╝ ╚══════╝╚═╝  ╚═╝
`

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)

	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Align(lipgloss.Center).
			MarginBottom(1)

	menuPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	menuTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true).
			MarginBottom(1)

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			PaddingLeft(1)

	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Bold(true).
				PaddingLeft(1)

	listPaneStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			BorderLeft(true).
			BorderRight(true).
			BorderForeground(lipgloss.Color("240"))

	itemTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Align(lipgloss.Center)

	itemDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Align(lipgloss.Center)
	itemSelectedTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Bold(true).
				Align(lipgloss.Center)

	itemSelectedDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Align(lipgloss.Center)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true).
			Padding(0, 1)
)

type item struct {
	title, path string
	sudo        bool
	setProxy    bool
	keyShortcut string
}

type storedItem struct {
	Title       string `json:"title"`
	Path        string `json:"path"`
	Sudo        bool   `json:"sudo,omitempty"`
	SetProxy    bool   `json:"setProxy,omitempty"`
	KeyShortcut string `json:"keyShortcut,omitempty"`
}

type storedProject struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Pinned bool   `json:"pinned,omitempty"`
}

type project struct {
	name, path string
	pinned     bool
}

type bookmark struct {
	title, url string
}

type storedBookmark struct {
	Title string `json:"title"`
	Url   string `json:"url"`
}

type noteCategory struct {
	name    string
	content string
	color   string
}

type storedNoteCategory struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Color   string `json:"color,omitempty"`
}

func (b bookmark) Title() string       { return b.title }
func (b bookmark) Description() string { return b.url }
func (b bookmark) FilterValue() string { return b.title }

func (nc noteCategory) Title() string       { return nc.name }
func (nc noteCategory) Description() string { return fmt.Sprintf("%d chars", len(nc.content)) }
func (nc noteCategory) FilterValue() string { return nc.name }

func (i item) Title() string {
	if i.keyShortcut != "" {
		return fmt.Sprintf("%s [%s]", i.title, i.keyShortcut)
	}
	return i.title
}
func (i item) Description() string {
	desc := i.path
	if i.sudo {
		desc += " (sudo)"
	}
	if i.setProxy {
		if i.sudo {
			desc += ", (proxy)"
		} else {
			desc += " (proxy)"
		}
	}
	return desc
}
func (i item) FilterValue() string { return i.title }

func (p project) Title() string {
	if p.pinned {
		return "📌 " + p.name
	}
	return p.name
}
func (p project) Description() string { return p.path }
func (p project) FilterValue() string { return p.name }

type model struct {
	list             list.Model
	scriptActive     bool
	width            int
	height           int
	lastAction       string
	lastError        error
	lastErrorCleared time.Time

	projectsList   list.Model
	projectsActive bool
	projectAddMode bool
	projectInputs  []textinput.Model
	projectFocus   int
	projectRemove  bool

	menuWidth int

	menuHidden   bool
	footerHidden bool

	settings settings

	removeMode           bool
	addMode              bool
	editMode             bool
	editSelection        int
	addInputs            []textinput.Model
	addFocus             int
	editInputs           []textinput.Model
	editKeyShortcutInput textinput.Model

	searchInput   textinput.Model
	searchFocused bool

	bookmarkList    list.Model
	bookmarksActive bool

	dockerList           list.Model
	dockerServicesActive bool

	dockerImageList    list.Model
	dockerImagesActive bool

	dockerHubList      list.Model
	dockerHubActive    bool
	dockerHubInput     textinput.Model
	dockerHubSearching bool
	dockerHubQuery     string
	hubSearchSeq       int

	imageRunDialogOpen  bool
	imageRunImageRef    string
	imageRunInputs      []textinput.Model
	imageRunFocus       int
	imageRunContainerID string

	noteOpen               bool
	noteFullscreen         bool
	noteInput              textarea.Model
	noteCategoryListOpen   bool
	noteCategoryList       list.Model
	noteCategoryAddMode    bool
	noteCategoryEditMode   bool
	noteCategoryMenuOpen   bool
	noteCategoryMenuIndex  int
	noteCategoryNameInput  textinput.Model
	noteCategories         []noteCategory
	currentCategoryIndex   int
	noteCategoryEditInput  textinput.Model
	noteCategoryColorInput textinput.Model
	noteCategorySaveFolder string

	noteLastSaved         string
	noteSaveSeq           int
	noteSaveErr           error
	noteHasUnsavedChanges bool

	// Htop functionality
	htopActive bool
	htopOutput string
	htopFrame  int

	// Key shortcuts management
	shortcutListOpen     bool
	shortcutEditMode     bool
	shortcutAddMode      bool
	shortcutEditInput    textinput.Model
	shortcutKeyInput     textinput.Model
	shortcutEditFocus    int
	shortcutEditOriginal string

	// Xray status
	xrayActive     bool
	xrayStatus     string
	xrayLastServer string

	// Tor status (like Xray) — uses /etc/ixi-tui-main/tor/ip-changer.sh
	torStatus string
	torIP     string

	// Clock functionality
	clock      clockModel
	clockActive bool
}

func newItemDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()

	d.Styles.NormalTitle = itemTitleStyle
	d.Styles.NormalDesc = itemDescStyle

	d.Styles.SelectedTitle = itemSelectedTitleStyle
	d.Styles.SelectedDesc = itemSelectedDescStyle

	return d
}

func newBookmarkDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()

	d.Styles.NormalTitle = itemTitleStyle
	d.Styles.NormalDesc = itemDescStyle

	d.Styles.SelectedTitle = itemSelectedTitleStyle
	d.Styles.SelectedDesc = itemSelectedDescStyle

	return d
}
func newDockerServiceDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()

	d.Styles.NormalTitle = itemTitleStyle
	d.Styles.NormalDesc = itemDescStyle

	d.Styles.SelectedTitle = itemSelectedTitleStyle
	d.Styles.SelectedDesc = itemSelectedDescStyle
	// another color for search/filter matches — green highlight
	d.Styles.FilterMatch = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Underline(true)

	return d
}

func newDockerHubDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	// distinct "another color" for hub search results — cyan/yellow vs docker services grey/white
	d.Styles.NormalTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Align(lipgloss.Center)
	d.Styles.NormalDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Align(lipgloss.Center)
	d.Styles.SelectedTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true).Align(lipgloss.Center)
	d.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Align(lipgloss.Center)
	d.Styles.FilterMatch = lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true)
	return d
}

func newNoteCategoryDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()

	d.Styles.NormalTitle = itemTitleStyle
	d.Styles.NormalDesc = itemDescStyle

	d.Styles.SelectedTitle = itemSelectedTitleStyle
	d.Styles.SelectedDesc = itemSelectedDescStyle

	return d
}

type noteSaveDebounceMsg struct {
	Seq int
}

type noteSavedMsg struct {
	Content string
	Err     error
}

type smasshFinishedMsg struct {
	Err error
}

type scriptFinishedMsg struct {
	Title string
	Path  string
	Err   error
}

type dockerServicesFetchedMsg struct {
	services []list.Item
	err      error
}

type dockerServiceActionMsg struct {
	service string
	action  string
	err     error
}

type dockerImagesFetchedMsg struct {
	images []list.Item
	err    error
}

type dockerImageActionMsg struct {
	image  string
	action string
	err    error
}

type dockerImageRunFinishedMsg struct {
	containerID string
	err         error
}

type htopOutputMsg struct {
	output string
	err    error
}

type dockerHubSearchMsg struct {
	results []list.Item
	query   string
	err     error
}

type dockerHubPullMsg struct {
	image string
	err   error
}

type torStatusMsg struct {
	status string
	ip     string
}

type hubSearchDebounceMsg struct {
	Seq   int
	Query string
}

type projectsDiscoveredMsg struct {
	projects []storedProject
	count    int
	err      error
}

type xrayStatusMsg struct {
	status string
	server string
}

func fetchDockerServicesCmd() tea.Cmd {
	return func() tea.Msg {
		services, err := getDockerServices()
		return dockerServicesFetchedMsg{services: services, err: err}
	}
}

func dockerServiceActionCmd(service, action string) tea.Cmd {
	return func() tea.Msg {
		err := runDockerServiceAction(service, action)
		return dockerServiceActionMsg{service: service, action: action, err: err}
	}
}

func fetchDockerImagesCmd() tea.Cmd {
	return func() tea.Msg {
		images, err := getDockerImages()
		return dockerImagesFetchedMsg{images: images, err: err}
	}
}

func dockerImageActionCmd(image, action string) tea.Cmd {
	return func() tea.Msg {
		err := runDockerImageAction(image, action)
		return dockerImageActionMsg{image: image, action: action, err: err}
	}
}

func searchDockerHubCmd(query string) tea.Cmd {
	return func() tea.Msg {
		results, err := searchDockerHub(query)
		return dockerHubSearchMsg{results: results, query: query, err: err}
	}
}

func debounceHubSearch(seq int, query string) tea.Cmd {
	return tea.Tick(hubSearchDebounce, func(time.Time) tea.Msg {
		return hubSearchDebounceMsg{Seq: seq, Query: query}
	})
}

func searchDockerHub(query string) ([]list.Item, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://hub.docker.com/v2/search/repositories/?query="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hub search %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Results []struct {
			RepoName         string `json:"repo_name"`
			ShortDescription string `json:"short_description"`
			StarCount        int    `json:"star_count"`
			PullCount        int64  `json:"pull_count"`
			IsOfficial       bool   `json:"is_official"`
		} `json:"results"`
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	var items []list.Item
	for _, r := range out.Results {
		if len(items) >= 50 {
			break
		}
		items = append(items, dockerHubImage{
			title:       r.RepoName,
			description: r.ShortDescription,
			stars:       r.StarCount,
			pulls:       r.PullCount,
			official:    r.IsOfficial,
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return items, nil
}

func dockerHubPullCmd(image string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("docker", "pull", image)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return dockerHubPullMsg{image: image, err: fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))}
		}
		return dockerHubPullMsg{image: image, err: nil}
	}
}

// Clean htop — minimal, not colorful
func htopBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := percent * width / 100
	empty := width - filled
	// subtle single accent, not rainbow
	filledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	barFilled := filledStyle.Render(strings.Repeat("█", filled))
	barEmpty := emptyStyle.Render(strings.Repeat("░", empty))
	pctStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	return fmt.Sprintf("%s%s %s", barFilled, barEmpty, pctStyle.Render(fmt.Sprintf("%3d%%", percent)))
}

// Function to get system stats similar to htop — clean, not messy
func getSystemStats() (string, error) {
	var stats strings.Builder

	// CPU
	cpuCount := 0
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "processor") {
				cpuCount++
			}
		}
	}
	if cpuCount == 0 {
		cpuCount = 1
	}
	load1 := 0.0
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fmt.Sscanf(strings.TrimSpace(string(data)), "%f", &load1)
	}
	cpuPct := int((load1 / float64(cpuCount)) * 100)
	if cpuPct > 100 {
		cpuPct = 100
	}
	cpuLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf("CPU %d", cpuCount))
	stats.WriteString(fmt.Sprintf("%s %s\n", cpuLabel, htopBar(cpuPct, 14)))

	// Memory
	var memTotal, memAvail int64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d", &memTotal)
			}
			if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %d", &memAvail)
			}
		}
	}
	memUsedPct := 0
	if memTotal > 0 {
		memUsedPct = int((memTotal - memAvail) * 100 / memTotal)
	}
	memLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("MEM")
	stats.WriteString(fmt.Sprintf("%s %s\n", memLabel, htopBar(memUsedPct, 14)))

	// Load average (plain, no sparkline)
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(strings.TrimSpace(string(data)))
		if len(fields) >= 3 {
			loadLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("LOAD")
			valsStr := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(fmt.Sprintf("%s %s %s", fields[0], fields[1], fields[2]))
			stats.WriteString(fmt.Sprintf("%s %s\n", loadLabel, valsStr))
		}
	}

	// Processes + uptime (plain)
	procCount := 0
	if files, err := os.ReadDir("/proc"); err == nil {
		for _, f := range files {
			if _, err := strconv.Atoi(f.Name()); err == nil {
				procCount++
			}
		}
	}
	uptimeStr := ""
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		var up float64
		fmt.Sscanf(string(data), "%f", &up)
		d := time.Duration(up * float64(time.Second))
		uptimeStr = fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	procLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("PROCS")
	procVal := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(fmt.Sprintf("%d", procCount))
	upVal := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(uptimeStr)
	stats.WriteString(fmt.Sprintf("%s %s  %s %s\n", procLabel, procVal, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("UP"), upVal))

	return stats.String(), nil
}

// Htop command function
func runHtopCmd() tea.Cmd {
	return func() tea.Msg {
		stats, err := getSystemStats()
		return htopOutputMsg{output: stats, err: err}
	}
}

// Periodic htop refresh command
func htopRefreshCmd() tea.Cmd {
	return tea.Tick(htopRefreshInterval, func(t time.Time) tea.Msg {
		stats, err := getSystemStats()
		return htopOutputMsg{output: stats, err: err}
	})
}

func checkXrayStatusCmd() tea.Cmd {
	return func() tea.Msg {
		status, server := getXrayStatus()
		return xrayStatusMsg{status: status, server: server}
	}
}

func checkTorStatusCmd() tea.Cmd {
	return func() tea.Msg {
		status, ip := getTorStatus()
		return torStatusMsg{status: status, ip: ip}
	}
}

func initialModel() model {
	stored, err := loadScripts()
	if err != nil {
		stored = []storedItem{}

	}

	storedBookmarks, bErr := loadBookmarks()
	if bErr != nil {
		storedBookmarks = []storedBookmark{}
	}

	storedProjects, pErr := loadProjects()
	if pErr != nil {
		storedProjects = []storedProject{}
	}

	notes, notesErr := loadNotes()
	if notesErr != nil {
		notes = ""
	}

	// Load settings including htopActive preference
	savedSettings, _ := loadSettings()

	items := make([]list.Item, 0, len(stored))
	for _, s := range stored {
		items = append(items, item{title: s.Title, path: s.Path, sudo: s.Sudo, setProxy: s.SetProxy, keyShortcut: s.KeyShortcut})
	}

	bookmarkItems := make([]list.Item, 0, len(storedBookmarks))
	for _, b := range storedBookmarks {
		bookmarkItems = append(bookmarkItems, bookmark{title: b.Title, url: b.Url})
	}

	projectItems := make([]list.Item, 0, len(storedProjects))
	// pinned first, then rest
	for _, p := range storedProjects {
		if p.Pinned {
			projectItems = append(projectItems, project{name: p.Name, path: p.Path, pinned: p.Pinned})
		}
	}
	for _, p := range storedProjects {
		if !p.Pinned {
			projectItems = append(projectItems, project{name: p.Name, path: p.Path, pinned: p.Pinned})
		}
	}

	delegate := newItemDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Available Scripts"
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true).MarginBottom(1).Align(lipgloss.Center)

	bDelegate := newBookmarkDelegate()
	bl := list.New(bookmarkItems, bDelegate, 0, 0)
	bl.Title = "Bookmarks"
	bl.SetShowTitle(false)
	bl.SetShowHelp(false)
	bl.SetFilteringEnabled(true)
	bl.SetShowFilter(false)

	pDelegate := newItemDelegate()
	pl := list.New(projectItems, pDelegate, 0, 0)
	pl.Title = "Projects"
	pl.SetShowTitle(false)
	pl.SetShowHelp(false)
	pl.SetFilteringEnabled(true)
	pl.SetShowFilter(false)

	dockerItems := make([]list.Item, 0)
	dlDelegate := newDockerServiceDelegate()
	dl := list.New(dockerItems, dlDelegate, 0, 0)
	dl.Title = "Docker Services"
	dl.SetShowTitle(false)
	dl.SetShowHelp(false)
	dl.SetFilteringEnabled(false)

	dockerImageItems := make([]list.Item, 0)
	dilDelegate := newDockerServiceDelegate()
	dil := list.New(dockerImageItems, dilDelegate, 0, 0)
	dil.Title = "Docker Images"
	dil.SetShowTitle(false)
	dil.SetShowHelp(false)
	dil.SetFilteringEnabled(false)

	dhDelegate := newDockerHubDelegate()
	dhl := list.New([]list.Item{}, dhDelegate, 0, 0)
	dhl.Title = "Docker Hub"
	dhl.SetShowTitle(false)
	dhl.SetShowHelp(false)
	dhl.SetFilteringEnabled(false)

	dockerHubInput := textinput.New()
	dockerHubInput.Placeholder = "search Docker Hub (e.g. nginx)"
	dockerHubInput.Prompt = "Hub: "
	dockerHubInput.CharLimit = 128
	dockerHubInput.Width = 40
	dockerHubInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	dockerHubInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	dockerHubInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	dockerHubInput.Blur()

	nameInput := textinput.New()
	nameInput.Placeholder = "Script name"
	nameInput.Prompt = "Name: "
	nameInput.CharLimit = 80
	nameInput.Width = 28

	pathInput := textinput.New()
	pathInput.Placeholder = "/path/to/script.sh"
	pathInput.Prompt = "Path: "
	pathInput.CharLimit = 256
	pathInput.Width = 28

	projectNameInput := textinput.New()
	projectNameInput.Placeholder = "Project name"
	projectNameInput.Prompt = "Name: "
	projectNameInput.CharLimit = 80
	projectNameInput.Width = 28

	projectPathInput := textinput.New()
	projectPathInput.Placeholder = "/path/to/project"
	projectPathInput.Prompt = "Path: "
	projectPathInput.CharLimit = 256
	projectPathInput.Width = 28

	editNameInput := textinput.New()
	editNameInput.Placeholder = "Script name"
	editNameInput.Prompt = "Name: "
	editNameInput.CharLimit = 80
	editNameInput.Width = 28

	editPathInput := textinput.New()
	editPathInput.Placeholder = "/path/to/script.sh"
	editPathInput.Prompt = "Path: "
	editPathInput.CharLimit = 256
	editPathInput.Width = 28

	editKeyShortcutInput := textinput.New()
	editKeyShortcutInput.Placeholder = "Key shortcut (e.g., F1, Ctrl+A)"
	editKeyShortcutInput.Prompt = "Key: "
	editKeyShortcutInput.CharLimit = 20
	editKeyShortcutInput.Width = 28

	searchInput := textinput.New()
	searchInput.Prompt = "Search: "
	searchInput.CharLimit = 64
	searchInput.Width = 30
	searchInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	searchInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	searchInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	searchInput.Blur()

	noteInput := textarea.New()
	noteInput.Placeholder = "Notes..."
	noteInput.Prompt = ""
	noteInput.ShowLineNumbers = false
	noteInput.SetValue(notes)
	noteInput.Blur()

	// Set default focused style with white border
	focusedStyle := noteInput.FocusedStyle
	focusedStyle.Base = focusedStyle.Base.BorderForeground(lipgloss.Color("255"))
	noteInput.FocusedStyle = focusedStyle

	// Load note categories
	storedCategories, catErr := loadNoteCategories()
	if catErr != nil {
		storedCategories = []storedNoteCategory{}
	}

	noteCategoryItems := make([]list.Item, 0, len(storedCategories))
	noteCategories := make([]noteCategory, 0, len(storedCategories))
	for _, c := range storedCategories {
		noteCategoryItems = append(noteCategoryItems, noteCategory{name: c.Name, content: c.Content, color: c.Color})
		noteCategories = append(noteCategories, noteCategory{name: c.Name, content: c.Content, color: c.Color})
	}

	ncDelegate := newNoteCategoryDelegate()
	ncl := list.New(noteCategoryItems, ncDelegate, 0, 0)
	ncl.Title = "Note Categories"
	ncl.SetShowTitle(false)
	ncl.SetShowHelp(false)
	ncl.SetFilteringEnabled(false)

	noteCategoryNameInput := textinput.New()
	noteCategoryNameInput.Placeholder = "Category name"
	noteCategoryNameInput.Prompt = "Name: "
	noteCategoryNameInput.CharLimit = 80
	noteCategoryNameInput.Width = 28
	noteCategoryNameInput.Blur()

	noteCategoryEditInput := textinput.New()
	noteCategoryEditInput.Placeholder = "Edit category name"
	noteCategoryEditInput.Prompt = "Name: "
	noteCategoryEditInput.CharLimit = 80
	noteCategoryEditInput.Width = 28
	noteCategoryEditInput.Blur()

	noteCategoryColorInput := textinput.New()
	noteCategoryColorInput.Placeholder = "Color (e.g., 255, red, #FF0000)"
	noteCategoryColorInput.Prompt = "Color: "
	noteCategoryColorInput.CharLimit = 20
	noteCategoryColorInput.Width = 28
	noteCategoryColorInput.Blur()

	shortcutEditInput := textinput.New()
	shortcutEditInput.Placeholder = "Script name"
	shortcutEditInput.Prompt = "Name: "
	shortcutEditInput.CharLimit = 80
	shortcutEditInput.Width = 40
	shortcutEditInput.Blur()

	shortcutKeyInput := textinput.New()
	shortcutKeyInput.Placeholder = "Key shortcut (e.g., F1, Ctrl+A)"
	shortcutKeyInput.Prompt = "Key: "
	shortcutKeyInput.CharLimit = 20
	shortcutKeyInput.Width = 40
	shortcutKeyInput.Blur()

	runNameInput := textinput.New()
	runNameInput.Placeholder = "my-container (optional, auto-generated)"
	runNameInput.Prompt = "Name: "
	runNameInput.CharLimit = 80
	runNameInput.Width = 40
	runNameInput.Blur()

	runPortsInput := textinput.New()
	runPortsInput.Placeholder = "80:80, 3000:3000 (optional)"
	runPortsInput.Prompt = "Ports: "
	runPortsInput.CharLimit = 256
	runPortsInput.Width = 40
	runPortsInput.Blur()

	runVolumesInput := textinput.New()
	runVolumesInput.Placeholder = "/host:/container (optional)"
	runVolumesInput.Prompt = "Volumes: "
	runVolumesInput.CharLimit = 256
	runVolumesInput.Width = 40
	runVolumesInput.Blur()

	runExtraInput := textinput.New()
	runExtraInput.Placeholder = "-e ENV=val --restart always (optional)"
	runExtraInput.Prompt = "Extra: "
	runExtraInput.CharLimit = 512
	runExtraInput.Width = 40
	runExtraInput.Blur()

	return model{
		list:                   l,
		scriptActive:           false,
		bookmarkList:           bl,
		projectsList:           pl,
		projectsActive:         false,
		dockerList:             dl,
		dockerImageList:        dil,
		noteCategoryList:       ncl,
		noteCategoryNameInput:  noteCategoryNameInput,
		noteCategoryEditInput:  noteCategoryEditInput,
		noteCategoryColorInput: noteCategoryColorInput,
		noteCategories:         noteCategories,
		currentCategoryIndex:   -1,
		noteCategorySaveFolder: "",
		menuWidth:              30,
		footerHidden:           true,
		addInputs:              []textinput.Model{nameInput, pathInput},
		addFocus:               0,
		editInputs:             []textinput.Model{editNameInput, editPathInput},
		editSelection:          0,
		editKeyShortcutInput:   editKeyShortcutInput,
		projectInputs:          []textinput.Model{projectNameInput, projectPathInput},
		projectFocus:           0,
		searchInput:            searchInput,
		noteInput:              noteInput,
		noteLastSaved:          notes,
		noteSaveErr:            notesErr,
		htopActive:             savedSettings.HtopActive,
		htopOutput:             "",
		shortcutEditInput:      shortcutEditInput,
		shortcutKeyInput:       shortcutKeyInput,
		imageRunInputs:         []textinput.Model{runNameInput, runPortsInput, runVolumesInput, runExtraInput},
		imageRunFocus:          0,
		imageRunContainerID:    "",
		dockerHubList:          dhl,
		dockerHubInput:         dockerHubInput,
		clock:                  initialClockModel(0, 0), // Initialize with zero width/height, will be updated by WindowSizeMsg
	}
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.htopActive {
		cmds = append(cmds, htopRefreshCmd())
	}
	cmds = append(cmds, checkXrayStatusCmd())
	cmds = append(cmds, checkTorStatusCmd())
	return tea.Batch(cmds...)
}

func debounceNoteSave(seq int) tea.Cmd {
	return tea.Tick(noteAutosaveDebounce, func(time.Time) tea.Msg {
		return noteSaveDebounceMsg{Seq: seq}
	})
}

func saveNotesCmd(content string) tea.Cmd {
	return func() tea.Msg {
		err := saveNotes(content)
		return noteSavedMsg{Content: content, Err: err}
	}
}

func runSmasshCmd() tea.Cmd {
	cmd := exec.Command("smassh")
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return smasshFinishedMsg{Err: err}
	})
}

func runScriptCmd(title, path string, sudo bool, setProxy bool) tea.Cmd {

	cleanPath := strings.TrimSpace(path)
	dir := filepath.Dir(cleanPath)
	base := filepath.Base(cleanPath)

	quotedDir := strings.ReplaceAll(dir, "\"", "\\\"")
	quotedBase := strings.ReplaceAll(base, "\"", "\\\"")

	isGoFile := strings.HasSuffix(strings.ToLower(base), ".go")
	if isGoFile {
		var prefix string
		if setProxy {
			prefix = "export HTTP_PROXY=\"http://127.0.0.1:10808\"; export HTTPS_PROXY=\"http://127.0.0.1:10808\"; echo 'Proxy set: HTTP_PROXY and HTTPS_PROXY = http://127.0.0.1:10808'; "
		}
		binName := ".ixi_flower_tmp_run"
		runLine := fmt.Sprintf("%sgo build -o \"%s\" \"%s\" && \"./%s\"; rm -f \"%s\"", prefix, binName, quotedBase, binName, binName)
		if sudo {
			runLine = fmt.Sprintf("%sgo build -o \"%s\" \"%s\" && sudo -E \"./%s\"; rm -f \"%s\"", prefix, binName, quotedBase, binName, binName)
		}
		shellCmd := fmt.Sprintf("cd \"%s\" && %s", quotedDir, runLine)
		_ = logCommand(shellCmd)
		var cmd *exec.Cmd
		if sudo {
			cmd = exec.Command("sudo", "bash", "-lc", shellCmd)
		} else {
			cmd = exec.Command("bash", "-lc", shellCmd)
		}
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			return scriptFinishedMsg{Title: title, Path: path, Err: err}
		})
	}

	var prefix string
	if setProxy {
		prefix = "export HTTP_PROXY=\"http://127.0.0.1:10808\"; export HTTPS_PROXY=\"http://127.0.0.1:10808\"; echo 'Proxy set: HTTP_PROXY and HTTPS_PROXY = http://127.0.0.1:10808'; "
	}

	runLine := fmt.Sprintf("./%s", quotedBase)
	shellCmd := fmt.Sprintf("%scd \"%s\" && %s", prefix, quotedDir, runLine)

	_ = logCommand(shellCmd)

	var cmd *exec.Cmd
	if sudo {
		cmd = exec.Command("sudo", "bash", "-lc", shellCmd)
	} else {
		cmd = exec.Command("bash", "-lc", shellCmd)
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return scriptFinishedMsg{Title: title, Path: path, Err: err}
	})
}

func openScriptInNewTerminalCmd(title, path string, sudo bool, setProxy bool) tea.Cmd {

	cleanPath := strings.TrimSpace(path)
	dir := filepath.Dir(cleanPath)
	base := filepath.Base(cleanPath)

	quotedDir := strings.ReplaceAll(dir, "\"", "\\\"")
	quotedBase := strings.ReplaceAll(base, "\"", "\\\"")

	var runLine string
	if strings.HasSuffix(strings.ToLower(base), ".go") {
		binName := ".ixi_flower_tmp_run"
		if sudo {
			runLine = fmt.Sprintf("cd \"%s\" && go build -o \"%s\" \"%s\" && sudo -E \"./%s\"; rm -f \"%s\"", quotedDir, binName, quotedBase, binName, binName)
		} else {
			runLine = fmt.Sprintf("cd \"%s\" && go build -o \"%s\" \"%s\" && \"./%s\"; rm -f \"%s\"", quotedDir, binName, quotedBase, binName, binName)
		}
	} else if sudo {
		runLine = fmt.Sprintf("cd \"%s\" && sudo ./%s", quotedDir, quotedBase)
	} else {
		runLine = fmt.Sprintf("cd \"%s\" && ./%s", quotedDir, quotedBase)
	}

	prefix := ""
	if setProxy {
		prefix = "export HTTP_PROXY=\"http://127.0.0.1:10808\"; export HTTPS_PROXY=\"http://127.0.0.1:10808\"; echo 'Proxy set: HTTP_PROXY and HTTPS_PROXY = http://127.0.0.1:10808'; "
	}
	shellCmd := fmt.Sprintf("%s%s; echo; echo '--- finished ---'; read -r -n 1 -s -p 'Press any key to close...'", prefix, runLine)

	_ = logCommand(shellCmd)

	var cmd *exec.Cmd
	if term, err := exec.LookPath("xdg-terminal-exec"); err == nil {
		cmd = exec.Command(term, "--hold", "--", "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("kitty"); err == nil {
		cmd = exec.Command(term, "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("konsole"); err == nil {
		cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("gnome-terminal"); err == nil {
		cmd = exec.Command(term, "--", "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("xterm"); err == nil {
		cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
	} else {

		cmd = exec.Command("bash", "-lc", shellCmd)
	}

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return scriptFinishedMsg{Title: title, Path: path, Err: err}
	})
}

func openUrlCmd(title, url string) tea.Cmd {
	cmd := exec.Command("xdg-open", url)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return scriptFinishedMsg{Title: title, Path: url, Err: err}
	})
}

func openProjectInNewTerminalCmd(title, projectPath, editor string) tea.Cmd {

	editorCmd := editor
	if editor == "nvim" {
		editorCmd = "nvim ."
	}
	shellCmd := fmt.Sprintf("cd %q && %s", projectPath, editorCmd)
	cmd := exec.Command("bash", "-lc", shellCmd)
	if term, err := exec.LookPath("alacritty"); err == nil {
		cmd = exec.Command(term, "--hold", "--", "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("kitty"); err == nil {
		cmd = exec.Command(term, "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("konsole"); err == nil {
		cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("gnome-terminal"); err == nil {
		cmd = exec.Command(term, "--", "bash", "-lc", shellCmd)
	} else if term, err := exec.LookPath("xterm"); err == nil {
		cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
	}

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return scriptFinishedMsg{Title: title, Path: projectPath, Err: err}
	})
}

func openProjectInFileManagerCmd(title, projectPath string) tea.Cmd {

	cmd := exec.Command("xdg-open", projectPath)
	if thunar, err := exec.LookPath("thunar"); err == nil {
		cmd = exec.Command(thunar, projectPath)
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return scriptFinishedMsg{Title: title, Path: projectPath, Err: err}
	})
}

func openProjectInVSCodeCmd(title, projectPath string) tea.Cmd {
	cmd := exec.Command("code", projectPath)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return scriptFinishedMsg{Title: title, Path: projectPath, Err: err}
	})
}

func openProjectAICmd(title, projectPath, aiCmd string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		// hermes/opencode/claude/codex all work from project dir
		shellCmd := fmt.Sprintf("cd %q && %s; exec bash", projectPath, aiCmd)
		if term, err := exec.LookPath("alacritty"); err == nil {
			cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
		} else if term, err := exec.LookPath("kitty"); err == nil {
			cmd = exec.Command(term, "bash", "-lc", shellCmd)
		} else if term, err := exec.LookPath("konsole"); err == nil {
			cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
		} else if term, err := exec.LookPath("gnome-terminal"); err == nil {
			cmd = exec.Command(term, "--", "bash", "-lc", shellCmd)
		} else if term, err := exec.LookPath("xterm"); err == nil {
			cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
		} else {
			return scriptFinishedMsg{Title: title, Path: projectPath, Err: fmt.Errorf("no terminal emulator found for %s", aiCmd)}
		}
		if err := cmd.Start(); err != nil {
			return scriptFinishedMsg{Title: title, Path: projectPath, Err: err}
		}
		return scriptFinishedMsg{Title: title, Path: projectPath, Err: nil}
	}
}

func openKittyTerminalCmd() tea.Cmd {
	// Launch terminal as a completely independent process
	// This way it stays open even after the TUI closes
	return func() tea.Msg {
		var cmd *exec.Cmd

		// Try to find available terminal emulators
		if term, err := exec.LookPath("alacritty"); err == nil {
			cmd = exec.Command(term)
		} else if term, err := exec.LookPath("kitty"); err == nil {
			cmd = exec.Command(term)
		} else if term, err := exec.LookPath("konsole"); err == nil {
			cmd = exec.Command(term)
		} else if term, err := exec.LookPath("gnome-terminal"); err == nil {
			cmd = exec.Command(term)
		} else if term, err := exec.LookPath("xterm"); err == nil {
			cmd = exec.Command(term)
		} else {
			return scriptFinishedMsg{Title: "Terminal", Path: "", Err: fmt.Errorf("no terminal emulator found")}
		}

		// Start the terminal as an independent process
		err := cmd.Start()
		if err != nil {
			return scriptFinishedMsg{Title: "Terminal", Path: "", Err: err}
		}

		// Don't wait for the process - let it run independently
		return scriptFinishedMsg{Title: "Terminal", Path: "", Err: nil}
	}
}

func openProjectTerminalCmd(title, projectPath string) tea.Cmd {
	// Launch terminal as a completely independent process
	// This way it stays open even after the TUI closes
	return func() tea.Msg {
		var cmd *exec.Cmd
		if term, err := exec.LookPath("alacritty"); err == nil {
			cmd = exec.Command(term, "--working-directory", projectPath)
		} else if term, err := exec.LookPath("kitty"); err == nil {
			cmd = exec.Command(term, "--directory", projectPath)
		} else if term, err := exec.LookPath("konsole"); err == nil {
			cmd = exec.Command(term, "--workdir", projectPath)
		} else if term, err := exec.LookPath("gnome-terminal"); err == nil {
			cmd = exec.Command(term, "--working-directory", projectPath)
		} else if term, err := exec.LookPath("xterm"); err == nil {
			cmd = exec.Command(term, "-e", "bash", "-lc", fmt.Sprintf("cd %q && exec bash", projectPath))
		} else {
			// Fallback: no suitable terminal found
			return scriptFinishedMsg{Title: title, Path: projectPath, Err: fmt.Errorf("no terminal emulator found")}
		}

		// Start the terminal as an independent process
		err := cmd.Start()
		if err != nil {
			return scriptFinishedMsg{Title: title, Path: projectPath, Err: err}
		}

		// Don't wait for the process - let it run independently
		return scriptFinishedMsg{Title: title, Path: projectPath, Err: nil}
	}
}

func autoDiscoverProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		baseDir := "/home/ixi_flower/Documents"
		var discovered []storedProject

		// Check if base directory exists
		if _, err := os.Stat(baseDir); os.IsNotExist(err) {
			return projectsDiscoveredMsg{projects: nil, count: 0, err: fmt.Errorf("directory not found: %s", baseDir)}
		}

		// Read all entries in Documents
		entries, err := os.ReadDir(baseDir)
		if err != nil {
			return projectsDiscoveredMsg{projects: nil, count: 0, err: err}
		}

		// Look for directories that might be projects
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			projectPath := filepath.Join(baseDir, entry.Name())

			// Check if it's likely a project directory
			// (contains .git, go.mod, package.json, etc.)
			isProject := false

			// Check for common project indicators
			indicators := []string{".git", "go.mod", "package.json", "pom.xml", "Cargo.toml", "requirements.txt", ".gitignore"}
			for _, indicator := range indicators {
				if _, err := os.Stat(filepath.Join(projectPath, indicator)); err == nil {
					isProject = true
					break
				}
			}

			// Add project if it looks like one
			if isProject {
				discovered = append(discovered, storedProject{
					Name: entry.Name(),
					Path: projectPath,
				})
			}
		}

		return projectsDiscoveredMsg{projects: discovered, count: len(discovered), err: nil}
	}
}

type settings struct {
	MenuHidden   bool `json:"menuHidden"`
	FooterHidden bool `json:"footerHidden"`
	MenuWidth    int  `json:"menuWidth"`
	HtopActive   bool `json:"htopActive"`
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "ixi-flower-tui")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "settings.json"), nil
}

func loadSettings() (settings, error) {
	p, err := settingsPath()
	if err != nil {
		return settings{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return settings{}, nil
		}
		return settings{}, err
	}
	var s settings
	if err := json.Unmarshal(b, &s); err != nil {
		return settings{}, err
	}
	return s, nil
}

func saveSettings(s settings) error {
	p, err := settingsPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func storagePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "ixi-flower-tui")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "scripts.json"), nil
}

func notesPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "ixi-flower-tui")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "notes.txt"), nil
}

func loadNotes() (string, error) {
	p, err := notesPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func saveNotes(content string) error {
	p, err := notesPath()
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

func defaultScripts() []storedItem {

	return []storedItem{}
}

func loadScripts() ([]storedItem, error) {
	p, err := storagePath()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {

			return []storedItem{}, nil
		}
		return nil, err
	}

	var out []storedItem
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}

	if isLegacyDefaultList(out) {
		return []storedItem{}, nil
	}

	return out, nil
}

func saveScripts(scripts []storedItem) error {
	p, err := storagePath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(scripts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func projectsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "ixi-flower-tui")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "projects.json"), nil
}

func bookmarksPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "ixi-flower-tui")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "bookmarks.json"), nil
}

func loadBookmarks() ([]storedBookmark, error) {
	p, err := bookmarksPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []storedBookmark{}, nil
		}
		return nil, err
	}
	var out []storedBookmark
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveBookmarks(bookmarks []storedBookmark) error {
	p, err := bookmarksPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(bookmarks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func loadProjects() ([]storedProject, error) {
	p, err := projectsPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []storedProject{}, nil
		}
		return nil, err
	}
	var out []storedProject
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveProjects(projects []storedProject) error {
	p, err := projectsPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func noteCategoriesPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "ixi-flower-tui")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "note_categories.json"), nil
}

func loadNoteCategories() ([]storedNoteCategory, error) {
	p, err := noteCategoriesPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []storedNoteCategory{}, nil
		}
		return nil, err
	}
	var out []storedNoteCategory
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveNoteCategories(categories []storedNoteCategory) error {
	p, err := noteCategoriesPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(categories, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update clock model dimensions
		m.clock.width = msg.Width
		m.clock.height = msg.Height

		logoHeight := 1
		if msg.Width >= 80 {
			logoHeight = lipgloss.Height(logoText)
		}
		topHeight := logoHeight + 3
		availableHeight := m.height - topHeight - 4

		if !m.footerHidden {
			availableHeight -= 1
		}

		if m.noteOpen {
			m.noteInput.SetWidth(max(10, m.width-4))
			if m.noteFullscreen {

				m.noteInput.SetHeight(max(3, availableHeight))

				return m, nil
			}

			availableHeight -= 6
			if availableHeight < 5 {
				availableHeight = 5
			}
			m.noteInput.SetHeight(5)
		}

		listWidth := m.width - 4
		if !m.menuHidden {
			// responsive sidebar — good on small windows, fixes tor/xray bad style
			if m.width < 50 {
				m.menuWidth = 14
			} else if m.width < 70 {
				m.menuWidth = 20
			} else if m.width < 90 {
				m.menuWidth = 26
			} else {
				minMenu := 18
				maxMenu := max(minMenu, m.width-30)
				if m.menuWidth < minMenu {
					m.menuWidth = minMenu
				}
				if m.menuWidth > maxMenu {
					m.menuWidth = maxMenu
				}
			}
			listWidth = m.width - m.menuWidth - 1 - 4
			if listWidth < 10 {
				listWidth = 10
				if m.width < 50 {
					// auto-hide sidebar on very small screens to keep content readable
					m.menuHidden = true
					listWidth = m.width - 4
				}
			}
		}

		listHeight := availableHeight - 2
		if listHeight < 3 {
			listHeight = 3
		}

		m.searchInput.Width = max(10, listWidth-10)
		m.dockerHubInput.Width = max(10, listWidth-10)
		m.list.SetSize(listWidth, listHeight)
		m.bookmarkList.SetSize(listWidth, listHeight)
		m.projectsList.SetSize(listWidth, listHeight)
		m.dockerList.SetSize(listWidth, listHeight)
		m.dockerImageList.SetSize(listWidth, listHeight)
		m.dockerHubList.SetSize(listWidth, listHeight)

	case tickMsg, startMsg, pauseMsg, resetMsg: // Messages specific to the clock
		if m.clockActive {
			var clockCmd tea.Cmd
			m.clock, clockCmd = m.clock.Update(msg)
			return m, clockCmd
		}

	case noteSaveDebounceMsg:

		if msg.Seq != m.noteSaveSeq {
			return m, nil
		}

		cur := m.noteInput.Value()
		if cur == m.noteLastSaved {
			return m, nil
		}
		return m, saveNotesCmd(cur)

	case noteSavedMsg:
		m.noteSaveErr = msg.Err
		if msg.Err == nil {
			m.noteLastSaved = msg.Content
		}
		return m, nil

	case smasshFinishedMsg:
		if msg.Err != nil {
			m.lastAction = "smassh exited: " + msg.Err.Error()
		} else {
			m.lastAction = "smassh finished"
		}
		return m, nil

	case scriptFinishedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err
			m.lastAction = fmt.Sprintf("Script %q FAILED: %v", msg.Title, msg.Err)
			m.lastErrorCleared = time.Time{}
		} else {
			m.lastError = nil
			m.lastAction = fmt.Sprintf("Script %q finished", msg.Title)
		}
		return m, nil

	case tea.MouseMsg:

		if m.noteOpen && m.noteFullscreen {
			return m, nil
		}

		if m.menuHidden {
			return m, nil
		}

		headerHeight := lipgloss.Height(logoText) + 3

		switch msg.Action {
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft && msg.Y >= headerHeight {

			}

		case tea.MouseActionRelease:
			return m, nil

		case tea.MouseActionMotion:
			return m, nil
		}

	case dockerServicesFetchedMsg:
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = "Failed to fetch docker services"
		} else {
			m.dockerList.SetItems(msg.services)
			m.lastAction = "Docker services loaded"
		}
		return m, nil

	case dockerServiceActionMsg:
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = fmt.Sprintf("Failed to %s %s", msg.action, msg.service)
			return m, nil
		}
		m.lastAction = fmt.Sprintf("%s %s", strings.Title(msg.action), msg.service)
		return m, fetchDockerServicesCmd()

	case dockerImagesFetchedMsg:
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = "Failed to fetch docker images"
		} else {
			m.dockerImageList.SetItems(msg.images)
			m.lastAction = "Docker images loaded"
		}
		return m, nil

	case dockerImageActionMsg:
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = fmt.Sprintf("Failed to %s %s", msg.action, msg.image)
			return m, nil
		}
		m.lastAction = fmt.Sprintf("Image %s %sd", msg.image, msg.action)
		if m.dockerImagesActive {
			return m, fetchDockerImagesCmd()
		}
		return m, nil

	case dockerImageRunFinishedMsg:
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = "Failed to run container: " + msg.err.Error()
		} else {
			m.lastAction = "Container started: " + msg.containerID
			m.imageRunContainerID = msg.containerID
		}
		if m.dockerImagesActive {
			return m, fetchDockerImagesCmd()
		}
		return m, nil

	case dockerHubSearchMsg:
		m.dockerHubSearching = false
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = "Hub search failed: " + msg.err.Error()
			return m, nil
		}
		m.dockerHubList.SetItems(msg.results)
		m.dockerHubQuery = msg.query
		m.lastAction = fmt.Sprintf("Hub: %d results for %q (enter/p=pull, r=run)", len(msg.results), msg.query)
		return m, nil

	case dockerHubPullMsg:
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = "Pull failed: " + msg.err.Error()
		} else {
			m.lastAction = "Pulled " + msg.image + " — press r to run or check Images (i)"
			m.lastError = nil
		}
		if m.dockerImagesActive {
			return m, fetchDockerImagesCmd()
		}
		return m, nil

	case hubSearchDebounceMsg:
		if msg.Seq != m.hubSearchSeq {
			return m, nil
		}
		q := strings.TrimSpace(msg.Query)
		if q == "" {
			m.dockerHubList.SetItems([]list.Item{})
			m.dockerHubQuery = ""
			m.dockerHubSearching = false
			return m, nil
		}
		if len(q) < 2 {
			return m, nil
		}
		m.dockerHubSearching = true
		m.lastAction = "Searching Hub for " + q + "..."
		return m, searchDockerHubCmd(q)

	case htopOutputMsg:
		m.htopFrame++
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = "htop error: " + msg.err.Error()
		} else {
			m.htopOutput = msg.output
			// keep lastAction minimal for butiful view
		}
		// Continue refreshing htop
		if m.htopActive {
			return m, htopRefreshCmd()
		}
		return m, nil

	case xrayStatusMsg:
		m.xrayStatus = msg.status
		m.xrayLastServer = msg.server
		return m, nil

	case torStatusMsg:
		m.torStatus = msg.status
		m.torIP = msg.ip
		return m, nil

	case projectsDiscoveredMsg:
		if msg.err != nil {
			m.lastError = msg.err
			m.lastAction = "Auto-discover failed: " + msg.err.Error()
			return m, nil
		}

		// Get existing projects to avoid duplicates
		existingProjects := make(map[string]bool)
		for _, item := range m.projectsList.Items() {
			if p, ok := item.(project); ok {
				existingProjects[p.path] = true
			}
		}

		// Add new projects
		newCount := 0
		for _, discovered := range msg.projects {
			if !existingProjects[discovered.Path] {
				m.projectsList.InsertItem(len(m.projectsList.Items()), project{name: discovered.Name, path: discovered.Path})
				newCount++
			}
		}

		// Save the updated list
		if newCount > 0 {
			if err := saveProjects(listProjectsToStored(m.projectsList.Items())); err != nil {
				m.lastAction = fmt.Sprintf("Found %d projects, but failed to save: %s", newCount, err.Error())
			} else {
				m.lastAction = fmt.Sprintf("Auto-discovered and added %d new projects!", newCount)
			}
		} else {
			m.lastAction = fmt.Sprintf("Found %d projects, but all were already in the list", msg.count)
		}
		return m, nil

	case tea.KeyMsg:
		if m.clockActive {
			var clockCmd tea.Cmd
			m.clock, clockCmd = m.clock.Update(msg)
			return m, clockCmd
		}

		// Handle shortcuts list navigation
		if m.shortcutListOpen {
			switch msg.String() {
			case "esc", "enter":
				m.shortcutListOpen = false
				m.lastAction = "Shortcuts list closed"
				return m, nil
			case "a", "A":
				// Add new shortcut
				m.shortcutListOpen = false
				m.shortcutAddMode = true
				m.shortcutEditFocus = 0
				m.shortcutEditInput.SetValue("")
				m.shortcutKeyInput.SetValue("")
				m.shortcutEditInput.Focus()
				m.shortcutKeyInput.Blur()
				m.lastAction = "Add new shortcut (Tab: switch fields, Enter: save, Esc: cancel)"
				return m, nil
			case "d", "D":
				// Delete shortcut from selected item
				if idx := m.list.Index(); idx >= 0 {
					if it, ok := m.list.SelectedItem().(item); ok {
						if it.keyShortcut != "" {
							it.keyShortcut = ""
							m.list.SetItem(idx, it)
							if err := saveScripts(listItemsToStored(m.list.Items())); err != nil {
								m.lastAction = "Removed shortcut, but failed to save: " + err.Error()
							} else {
								m.lastAction = fmt.Sprintf("Removed shortcut from: %s", it.title)
							}
						} else {
							m.lastAction = "This script has no shortcut to remove"
						}
					}
				}
				return m, nil
			case "r", "R", "e", "E":
				// Rename/Edit shortcut from selected item
				if idx := m.list.Index(); idx >= 0 {
					if it, ok := m.list.SelectedItem().(item); ok {
						m.shortcutListOpen = false
						m.shortcutEditMode = true
						m.shortcutEditFocus = 0
						m.shortcutEditInput.SetValue(it.title)
						m.shortcutKeyInput.SetValue(it.keyShortcut)
						m.shortcutEditOriginal = it.title
						m.shortcutEditInput.Focus()
						m.shortcutKeyInput.Blur()
						m.lastAction = "Edit shortcut (Tab: switch fields, Enter: save, Esc: cancel)"
						return m, nil
					}
				}
				return m, nil
			}
			// Navigate the list
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

		// Handle shortcut add mode
		if m.shortcutAddMode {
			switch msg.String() {
			case "esc":
				m.shortcutAddMode = false
				m.lastAction = "Cancelled adding shortcut"
				return m, nil
			case "tab", "shift+tab":
				m.shortcutEditFocus = (m.shortcutEditFocus + 1) % 2
				if m.shortcutEditFocus == 0 {
					m.shortcutEditInput.Focus()
					m.shortcutKeyInput.Blur()
				} else {
					m.shortcutEditInput.Blur()
					m.shortcutKeyInput.Focus()
				}
				return m, nil
			case "enter":
				scriptName := strings.TrimSpace(m.shortcutEditInput.Value())
				newShortcut := strings.TrimSpace(m.shortcutKeyInput.Value())

				if scriptName == "" || newShortcut == "" {
					m.lastAction = "Both script name and shortcut key are required"
					return m, nil
				}

				// Find the script by name and add shortcut
				found := false
				for i, listItem := range m.list.Items() {
					if it, ok := listItem.(item); ok {
						if it.title == scriptName {
							// Check for duplicate shortcuts
							for _, checkItem := range m.list.Items() {
								if checkIt, ok := checkItem.(item); ok {
									if checkIt.title != it.title && strings.EqualFold(checkIt.keyShortcut, newShortcut) {
										m.lastAction = fmt.Sprintf("Shortcut '%s' already used by '%s'", newShortcut, checkIt.title)
										return m, nil
									}
								}
							}

							it.keyShortcut = newShortcut
							m.list.SetItem(i, it)
							if err := saveScripts(listItemsToStored(m.list.Items())); err != nil {
								m.lastAction = "Added shortcut, but failed to save: " + err.Error()
							} else {
								m.lastAction = fmt.Sprintf("Added shortcut '%s' to '%s'", newShortcut, scriptName)
							}
							m.shortcutAddMode = false
							found = true
							break
						}
					}
				}

				if !found {
					m.lastAction = fmt.Sprintf("Script '%s' not found", scriptName)
				}
				return m, nil
			}

			// Update focused input
			if m.shortcutEditFocus == 0 {
				m.shortcutEditInput, cmd = m.shortcutEditInput.Update(msg)
			} else {
				m.shortcutKeyInput, cmd = m.shortcutKeyInput.Update(msg)
			}
			return m, cmd
		}

		// Handle shortcut edit mode
		if m.shortcutEditMode {
			switch msg.String() {
			case "esc":
				m.shortcutEditMode = false
				m.lastAction = "Cancelled editing shortcut"
				return m, nil
			case "tab", "shift+tab":
				m.shortcutEditFocus = (m.shortcutEditFocus + 1) % 2
				if m.shortcutEditFocus == 0 {
					m.shortcutEditInput.Focus()
					m.shortcutKeyInput.Blur()
				} else {
					m.shortcutEditInput.Blur()
					m.shortcutKeyInput.Focus()
				}
				return m, nil
			case "enter":
				newShortcut := strings.TrimSpace(m.shortcutKeyInput.Value())

				// Find the script by original name and update shortcut
				for i, listItem := range m.list.Items() {
					if it, ok := listItem.(item); ok {
						if it.title == m.shortcutEditOriginal {
							// Check for duplicate shortcuts (excluding current item)
							if newShortcut != "" {
								for _, checkItem := range m.list.Items() {
									if checkIt, ok := checkItem.(item); ok {
										if checkIt.title != it.title && strings.EqualFold(checkIt.keyShortcut, newShortcut) {
											m.lastAction = fmt.Sprintf("Shortcut '%s' already used by '%s'", newShortcut, checkIt.title)
											return m, nil
										}
									}
								}
							}

							it.keyShortcut = newShortcut
							m.list.SetItem(i, it)
							if err := saveScripts(listItemsToStored(m.list.Items())); err != nil {
								m.lastAction = "Updated shortcut, but failed to save: " + err.Error()
							} else {
								if newShortcut == "" {
									m.lastAction = fmt.Sprintf("Removed shortcut from '%s'", it.title)
								} else {
									m.lastAction = fmt.Sprintf("Updated shortcut to '%s' for '%s'", newShortcut, it.title)
								}
							}
							m.shortcutEditMode = false
							break
						}
					}
				}
				return m, nil
			}

			// Update focused input
			if m.shortcutEditFocus == 0 {
				m.shortcutEditInput, cmd = m.shortcutEditInput.Update(msg)
			} else {
				m.shortcutKeyInput, cmd = m.shortcutKeyInput.Update(msg)
			}
			return m, cmd
		}

		if m.editMode {
			switch msg.String() {
			case "up", "shift+tab":
				m.editSelection--
				if m.editSelection < 0 {
					m.editSelection = 4 // Cycle to the last element
				}
			case "down", "tab":
				m.editSelection++
				if m.editSelection > 4 {
					m.editSelection = 0 // Cycle to the first element
				}
			case "enter":
				if m.editSelection <= 2 { // One of the text inputs
					// Save logic
					if idx := m.list.Index(); idx >= 0 {
						it, ok := m.list.SelectedItem().(item)
						if !ok {
							return m, nil
						}
						newTitle := strings.TrimSpace(m.editInputs[0].Value())
						newPath := strings.TrimSpace(m.editInputs[1].Value())
						newKeyShortcut := strings.TrimSpace(m.editKeyShortcutInput.Value())

						if newTitle != "" && newPath != "" {
							updatedItem := item{
								title:       newTitle,
								path:        newPath,
								sudo:        it.sudo,
								setProxy:    it.setProxy,
								keyShortcut: newKeyShortcut,
							}
							m.list.SetItem(idx, updatedItem)
							if err := saveScripts(listItemsToStored(m.list.Items())); err != nil {
								m.lastAction = "Updated script, but failed to save: " + err.Error()
							} else {
								m.lastAction = "Updated script: " + newTitle
							}
							m.editMode = false
						} else {
							m.lastAction = "Both name and path must be provided"
						}
					}
				} else { // One of the toggles
					if idx := m.list.Index(); idx >= 0 {
						it, ok := m.list.SelectedItem().(item)
						if !ok {
							return m, nil
						}
						if m.editSelection == 3 { // Sudo
							it.sudo = !it.sudo
						} else if m.editSelection == 4 { // Proxy
							it.setProxy = !it.setProxy
						}
						m.list.SetItem(idx, it)
						if err := saveScripts(listItemsToStored(m.list.Items())); err != nil {
							m.lastAction = "Updated option, but failed to save: " + err.Error()
						} else {
							m.lastAction = "Updated: " + it.title
						}
					}
				}
			case "esc":
				m.editMode = false
				m.lastAction = "Exited edit mode"
			}

			// Update focus of inputs
			for i := range m.editInputs {
				if m.editSelection == i {
					m.editInputs[i].Focus()
				} else {
					m.editInputs[i].Blur()
				}
			}
			if m.editSelection == 2 {
				m.editKeyShortcutInput.Focus()
			} else {
				m.editKeyShortcutInput.Blur()
			}

			// Update text inputs if they are focused
			var cmds []tea.Cmd
			if m.editSelection >= 0 && m.editSelection <= 1 {
				var cmd tea.Cmd
				m.editInputs[m.editSelection], cmd = m.editInputs[m.editSelection].Update(msg)
				cmds = append(cmds, cmd)
			} else if m.editSelection == 2 {
				var cmd tea.Cmd
				m.editKeyShortcutInput, cmd = m.editKeyShortcutInput.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		if m.noteOpen {
			// Handle category list view
			if m.noteCategoryListOpen {
				switch msg.String() {
				case "esc":
					m.noteCategoryListOpen = false
					m.lastAction = "Category list closed"
					return m, nil
				case "enter":
					// Select category and load its content
					if nc, ok := m.noteCategoryList.SelectedItem().(noteCategory); ok {
						// Save current note to current category first
						if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
							m.noteCategories[m.currentCategoryIndex].content = m.noteInput.Value()
						}
						// Find and switch to selected category
						for i, cat := range m.noteCategories {
							if cat.name == nc.name {
								m.currentCategoryIndex = i
								m.noteInput.SetValue(cat.content)
								m.noteLastSaved = cat.content
								m.lastAction = fmt.Sprintf("Switched to category: %s", cat.name)
								break
							}
						}
						m.noteCategoryListOpen = false

						// Update border color
						m.updateNoteTextareaBorderColor()

						// Save categories
						storedCats := make([]storedNoteCategory, len(m.noteCategories))
						for i, cat := range m.noteCategories {
							storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content, Color: cat.color}
						}
						_ = saveNoteCategories(storedCats)
					}
					return m, nil
				case "a", "A":
					// Add new category
					m.noteCategoryListOpen = false
					m.noteCategoryAddMode = true
					m.noteCategoryNameInput.Focus()
					m.lastAction = "Add new category"
					return m, nil
				case "d", "D":
					// Delete selected category
					if nc, ok := m.noteCategoryList.SelectedItem().(noteCategory); ok {
						// Find and remove the category
						for i, cat := range m.noteCategories {
							if cat.name == nc.name {
								// Remove from slice
								m.noteCategories = append(m.noteCategories[:i], m.noteCategories[i+1:]...)

								// Update list
								items := make([]list.Item, len(m.noteCategories))
								for j, c := range m.noteCategories {
									items[j] = c
								}
								m.noteCategoryList.SetItems(items)

								// Adjust current index if needed
								if m.currentCategoryIndex == i {
									m.currentCategoryIndex = -1
									m.noteInput.SetValue("")
								} else if m.currentCategoryIndex > i {
									m.currentCategoryIndex--
								}

								// Save categories
								storedCats := make([]storedNoteCategory, len(m.noteCategories))
								for j, c := range m.noteCategories {
									storedCats[j] = storedNoteCategory{Name: c.name, Content: c.content, Color: c.color}
								}
								_ = saveNoteCategories(storedCats)

								m.lastAction = fmt.Sprintf("Deleted category: %s", nc.name)
								break
							}
						}
					}
					return m, nil
				case "r", "R", "e", "E":
					// Rename/Edit selected category
					if nc, ok := m.noteCategoryList.SelectedItem().(noteCategory); ok {
						m.noteCategoryListOpen = false
						m.noteCategoryEditMode = true
						m.noteCategoryEditInput.SetValue(nc.name)
						m.noteCategoryColorInput.SetValue(nc.color)
						m.noteCategoryEditInput.Focus()
						m.lastAction = "Edit category"
						return m, nil
					}
					return m, nil
				}
				m.noteCategoryList, cmd = m.noteCategoryList.Update(msg)
				return m, cmd
			}

			// Handle category creation mode
			if m.noteCategoryAddMode {
				switch msg.String() {
				case "esc":
					m.noteCategoryAddMode = false
					m.noteCategoryNameInput.SetValue("")
					m.noteCategoryNameInput.Blur()
					m.lastAction = "Category creation cancelled"
					return m, nil
				case "enter":
					name := strings.TrimSpace(m.noteCategoryNameInput.Value())
					if name == "" {
						m.lastAction = "Category name cannot be empty"
						return m, nil
					}
					// Check if category already exists
					for _, cat := range m.noteCategories {
						if cat.name == name {
							m.lastAction = "Category already exists: " + name
							return m, nil
						}
					}
					// Create new category with current note content
					newCategory := noteCategory{name: name, content: m.noteInput.Value()}
					m.noteCategories = append(m.noteCategories, newCategory)
					m.currentCategoryIndex = len(m.noteCategories) - 1

					// Update category list
					items := make([]list.Item, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						items[i] = cat
					}
					m.noteCategoryList.SetItems(items)

					// Update border color
					m.updateNoteTextareaBorderColor()

					// Save categories
					storedCats := make([]storedNoteCategory, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content}
					}
					_ = saveNoteCategories(storedCats)

					m.noteCategoryAddMode = false
					m.noteCategoryNameInput.SetValue("")
					m.noteCategoryNameInput.Blur()
					m.lastAction = fmt.Sprintf("Created category: %s", name)
					return m, nil
				}
				m.noteCategoryNameInput, cmd = m.noteCategoryNameInput.Update(msg)
				return m, cmd
			}

			// Handle category edit menu
			if m.noteCategoryMenuOpen {
				switch msg.String() {
				case "esc":
					m.noteCategoryMenuOpen = false
					m.lastAction = "Category menu closed"
					return m, nil
				case "up", "k":
					m.noteCategoryMenuIndex--
					if m.noteCategoryMenuIndex < 0 {
						m.noteCategoryMenuIndex = 3 // 4 options: Rename, Change Color, Remove, Save
					}
					return m, nil
				case "down", "j":
					m.noteCategoryMenuIndex++
					if m.noteCategoryMenuIndex > 3 {
						m.noteCategoryMenuIndex = 0
					}
					return m, nil
				case "enter":
					// Execute selected menu action
					switch m.noteCategoryMenuIndex {
					case 0: // Rename
						m.noteCategoryMenuOpen = false
						m.noteCategoryEditMode = true
						m.noteCategoryEditInput.SetValue(m.noteCategories[m.currentCategoryIndex].name)
						m.noteCategoryColorInput.SetValue(m.noteCategories[m.currentCategoryIndex].color)
						m.noteCategoryEditInput.Focus()
						m.lastAction = "Rename mode: Tab to switch fields, Enter to save, Esc to cancel"
						return m, nil
					case 1: // Change Color
						m.noteCategoryMenuOpen = false
						m.noteCategoryEditMode = true
						m.noteCategoryEditInput.SetValue(m.noteCategories[m.currentCategoryIndex].name)
						m.noteCategoryColorInput.SetValue(m.noteCategories[m.currentCategoryIndex].color)
						m.noteCategoryColorInput.Focus() // Focus on color field
						m.lastAction = "Change color mode: Tab to switch fields, Enter to save, Esc to cancel"
						return m, nil
					case 2: // Remove
						m.noteCategoryMenuOpen = false
						// Delete current category
						if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
							deletedName := m.noteCategories[m.currentCategoryIndex].name
							// Remove category
							m.noteCategories = append(m.noteCategories[:m.currentCategoryIndex], m.noteCategories[m.currentCategoryIndex+1:]...)

							// Update category list
							items := make([]list.Item, len(m.noteCategories))
							for i, cat := range m.noteCategories {
								items[i] = cat
							}
							m.noteCategoryList.SetItems(items)

							// Adjust current index
							if len(m.noteCategories) > 0 {
								if m.currentCategoryIndex >= len(m.noteCategories) {
									m.currentCategoryIndex = len(m.noteCategories) - 1
								}
								m.noteInput.SetValue(m.noteCategories[m.currentCategoryIndex].content)
							} else {
								m.currentCategoryIndex = -1
								m.noteInput.SetValue("")
							}

							// Save categories
							storedCats := make([]storedNoteCategory, len(m.noteCategories))
							for i, cat := range m.noteCategories {
								storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content, Color: cat.color}
							}
							_ = saveNoteCategories(storedCats)

							m.lastAction = fmt.Sprintf("Deleted category: %s", deletedName)
						}
						return m, nil
					case 3: // Save to folder
						m.noteCategoryMenuOpen = false
						// Save to custom folder - open file dialog
						homeDir, _ := os.UserHomeDir()
						defaultPath := filepath.Join(homeDir, "Documents")

						// Use zenity for file dialog if available
						cmd := exec.Command("zenity", "--file-selection", "--directory", "--title=Select folder to save notes", "--filename="+defaultPath)
						output, err := cmd.Output()
						if err == nil {
							folder := strings.TrimSpace(string(output))
							if folder != "" {
								m.noteCategorySaveFolder = folder
								// Save all categories to the selected folder
								for _, cat := range m.noteCategories {
									fileName := filepath.Join(folder, cat.name+".txt")
									_ = os.WriteFile(fileName, []byte(cat.content), 0644)
								}
								m.lastAction = fmt.Sprintf("Saved %d categories to: %s", len(m.noteCategories), folder)
							} else {
								m.lastAction = "Save cancelled"
							}
						} else {
							// Fallback: save to Documents folder
							docsFolder := filepath.Join(homeDir, "Documents", "ixi-flower-notes")
							os.MkdirAll(docsFolder, 0755)
							for _, cat := range m.noteCategories {
								fileName := filepath.Join(docsFolder, cat.name+".txt")
								_ = os.WriteFile(fileName, []byte(cat.content), 0644)
							}
							m.lastAction = fmt.Sprintf("Saved to: %s (zenity not available)", docsFolder)
						}
						return m, nil
					}
					return m, nil
				}
				return m, nil
			}

			// Handle category edit mode
			if m.noteCategoryEditMode {
				switch msg.String() {
				case "esc":
					m.noteCategoryEditMode = false
					m.noteCategoryEditInput.SetValue("")
					m.noteCategoryColorInput.SetValue("")
					m.noteCategoryEditInput.Blur()
					m.noteCategoryColorInput.Blur()
					m.lastAction = "Edit cancelled"
					return m, nil
				case "tab":
					// Toggle between name and color input
					if m.noteCategoryEditInput.Focused() {
						m.noteCategoryEditInput.Blur()
						m.noteCategoryColorInput.Focus()
					} else {
						m.noteCategoryColorInput.Blur()
						m.noteCategoryEditInput.Focus()
					}
					return m, nil
				case "ctrl+d":
					// Delete current category
					if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
						deletedName := m.noteCategories[m.currentCategoryIndex].name
						// Remove category
						m.noteCategories = append(m.noteCategories[:m.currentCategoryIndex], m.noteCategories[m.currentCategoryIndex+1:]...)

						// Update category list
						items := make([]list.Item, len(m.noteCategories))
						for i, cat := range m.noteCategories {
							items[i] = cat
						}
						m.noteCategoryList.SetItems(items)

						// Adjust current index
						if len(m.noteCategories) > 0 {
							if m.currentCategoryIndex >= len(m.noteCategories) {
								m.currentCategoryIndex = len(m.noteCategories) - 1
							}
							m.noteInput.SetValue(m.noteCategories[m.currentCategoryIndex].content)
						} else {
							m.currentCategoryIndex = -1
							m.noteInput.SetValue("")
						}

						// Save categories
						storedCats := make([]storedNoteCategory, len(m.noteCategories))
						for i, cat := range m.noteCategories {
							storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content}
						}
						_ = saveNoteCategories(storedCats)

						m.noteCategoryEditMode = false
						m.noteCategoryEditInput.SetValue("")
						m.noteCategoryEditInput.Blur()
						m.lastAction = fmt.Sprintf("Deleted category: %s", deletedName)
						return m, nil
					}
				case "ctrl+s":
					// Save to custom folder - open file dialog
					homeDir, _ := os.UserHomeDir()
					defaultPath := filepath.Join(homeDir, "Documents")

					// Use zenity for file dialog if available
					cmd := exec.Command("zenity", "--file-selection", "--directory", "--title=Select folder to save notes", "--filename="+defaultPath)
					output, err := cmd.Output()
					if err == nil {
						folder := strings.TrimSpace(string(output))
						if folder != "" {
							m.noteCategorySaveFolder = folder
							// Save all categories to the selected folder
							for _, cat := range m.noteCategories {
								fileName := filepath.Join(folder, cat.name+".json")
								data := map[string]string{
									"name":    cat.name,
									"content": cat.content,
								}
								jsonData, _ := json.MarshalIndent(data, "", "  ")
								_ = os.WriteFile(fileName, jsonData, 0644)
							}
							m.lastAction = fmt.Sprintf("Saved %d categories to: %s", len(m.noteCategories), folder)
						} else {
							m.lastAction = "Save cancelled"
						}
					} else {
						// Fallback: save to Documents folder
						docsFolder := filepath.Join(homeDir, "Documents", "ixi-flower-notes")
						os.MkdirAll(docsFolder, 0755)
						for _, cat := range m.noteCategories {
							fileName := filepath.Join(docsFolder, cat.name+".json")
							data := map[string]string{
								"name":    cat.name,
								"content": cat.content,
							}
							jsonData, _ := json.MarshalIndent(data, "", "  ")
							_ = os.WriteFile(fileName, jsonData, 0644)
						}
						m.lastAction = fmt.Sprintf("Saved to: %s (zenity not available)", docsFolder)
					}
					m.noteCategoryEditMode = false
					m.noteCategoryEditInput.Blur()
					return m, nil
				case "enter":
					// Save changes (rename and/or color)
					newName := strings.TrimSpace(m.noteCategoryEditInput.Value())
					newColor := strings.TrimSpace(m.noteCategoryColorInput.Value())

					if newName == "" {
						m.lastAction = "Category name cannot be empty"
						return m, nil
					}
					// Check if new name already exists
					for i, cat := range m.noteCategories {
						if i != m.currentCategoryIndex && cat.name == newName {
							m.lastAction = "Category already exists: " + newName
							return m, nil
						}
					}
					// Update category
					oldName := m.noteCategories[m.currentCategoryIndex].name
					m.noteCategories[m.currentCategoryIndex].name = newName
					m.noteCategories[m.currentCategoryIndex].color = newColor

					// Update category list
					items := make([]list.Item, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						items[i] = cat
					}
					m.noteCategoryList.SetItems(items)

					// Update border color
					m.updateNoteTextareaBorderColor()

					// Save categories with color
					storedCats := make([]storedNoteCategory, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content, Color: cat.color}
					}
					_ = saveNoteCategories(storedCats)

					m.noteCategoryEditMode = false
					m.noteCategoryEditInput.SetValue("")
					m.noteCategoryColorInput.SetValue("")
					m.noteCategoryEditInput.Blur()
					m.noteCategoryColorInput.Blur()

					updateMsg := fmt.Sprintf("Updated: %s", newName)
					if newName != oldName {
						updateMsg = fmt.Sprintf("Renamed: %s → %s", oldName, newName)
					}
					if newColor != "" {
						updateMsg += fmt.Sprintf(" (color: %s)", newColor)
					}
					m.lastAction = updateMsg
					return m, nil
				}
				// Update the focused input
				if m.noteCategoryEditInput.Focused() {
					m.noteCategoryEditInput, cmd = m.noteCategoryEditInput.Update(msg)
				} else if m.noteCategoryColorInput.Focused() {
					m.noteCategoryColorInput, cmd = m.noteCategoryColorInput.Update(msg)
				}
				return m, cmd
			}

			// Normal note editing mode
			switch msg.String() {
			case "esc":
				// Save current category before closing
				if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
					m.noteCategories[m.currentCategoryIndex].content = m.noteInput.Value()
					storedCats := make([]storedNoteCategory, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content}
					}
					_ = saveNoteCategories(storedCats)
				}
				m.noteOpen = false
				m.noteFullscreen = false
				m.noteInput.Blur()

			case "ctrl+f":
				m.noteFullscreen = !m.noteFullscreen
				m.noteInput.SetWidth(max(10, m.width-4))
				if m.noteFullscreen {
					logoHeight := 1
					if m.width >= 80 {
						logoHeight = lipgloss.Height(logoText)
					}
					topHeight := logoHeight + 3
					availableHeight := m.height - topHeight - 4
					if !m.footerHidden {
						availableHeight -= 1
					}
					m.noteInput.SetHeight(max(3, availableHeight))
				} else {
					m.noteInput.SetHeight(5)
				}
				return m, nil

			case "ctrl+l":
				// Show category list
				m.noteCategoryListOpen = true
				m.lastAction = "Category list (Enter: select, Esc: close)"
				return m, nil

			case "ctrl+g":
				// Create new category
				m.noteCategoryAddMode = true
				m.noteCategoryNameInput.Focus()
				m.lastAction = "Enter category name (Enter: save, Esc: cancel)"
				return m, nil

			case "ctrl+n":
				// Next category
				if len(m.noteCategories) > 0 {
					// Save current category
					if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
						m.noteCategories[m.currentCategoryIndex].content = m.noteInput.Value()
					}
					// Move to next
					m.currentCategoryIndex++
					if m.currentCategoryIndex >= len(m.noteCategories) {
						m.currentCategoryIndex = 0
					}
					// Load next category
					m.noteInput.SetValue(m.noteCategories[m.currentCategoryIndex].content)
					m.noteLastSaved = m.noteCategories[m.currentCategoryIndex].content
					m.lastAction = fmt.Sprintf("Category: %s (%d/%d)", m.noteCategories[m.currentCategoryIndex].name, m.currentCategoryIndex+1, len(m.noteCategories))

					// Update border color
					m.updateNoteTextareaBorderColor()

					// Save categories
					storedCats := make([]storedNoteCategory, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content}
					}
					_ = saveNoteCategories(storedCats)
				} else {
					m.lastAction = "No categories. Create one with Ctrl+G"
				}
				return m, nil

			case "ctrl+p":
				// Previous category
				if len(m.noteCategories) > 0 {
					// Save current category
					if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
						m.noteCategories[m.currentCategoryIndex].content = m.noteInput.Value()
					}
					// Move to previous
					m.currentCategoryIndex--
					if m.currentCategoryIndex < 0 {
						m.currentCategoryIndex = len(m.noteCategories) - 1
					}
					// Load previous category
					m.noteInput.SetValue(m.noteCategories[m.currentCategoryIndex].content)
					m.noteLastSaved = m.noteCategories[m.currentCategoryIndex].content
					m.lastAction = fmt.Sprintf("Category: %s (%d/%d)", m.noteCategories[m.currentCategoryIndex].name, m.currentCategoryIndex+1, len(m.noteCategories))

					// Update border color
					m.updateNoteTextareaBorderColor()

					// Save categories
					storedCats := make([]storedNoteCategory, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content}
					}
					_ = saveNoteCategories(storedCats)
				} else {
					m.lastAction = "No categories. Create one with Ctrl+G"
				}
				return m, nil

			case "ctrl+s":
				// Save current note
				if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
					m.noteCategories[m.currentCategoryIndex].content = m.noteInput.Value()
					// Save all categories
					storedCats := make([]storedNoteCategory, len(m.noteCategories))
					for i, cat := range m.noteCategories {
						storedCats[i] = storedNoteCategory{Name: cat.name, Content: cat.content, Color: cat.color}
					}
					err := saveNoteCategories(storedCats)
					if err == nil {
						m.noteHasUnsavedChanges = false
						m.lastAction = "Note saved ✓"
					} else {
						m.lastAction = fmt.Sprintf("Save failed: %v", err)
					}
				} else {
					m.lastAction = "No category selected. Create one with Ctrl+G first"
				}
				return m, nil

			case "ctrl+e":
				// Open edit menu for current category
				if len(m.noteCategories) > 0 {
					// Auto-select first category if none selected
					if m.currentCategoryIndex < 0 || m.currentCategoryIndex >= len(m.noteCategories) {
						m.currentCategoryIndex = 0
						m.noteInput.SetValue(m.noteCategories[0].content)
						m.noteLastSaved = m.noteCategories[0].content
					}
					m.noteCategoryMenuOpen = true
					m.noteCategoryMenuIndex = 0
					m.lastAction = "Select action: ↑/↓ to navigate, Enter to select, Esc to cancel"
					return m, nil
				} else {
					m.lastAction = "No categories. Create one with Ctrl+G first"
					return m, nil
				}
			}

			if msg.String() == "ctrl+t" {
				// make line task/todo — Ctrl+T
				lineIdx := m.noteInput.Line()
				val := m.noteInput.Value()
				lines := strings.Split(val, "\n")
				if lineIdx >= 0 && lineIdx < len(lines) {
					lines[lineIdx] = toggleTaskLine(lines[lineIdx])
					newVal := strings.Join(lines, "\n")
					m.noteInput.SetValue(newVal)
					if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
						m.noteCategories[m.currentCategoryIndex].content = newVal
					}
					m.noteSaveSeq++
					m.noteHasUnsavedChanges = true
					// keep cursor on same line
					m.lastAction = "Task toggled — line " + strconv.Itoa(lineIdx+1) + " (Ctrl+T)"
					return m, tea.Batch(debounceNoteSave(m.noteSaveSeq), saveNotesCmd(newVal))
				}
				return m, nil
			}
			prev := m.noteInput.Value()
			var noteCmd tea.Cmd
			m.noteInput, noteCmd = m.noteInput.Update(msg)
			cur := m.noteInput.Value()
			if cur != prev {
				m.noteSaveSeq++
				m.noteHasUnsavedChanges = true // Mark as having unsaved changes
				// Also update current category if one is selected
				if m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
					m.noteCategories[m.currentCategoryIndex].content = cur
				}
				return m, tea.Batch(noteCmd, debounceNoteSave(m.noteSaveSeq))
			}
			return m, noteCmd
		}

		if m.dockerHubActive {
			// when input is focused, only esc/enter/tab should be shortcuts — letters like s/p/r/d must be typed
			if m.dockerHubInput.Focused() {
				switch msg.String() {
				case "esc":
					m.dockerHubInput.Blur()
					if m.dockerHubList.Items() != nil && len(m.dockerHubList.Items()) > 0 {
						return m, nil
					}
					m.dockerHubActive = false
					m.dockerHubSearching = false
					m.lastAction = "Closed Docker Hub search"
					return m, nil
				case "enter":
					q := strings.TrimSpace(m.dockerHubInput.Value())
					if q == "" {
						m.lastAction = "Enter a search term for Docker Hub"
						return m, nil
					}
					m.hubSearchSeq++
					m.dockerHubSearching = true
					m.dockerHubInput.Blur()
					m.lastAction = "Searching Hub for " + q + "..."
					return m, searchDockerHubCmd(q)
				case "tab", "/":
					m.dockerHubInput.Blur()
					return m, nil
				}
				// any other key when focused goes to input (so s/p/r/d can be typed)
			} else {
				switch msg.String() {
				case "esc":
					m.dockerHubActive = false
					m.dockerHubInput.Blur()
					m.dockerHubSearching = false
					m.lastAction = "Closed Docker Hub search"
					return m, nil
				case "enter", "p", "P", "d", "D":
					if m.dockerHubSearching {
						return m, nil
					}
					if it, ok := m.dockerHubList.SelectedItem().(dockerHubImage); ok {
						m.lastAction = "Pulling " + it.title + "..."
						return m, dockerHubPullCmd(it.title)
					}
					return m, nil
				case "r", "R":
					if it, ok := m.dockerHubList.SelectedItem().(dockerHubImage); ok {
						m.dockerHubActive = false
						m.dockerHubInput.Blur()
						m.imageRunDialogOpen = true
						m.imageRunImageRef = it.title
						m.imageRunInputs[0].SetValue("")
						m.imageRunInputs[1].SetValue("")
						m.imageRunInputs[2].SetValue("")
						m.imageRunInputs[3].SetValue("")
						m.imageRunFocus = 0
						m.imageRunInputs[0].Focus()
						m.imageRunInputs[1].Blur()
						m.imageRunInputs[2].Blur()
						m.imageRunInputs[3].Blur()
						m.lastAction = "Run " + it.title + " (config ports/volumes, enter to run)"
						return m, nil
					}
					return m, nil
				case "tab", "/":
					m.dockerHubInput.Focus()
					return m, nil
				}
			}
			// update hub input if focused, otherwise update list
			if m.dockerHubInput.Focused() {
				prev := m.dockerHubInput.Value()
				var inputCmd tea.Cmd
				m.dockerHubInput, inputCmd = m.dockerHubInput.Update(msg)
				newQ := m.dockerHubInput.Value()
				if newQ != prev {
					if strings.TrimSpace(newQ) == "" {
						m.dockerHubList.SetItems([]list.Item{})
						m.dockerHubQuery = ""
						m.dockerHubSearching = false
						return m, inputCmd
					}
					if len(strings.TrimSpace(newQ)) >= 2 {
						m.hubSearchSeq++
						return m, tea.Batch(inputCmd, debounceHubSearch(m.hubSearchSeq, newQ))
					}
				}
				return m, inputCmd
			}
			// also allow typing to jump to search input
			if len(msg.String()) == 1 && strings.Contains("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_/.:", msg.String()) {
				m.dockerHubInput.Focus()
				prev := m.dockerHubInput.Value()
				var inputCmd tea.Cmd
				m.dockerHubInput, inputCmd = m.dockerHubInput.Update(msg)
				newQ := m.dockerHubInput.Value()
				if newQ != prev && len(strings.TrimSpace(newQ)) >= 2 {
					m.hubSearchSeq++
					return m, tea.Batch(inputCmd, debounceHubSearch(m.hubSearchSeq, newQ))
				}
				return m, inputCmd
			}
			m.dockerHubList, cmd = m.dockerHubList.Update(msg)
			return m, cmd
		}

		if m.dockerServicesActive {
			if m.searchFocused {
				switch msg.String() {
				case "esc":
					m.searchFocused = false
					m.searchInput.Blur()
					m.searchInput.SetValue("")
					m.dockerList.ResetFilter()
					return m, nil

				case "enter":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Restarting " + svc.title
						return m, dockerServiceActionCmd(svc.title, "restart")
					}
					return m, nil

				case "r":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Restarting " + svc.title
						return m, dockerServiceActionCmd(svc.title, "restart")
					}
					return m, nil

				case "x":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Stopping " + svc.title
						return m, dockerServiceActionCmd(svc.title, "stop")
					}
					return m, nil

				case "g":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Starting " + svc.title
						return m, dockerServiceActionCmd(svc.title, "start")
					}
					return m, nil
				}

				var listCmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				filterText := m.searchInput.Value()
				m.dockerList.SetFilterText(filterText)

				// Store the current selection before updating the list
				oldSelectedItem := m.dockerList.SelectedItem()

				m.dockerList, listCmd = m.dockerList.Update(msg)

				// If there was a selection and it's gone after filtering, try to restore it
				if oldSelectedItem != nil && m.dockerList.SelectedItem() == nil && len(m.dockerList.Items()) > 0 {
					// If the old selected item is still in the visible items, try to select it
					visibleItems := m.dockerList.VisibleItems()
					for i, item := range visibleItems {
						if item == oldSelectedItem {
							m.dockerList.Select(i)
							break
						}
					}
					// If we still don't have a selection and there are visible items, select the first one
					if m.dockerList.SelectedItem() == nil && len(visibleItems) > 0 {
						m.dockerList.Select(0)
					}
				}

				return m, tea.Batch(cmd, listCmd)
			} else {
				switch msg.String() {
		case "D", "d", "esc":
				m.dockerServicesActive = false
				m.lastAction = "Closed docker services"
				return m, nil
			case "i", "I":
				m.dockerServicesActive = false
				m.dockerImagesActive = true
				m.lastAction = "Docker images opened"
				m.dockerImageList.ResetSelected()
				return m, fetchDockerImagesCmd()
				case "s", "S", "/":
					m.searchFocused = true
					m.searchInput.Focus()
					return m, nil
				case "h", "H":
					m.dockerHubActive = true
					m.dockerHubInput.Focus()
					m.dockerHubInput.SetValue("")
					m.dockerHubList.SetItems([]list.Item{})
					m.lastAction = "Docker Hub search: type query + enter to search, enter to pull, esc to close | / or s = local filter"
					return m, nil
				case "enter":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Restarting " + svc.title
						return m, dockerServiceActionCmd(svc.title, "restart")
					}
					return m, nil
				case "r":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Restarting " + svc.title
						return m, dockerServiceActionCmd(svc.title, "restart")
					}
					return m, nil
				case "x":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Stopping " + svc.title
						return m, dockerServiceActionCmd(svc.title, "stop")
					}
					return m, nil
				case "g":
					if svc, ok := m.dockerList.SelectedItem().(dockerService); ok {
						m.lastAction = "Starting " + svc.title
						return m, dockerServiceActionCmd(svc.title, "start")
					}
					return m, nil
				}
				m.dockerList, cmd = m.dockerList.Update(msg)
				return m, cmd
			}
		}

		if m.imageRunDialogOpen {
			switch msg.String() {
			case "esc":
				m.imageRunDialogOpen = false
				m.lastAction = "Cancelled docker run"
				return m, nil
			case "tab", "down":
				m.imageRunInputs[m.imageRunFocus].Blur()
				m.imageRunFocus = (m.imageRunFocus + 1) % len(m.imageRunInputs)
				m.imageRunInputs[m.imageRunFocus].Focus()
				return m, nil
			case "shift+tab", "up":
				m.imageRunInputs[m.imageRunFocus].Blur()
				m.imageRunFocus = (m.imageRunFocus - 1 + len(m.imageRunInputs)) % len(m.imageRunInputs)
				m.imageRunInputs[m.imageRunFocus].Focus()
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.imageRunInputs[0].Value())
				ports := strings.TrimSpace(m.imageRunInputs[1].Value())
				volumes := strings.TrimSpace(m.imageRunInputs[2].Value())
				extra := strings.TrimSpace(m.imageRunInputs[3].Value())
				img := m.imageRunImageRef
				m.imageRunDialogOpen = false
				m.lastAction = "Running " + img + "..."
				return m, func() tea.Msg {
					cid, err := runDockerImageWithOptions(name, ports, volumes, extra, img)
					return dockerImageRunFinishedMsg{containerID: cid, err: err}
				}
			}
			var inputCmd tea.Cmd
			m.imageRunInputs[m.imageRunFocus], inputCmd = m.imageRunInputs[m.imageRunFocus].Update(msg)
			return m, inputCmd
		}

		if m.dockerImagesActive {
			if m.searchFocused {
				switch msg.String() {
				case "esc":
					m.searchFocused = false
					m.searchInput.Blur()
					m.searchInput.SetValue("")
					m.dockerImageList.ResetFilter()
					return m, nil
				case "enter", "r":
					if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
						m.searchFocused = false
						m.searchInput.Blur()
						m.imageRunDialogOpen = true
						m.imageRunImageRef = img.id
						m.imageRunInputs[0].SetValue("")
						m.imageRunInputs[1].SetValue("")
						m.imageRunInputs[2].SetValue("")
						m.imageRunInputs[3].SetValue("")
						m.imageRunFocus = 0
						m.imageRunInputs[0].Focus()
						m.imageRunInputs[1].Blur()
						m.imageRunInputs[2].Blur()
						m.imageRunInputs[3].Blur()
						m.lastAction = "Configure run options for " + img.title
					}
					return m, nil
				case "x":
					if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
						m.lastAction = "Removing " + img.title
						return m, dockerImageActionCmd(img.id, "remove")
					}
					return m, nil
				case "i":
					if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
						m.lastAction = "Inspecting " + img.title
						return m, dockerImageActionCmd(img.id, "inspect")
					}
					return m, nil
				case "e":
					if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
						m.searchFocused = false
						m.searchInput.Blur()
						containerName, err := findContainerFromImage(img.id)
						if err != nil {
							m.lastAction = err.Error()
						} else {
							m.lastAction = "Exec into " + containerName
							return m, execContainerCmd(containerName)
						}
					}
					return m, nil
				}
				var listCmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				filterText := m.searchInput.Value()
				m.dockerImageList.SetFilterText(filterText)
				oldSelectedItem := m.dockerImageList.SelectedItem()
				m.dockerImageList, listCmd = m.dockerImageList.Update(msg)
				if oldSelectedItem != nil && m.dockerImageList.SelectedItem() == nil && len(m.dockerImageList.Items()) > 0 {
					visibleItems := m.dockerImageList.VisibleItems()
					for i, item := range visibleItems {
						if item == oldSelectedItem {
							m.dockerImageList.Select(i)
							break
						}
					}
					if m.dockerImageList.SelectedItem() == nil && len(visibleItems) > 0 {
						m.dockerImageList.Select(0)
					}
				}
				return m, tea.Batch(cmd, listCmd)
			} else {
				switch msg.String() {
				case "esc":
					m.dockerImagesActive = false
					m.lastAction = "Closed docker images"
					return m, nil
				case "d", "D":
					m.dockerImagesActive = false
					m.dockerServicesActive = true
					m.lastAction = "Docker services opened"
					m.dockerList.ResetSelected()
					return m, fetchDockerServicesCmd()
				case "s", "S", "/":
					m.searchFocused = true
					m.searchInput.Focus()
					return m, nil
				case "h", "H":
					m.dockerHubActive = true
					m.dockerHubInput.Focus()
					m.dockerHubInput.SetValue("")
					m.dockerHubList.SetItems([]list.Item{})
					m.lastAction = "Docker Hub search: type query + enter to search, enter to pull, esc to close"
					return m, nil
			case "enter", "r":
				if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
					m.imageRunDialogOpen = true
					m.imageRunImageRef = img.id
					m.imageRunInputs[0].SetValue("")
					m.imageRunInputs[1].SetValue("")
					m.imageRunInputs[2].SetValue("")
					m.imageRunInputs[3].SetValue("")
					m.imageRunFocus = 0
					m.imageRunInputs[0].Focus()
					m.imageRunInputs[1].Blur()
					m.imageRunInputs[2].Blur()
					m.imageRunInputs[3].Blur()
					m.lastAction = "Configure run options for " + img.title
				}
				return m, nil
			case "x":
				if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
					m.lastAction = "Removing " + img.title
					return m, dockerImageActionCmd(img.id, "remove")
				}
				return m, nil
			case "i":
				if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
					m.lastAction = "Inspecting " + img.title
					return m, dockerImageActionCmd(img.id, "inspect")
				}
				return m, nil
			case "e":
				if img, ok := m.dockerImageList.SelectedItem().(dockerImage); ok {
					containerName, err := findContainerFromImage(img.id)
					if err != nil {
						m.lastAction = err.Error()
					} else {
						m.lastAction = "Exec into " + containerName
						return m, execContainerCmd(containerName)
					}
				}
				return m, nil
			case "p":
				m.lastAction = "Pruning unused images"
				return m, dockerImageActionCmd("", "prune")
				}
				m.dockerImageList, cmd = m.dockerImageList.Update(msg)
				return m, cmd
			}
		}

		if m.bookmarksActive {
			if m.searchFocused {
				switch msg.String() {
				case "esc":
					m.searchFocused = false
					m.searchInput.Blur()
					m.searchInput.SetValue("")
					m.bookmarkList.ResetFilter()
					return m, nil

				case "enter":
					if b, ok := m.bookmarkList.SelectedItem().(bookmark); ok {
						m.lastAction = "Opening: " + b.title
						m.searchFocused = false
						m.searchInput.Blur()
						m.searchInput.SetValue("")
						m.bookmarkList.ResetFilter()
						return m, openUrlCmd(b.title, b.url)
					}
					return m, nil
				}

				// Handle search input when focused
				var listCmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.bookmarkList.SetFilterText(m.searchInput.Value())
				m.bookmarkList, listCmd = m.bookmarkList.Update(msg)
				return m, tea.Batch(cmd, listCmd)
			} else {
				switch msg.String() {
				case "b", "B", "esc":
					m.bookmarksActive = false
					m.lastAction = "Closed bookmarks"
					return m, nil

				case "s", "S":
					m.searchFocused = true
					m.searchInput.Focus()
					return m, nil

				case "enter":
					if b, ok := m.bookmarkList.SelectedItem().(bookmark); ok {
						m.lastAction = "Opening: " + b.title
						return m, openUrlCmd(b.title, b.url)
					}
					return m, nil
				}
				m.bookmarkList, cmd = m.bookmarkList.Update(msg)
				return m, cmd
			}
		}

		if m.projectsActive {
			if m.projectAddMode {
				switch msg.String() {
				case "esc":
					m.projectAddMode = false
					m.lastAction = "Cancelled project add"
					for i := range m.projectInputs {
						m.projectInputs[i].Blur()
					}
					return m, nil

				case "tab", "shift+tab", "up", "down":
					if msg.String() == "up" || msg.String() == "shift+tab" {
						m.projectFocus--
					} else {
						m.projectFocus++
					}
					if m.projectFocus < 0 {
						m.projectFocus = len(m.projectInputs) - 1
					} else if m.projectFocus >= len(m.projectInputs) {
						m.projectFocus = 0
					}
					for i := range m.projectInputs {
						if i == m.projectFocus {
							m.projectInputs[i].Focus()
						} else {
							m.projectInputs[i].Blur()
						}
					}
					return m, nil

				case "enter":
					name := strings.TrimSpace(m.projectInputs[0].Value())
					path := strings.TrimSpace(m.projectInputs[1].Value())
					if name == "" || path == "" {
						m.lastAction = "Please enter both project name + path"
						return m, nil
					}

					// insert after pinned so pinned stay at top
					pinnedCount := 0
					for _, it := range m.projectsList.Items() {
						if p, ok := it.(project); ok && p.pinned {
							pinnedCount++
						}
					}
					m.projectsList.InsertItem(pinnedCount, project{name: name, path: path})
					m.projectsList.Select(pinnedCount)
					if err := saveProjects(listProjectsToStored(m.projectsList.Items())); err != nil {
						m.lastAction = "Project added, but failed to save: " + err.Error()
					} else {
						m.lastAction = "Project added: " + name + " (after pinned)"
					}
					m.projectInputs[0].SetValue("")
					m.projectInputs[1].SetValue("")
					m.projectInputs[0].Blur()
					m.projectInputs[1].Blur()
					m.projectAddMode = false
					return m, nil
				}

				m.projectInputs[m.projectFocus], cmd = m.projectInputs[m.projectFocus].Update(msg)
				return m, cmd
			}

			if m.searchFocused {
				switch msg.String() {
				case "esc":
					m.searchFocused = false
					m.searchInput.Blur()
					m.searchInput.SetValue("")
					m.projectsList.ResetFilter()
					return m, nil

				case "enter":
					// Handle enter in search mode to select item
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening project: " + p.name
						m.searchFocused = false
						m.searchInput.Blur()
						m.searchInput.SetValue("")
						m.projectsList.ResetFilter()
						return m, openProjectInNewTerminalCmd(p.name, p.path, "nvim")
					}
					return m, nil
				}

				// Handle search input when focused
				var listCmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.projectsList.SetFilterText(m.searchInput.Value())
				m.projectsList, listCmd = m.projectsList.Update(msg)
				return m, tea.Batch(cmd, listCmd)
			} else {
				// Handle commands when NOT in search mode
				switch msg.String() {
				case "esc":
					m.projectsActive = false
					m.projectAddMode = false
					m.projectRemove = false
					m.lastAction = "Closed projects"
					return m, nil

				case "a", "A":
					m.projectAddMode = true
					m.projectRemove = false
					m.projectFocus = 0
					m.projectInputs[0].Focus()
					m.projectInputs[1].Blur()
					m.lastAction = "Add project (Tab switch, Enter save, Esc cancel)"
					return m, nil

				case "s":
					m.searchFocused = true
					m.searchInput.Focus()
					return m, nil

				case "d", "D", "backspace":
					idx := m.projectsList.Index()
					if idx >= 0 && idx < len(m.projectsList.Items()) {
						if p, ok := m.projectsList.SelectedItem().(project); ok {
							m.projectsList.RemoveItem(idx)
							if err := saveProjects(listProjectsToStored(m.projectsList.Items())); err != nil {
								m.lastAction = "Project removed, but failed to save: " + err.Error()
							} else {
								m.lastAction = "Project removed: " + p.name
							}
						}
					}
					return m, nil

				case "n", "N":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening nvim: " + p.name
						return m, openProjectInNewTerminalCmd(p.name, p.path, "nvim")
					}
					return m, nil

				case "t", "T":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening terminal: " + p.name
						return m, openProjectTerminalCmd(p.name, p.path)
					}
					return m, nil

				case "f", "F":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening file manager: " + p.name
						return m, openProjectInFileManagerCmd(p.name, p.path)
					}
					return m, nil

				case "c", "C":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening VS Code: " + p.name
						return m, openProjectInVSCodeCmd(p.name, p.path)
					}
					return m, nil

				case "enter", " ":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening terminal: " + p.name + " (↑↓ to select, enter=terminal)"
						return m, openProjectTerminalCmd(p.name, p.path)
					}
					return m, nil

				case "o", "O":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening opencode: " + p.name
						return m, openProjectAICmd(p.name, p.path, "opencode")
					}
					return m, nil

				case "h", "H":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening hermes: " + p.name
						return m, openProjectAICmd(p.name, p.path, "hermes")
					}
					return m, nil

				case "l", "L":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening claude: " + p.name
						return m, openProjectAICmd(p.name, p.path, "claude")
					}
					return m, nil

				case "k", "K":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening codex: " + p.name
						return m, openProjectAICmd(p.name, p.path, "codex")
					}
					return m, nil

				case "m", "M":
					if p, ok := m.projectsList.SelectedItem().(project); ok {
						m.lastAction = "Opening ai-menu: " + p.name
						// ai-menu is built from /etc/ixi-tui-main/cmd/ai-menu
						aiMenuBin := "/etc/ixi-tui-main/ai-menu"
						if _, err := os.Stat(aiMenuBin); os.IsNotExist(err) {
							aiMenuBin = filepath.Join("/etc/ixi-tui-main", "cmd", "ai-menu", "ai-menu")
							if _, err := os.Stat(aiMenuBin); os.IsNotExist(err) {
								aiMenuBin = "ai-menu"
							}
						}
						return m, openProjectAICmd(p.name, p.path, aiMenuBin)
					}
					return m, nil

				case "p", "P":
					idx := m.projectsList.Index()
					if idx < 0 || idx >= len(m.projectsList.Items()) {
						return m, nil
					}
					if proj, ok := m.projectsList.SelectedItem().(project); ok {
						m.projectsList.RemoveItem(idx)
						proj.pinned = !proj.pinned
						// reinsert to keep pinned at top
						newPos := 0
						if proj.pinned {
							// insert before first unpinned
							for i, it := range m.projectsList.Items() {
								if pp, ok := it.(project); ok && !pp.pinned {
									newPos = i
									break
								}
								newPos = i + 1
							}
						} else {
							// unpinned: after last pinned
							pinnedCount := 0
							for _, it := range m.projectsList.Items() {
								if pp, ok := it.(project); ok && pp.pinned {
									pinnedCount++
								}
							}
							newPos = pinnedCount
						}
						if newPos < 0 {
							newPos = 0
						}
						if newPos > len(m.projectsList.Items()) {
							newPos = len(m.projectsList.Items())
						}
						m.projectsList.InsertItem(newPos, proj)
						m.projectsList.Select(newPos)
						_ = saveProjects(listProjectsToStored(m.projectsList.Items()))
						if proj.pinned {
							m.lastAction = "📌 Pinned: " + proj.name + " (now at top)"
						} else {
							m.lastAction = "Unpinned: " + proj.name
						}
					}
					return m, nil

				case "x", "X":
					m.lastAction = "Auto-discovering projects in /home/ixi_flower/Documents..."
					return m, autoDiscoverProjectsCmd()
				}
			}

			m.projectsList, cmd = m.projectsList.Update(msg)
			return m, cmd
		}

		if m.searchFocused {
			switch msg.String() {
			case "esc":
				m.searchFocused = false
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				m.list.ResetFilter()
				return m, nil

			case "enter":

				if it, ok := m.list.SelectedItem().(item); ok {
					m.lastAction = "Running (from search): " + it.title
					m.searchFocused = false
					m.searchInput.Blur()
					m.searchInput.SetValue("")
					m.list.ResetFilter()
					return m, runScriptCmd(it.title, it.path, it.sudo, it.setProxy)
				}
				return m, nil

			case "shift+enter", "ctrl+enter":

				if it, ok := m.list.SelectedItem().(item); ok {
					m.lastAction = "Opening (from search) in new terminal: " + it.title
					m.searchFocused = false
					m.searchInput.Blur()
					m.searchInput.SetValue("")
					m.list.ResetFilter()
					return m, openScriptInNewTerminalCmd(it.title, it.path, it.sudo, it.setProxy)
				}
				return m, nil
			}

			// Handle search input - all other keys go to search
			var listCmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.list.SetFilterText(m.searchInput.Value())
			m.list, listCmd = m.list.Update(msg)
			return m, tea.Batch(cmd, listCmd)
		}

		if m.addMode {
			switch msg.String() {
			case "esc":
				m.addMode = false
				m.editMode = false
				m.lastAction = "Cancelled add"
				for i := range m.addInputs {
					m.addInputs[i].Blur()
				}
				return m, nil

			case "tab", "shift+tab", "up", "down":
				if msg.String() == "up" || msg.String() == "shift+tab" {
					m.addFocus--
				} else {
					m.addFocus++
				}
				if m.addFocus < 0 {
					m.addFocus = len(m.addInputs) - 1
				} else if m.addFocus >= len(m.addInputs) {
					m.addFocus = 0
				}
				for i := range m.addInputs {
					if i == m.addFocus {
						m.addInputs[i].Focus()
					} else {
						m.addInputs[i].Blur()
					}
				}
				return m, nil

			case "enter":
				name := strings.TrimSpace(m.addInputs[0].Value())
				path := strings.TrimSpace(m.addInputs[1].Value())
				if name == "" || path == "" {
					m.lastAction = "Please enter both name + path"
					return m, nil
				}

				m.list.InsertItem(len(m.list.Items()), item{title: name, path: path, sudo: false, setProxy: false})
				if err := saveScripts(listItemsToStored(m.list.Items())); err != nil {
					m.lastAction = "Added, but failed to save: " + err.Error()
				} else {
					m.lastAction = "Added: " + name
				}
				m.addInputs[0].SetValue("")
				m.addInputs[1].SetValue("")
				m.addInputs[0].Blur()
				m.addInputs[1].Blur()
				m.addMode = false
				return m, nil
			}

			m.addInputs[m.addFocus], cmd = m.addInputs[m.addFocus].Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "ctrl+e":
			if m.lastError != nil {
				m.lastError = nil
				m.lastAction = "Error message cleared."
				m.lastErrorCleared = time.Now()
			}
			return m, nil

		case "esc":
			if m.scriptActive {
				m.scriptActive = false
				m.lastAction = "Scripts closed"
				return m, nil
			}
			// let other esc handlers below handle it

		case "q", "ctrl+c":

			if m.noteInput.Value() != m.noteLastSaved {
				return m, tea.Sequence(saveNotesCmd(m.noteInput.Value()), tea.Quit)
			}
			return m, tea.Quit

		case "d", "D":
			m.dockerServicesActive = !m.dockerServicesActive
			m.dockerImagesActive = false
			m.scriptActive = false
			if m.dockerServicesActive {
				m.bookmarksActive = false
				m.projectsActive = false
				m.lastAction = "Docker services opened (g:start x:stop r:restart i:images esc:back)"
				m.dockerList.ResetSelected()
				return m, fetchDockerServicesCmd()
			}
			return m, nil

		case "I":
			m.dockerImagesActive = !m.dockerImagesActive
			m.dockerServicesActive = false
			m.scriptActive = false
			if m.dockerImagesActive {
				m.bookmarksActive = false
				m.projectsActive = false
				m.lastAction = "Docker images opened (i:inspect r:run x:remove p:prune esc:back)"
				m.dockerImageList.ResetSelected()
				return m, fetchDockerImagesCmd()
			}
			return m, nil

		case "b", "B":
			m.bookmarksActive = !m.bookmarksActive
			if m.bookmarksActive {
				m.lastAction = "Bookmarks opened (Press 'b' or 'esc' to close)"
				m.bookmarkList.ResetSelected()
			} else {
				m.lastAction = "Bookmarks closed"
			}
			return m, nil

		case "n":
			m.scriptActive = false
			m.noteOpen = !m.noteOpen
			if m.noteOpen {
				m.noteFullscreen = false
				m.noteInput.SetHeight(5)
				m.noteInput.Focus()

				// Update border color when opening notes
				m.updateNoteTextareaBorderColor()

				m.searchFocused = false
				m.searchInput.Blur()
				m.addMode = false
				for i := range m.addInputs {
					m.addInputs[i].Blur()
				}
			} else {
				m.noteInput.Blur()
			}
			return m, nil

		case "t":
			m.lastAction = "Opening smassh... (exit to return)"
			return m, runSmasshCmd()

		case "T":
			m.lastAction = "Opening kitty terminal..."
			return m, openKittyTerminalCmd()

		case "h", "H":
			m.scriptActive = false
			m.htopActive = !m.htopActive
			if m.htopActive {
				m.lastAction = "htop activated"
				// Save htop preference
				_ = saveSettings(settings{
					MenuHidden:   m.menuHidden,
					FooterHidden: m.footerHidden,
					MenuWidth:    m.menuWidth,
					HtopActive:   m.htopActive,
				})
				return m, tea.Batch(runHtopCmd(), htopRefreshCmd())
			} else {
				m.lastAction = "htop deactivated"
				// Save htop preference
				_ = saveSettings(settings{
					MenuHidden:   m.menuHidden,
					FooterHidden: m.footerHidden,
					MenuWidth:    m.menuWidth,
					HtopActive:   m.htopActive,
				})
			}
			return m, nil

		case "k", "K":
			m.footerHidden = !m.footerHidden

			availableHeight := m.height - (lipgloss.Height(logoText) + 3) - 4
			if !m.footerHidden {
				availableHeight -= 1
			}
			if m.noteOpen {
				if m.noteFullscreen {
					m.noteInput.SetWidth(max(10, m.width-4))
					m.noteInput.SetHeight(max(3, availableHeight))
				} else {
					availableHeight -= 6
					if availableHeight < 5 {
						availableHeight = 5
					}
				}
			}

			listWidth := m.width - 4
			if !m.menuHidden {
				minMenu := 18
				maxMenu := max(minMenu, m.width-30)
				if m.menuWidth < minMenu {
					m.menuWidth = minMenu
				}
				if m.menuWidth > maxMenu {
					m.menuWidth = maxMenu
				}
				listWidth = m.width - m.menuWidth - 1 - 4
				if listWidth < 10 {
					listWidth = 10
				}
			}

			listHeight := availableHeight - 2
			if listHeight < 3 {
				listHeight = 3
			}
			m.searchInput.Width = max(10, listWidth-10)
			m.list.SetSize(listWidth, listHeight)
			m.bookmarkList.SetSize(listWidth, listHeight)
			m.projectsList.SetSize(listWidth, listHeight)
			return m, nil

		case "m":
			// Toggle menu visibility (lowercase m)
			m.menuHidden = !m.menuHidden

			listWidth := m.width - 4
			if !m.menuHidden {
				minMenu := 18
				maxMenu := max(minMenu, m.width-30)
				if m.menuWidth < minMenu {
					m.menuWidth = minMenu
				}
				if m.menuWidth > maxMenu {
					m.menuWidth = maxMenu
				}
				listWidth = m.width - m.menuWidth - 1 - 4
				if listWidth < 10 {
					listWidth = 10
				}
			}
			m.searchInput.Width = max(10, listWidth-10)
			m.list.SetSize(listWidth, m.list.Height())
			m.bookmarkList.SetSize(listWidth, m.list.Height())
			m.projectsList.SetSize(listWidth, m.list.Height())
			return m, nil

		case "M":
			// Hide menu and center scripts (Shift+M / capital M)
			m.menuHidden = true

			// Use a narrower width for centered display (40% of screen or max 70 chars)
			fullWidth := m.width - 4
			centeredWidth := min(fullWidth*4/10, 70)
			if centeredWidth < 35 {
				centeredWidth = 35
			}

			m.searchInput.Width = max(10, centeredWidth-10)
			m.list.SetSize(centeredWidth, m.list.Height())
			m.bookmarkList.SetSize(centeredWidth, m.bookmarkList.Height())
			m.projectsList.SetSize(centeredWidth, m.projectsList.Height())
			m.dockerList.SetSize(centeredWidth, m.dockerList.Height())
			m.dockerImageList.SetSize(centeredWidth, m.dockerImageList.Height())
			return m, nil

		case "[", "]":

			if m.menuHidden {
				return m, nil
			}
			delta := 2
			if msg.String() == "[" {
				delta = -2
			}
			minMenu := 18
			maxMenu := max(minMenu, m.width-30)
			m.menuWidth += delta
			if m.menuWidth < minMenu {
				m.menuWidth = minMenu
			}
			if m.menuWidth > maxMenu {
				m.menuWidth = maxMenu
			}

			listWidth := m.width - m.menuWidth - 1 - 4
			if listWidth < 10 {
				listWidth = 10
			}
			m.searchInput.Width = max(10, listWidth-10)
			m.list.SetSize(listWidth, m.list.Height())
			m.bookmarkList.SetSize(listWidth, m.list.Height())
			m.projectsList.SetSize(listWidth, m.list.Height())
			return m, nil

		case "1":
			m.removeMode = false
			m.addMode = true
			m.editMode = false
			m.list.Title = "Available Scripts"
			m.lastAction = "Add Scripts"
			m.addFocus = 0
			for i := range m.addInputs {
				if i == m.addFocus {
					m.addInputs[i].Focus()
				} else {
					m.addInputs[i].Blur()
				}
			}
			return m, nil
		case "2":
			m.removeMode = true
			m.addMode = false
			m.editMode = false
			m.list.Title = "Remove Mode"
			m.lastAction = "Select a script to remove"
			return m, nil
		case "3":
			m.removeMode = false
			m.addMode = false
			m.editMode = true

		case "4":
			m.projectsActive = true
			m.lastAction = "Opened Projects"
			m.editSelection = 0
			m.list.Title = "Available Scripts"
			m.lastAction = "Action: Edit Scripts (Tab switch, Enter save, Esc cancel)"

			// Initialize edit inputs with current values if an item is selected
			if it, ok := m.list.SelectedItem().(item); ok {
				m.editInputs[0].SetValue(it.title)
				m.editInputs[1].SetValue(it.path)
				m.editKeyShortcutInput.SetValue(it.keyShortcut)
			} else {
				m.editInputs[0].SetValue("")
				m.editInputs[1].SetValue("")
				m.editKeyShortcutInput.SetValue("")
			}
			m.editInputs[0].Focus()

			return m, nil

		case "c", "C":
			// Handle clock activation
			m.scriptActive = false
			m.clockActive = !m.clockActive
			if m.clockActive {
				m.noteOpen = false
				m.dockerServicesActive = false
				m.dockerImagesActive = false
				m.bookmarksActive = false
				m.projectsActive = false
				m.htopActive = false
				m.lastAction = "Clock opened"
				m.clock.width = m.width // Pass current width/height to clock
				m.clock.height = m.height
				return m, m.clock.Init() // Initialize clock
			} else {
				m.lastAction = "Clock closed"
				// When clock is deactivated, stop any running timer to prevent background ticks
				m.clock.running = false
			}
			return m, nil

		case "X":
			m.scriptActive = false
			xrayProjectPath := "/etc/ixi-tui-main/xray"
			if _, err := os.Stat(xrayProjectPath); os.IsNotExist(err) {
				m.lastAction = "Xray project not found at: " + xrayProjectPath
				return m, nil
			}
			xrayBinary := filepath.Join(xrayProjectPath, "xray-app")
			if _, err := os.Stat(xrayBinary); os.IsNotExist(err) {
				m.lastAction = "xray-app binary not found at: " + xrayBinary
				return m, nil
			}
			cmd := exec.Command("./xray-app")
			cmd.Dir = xrayProjectPath
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			m.lastAction = "Opening Xray App..."
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return checkXrayStatusCmd()()
			})

		case "Y", "y":
			m.scriptActive = false
			torProjectPath := "/etc/ixi-tui-main/tor"
			if _, err := os.Stat(torProjectPath); os.IsNotExist(err) {
				m.lastAction = "Tor project not found at: " + torProjectPath
				return m, nil
			}
			torScript := filepath.Join(torProjectPath, "ip-changer.sh")
			if _, err := os.Stat(torScript); os.IsNotExist(err) {
				m.lastAction = "ip-changer.sh not found at: " + torScript
				return m, nil
			}
			// Run tor ip changer like xray — needs sudo for systemctl/torrc
			cmd := exec.Command("sudo", "bash", "ip-changer.sh")
			cmd.Dir = torProjectPath
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			m.lastAction = "Opening Tor IP Changer..."
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return checkTorStatusCmd()()
			})

		case "p", "P":

			if m.editMode {
				idx := m.list.Index()
				if idx < 0 || idx >= len(m.list.Items()) {
					return m, nil
				}
				it, ok := m.list.SelectedItem().(item)
				if !ok {
					return m, nil
				}
				it.setProxy = !it.setProxy
				m.list.SetItem(idx, it)
				if err := saveScripts(listItemsToStored(m.list.Items())); err != nil {
					m.lastAction = "Updated proxy flag, but failed to save: " + err.Error()
				} else {
					if it.setProxy {
						m.lastAction = "Enabled set-proxy for: " + it.title
					} else {
						m.lastAction = "Disabled set-proxy for: " + it.title
					}
				}
				return m, nil
			}

			// Open projects section when not in edit mode
			m.scriptActive = false
			m.projectsActive = true
			m.lastAction = "Opened Projects"
			return m, nil

		case "s", "S":
			// in panels, s = search; otherwise s = Scripts like other TUI options
			if m.projectsActive || m.dockerServicesActive || m.dockerImagesActive || m.dockerHubActive || m.bookmarksActive || m.noteOpen || m.htopActive || m.clockActive || m.scriptActive {
				if !m.searchFocused {
					m.searchFocused = true
					m.searchInput.Focus()
				}
				return m, nil
			}
			m.scriptActive = !m.scriptActive
			if m.scriptActive {
				m.lastAction = "Scripts opened (s to close, a to add)"
			} else {
				m.lastAction = "Scripts closed"
			}
			return m, nil

		case "ctrl+l":
			// Show shortcuts list (only when not in note mode)
			if !m.noteOpen {
				m.shortcutListOpen = true
				m.lastAction = "Shortcuts Manager (A:add R:rename D:delete E:edit Enter:close Esc:close)"
			}
			return m, nil

		case "shift+enter", "ctrl+enter", "o", "O":

			if it, ok := m.list.SelectedItem().(item); ok {
				m.lastAction = "Opening in new terminal: " + it.title
				return m, openScriptInNewTerminalCmd(it.title, it.path, it.sudo, it.setProxy)
			}
			return m, nil

		default:
			// Handle key shortcuts for scripts
			if !m.editMode && !m.addMode && !m.removeMode && !m.searchFocused {
				// Check if the pressed key matches any script's key shortcut
				for _, listItem := range m.list.Items() {
					existingItem, ok := listItem.(item)
					if ok && existingItem.keyShortcut != "" && strings.EqualFold(existingItem.keyShortcut, msg.String()) {
						m.lastAction = "Running script via shortcut: " + existingItem.title
						return m, runScriptCmd(existingItem.title, existingItem.path, existingItem.sudo, existingItem.setProxy)
					}
				}
			}
		}
	}

	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) renderDockerServicesGrid() string {
	// Get visible items from the list (this respects filtering)
	items := m.dockerList.VisibleItems()

	// Check if services are still loading by checking if the list is empty but we're in docker mode
	if len(items) == 0 {
		return "Loading Docker services..."
	}

	// Create a minimal table-style layout
	var result []string

	// Header
	header := lipgloss.NewStyle().
		Underline(true).
		Padding(0, 1).
		Render("NAME                    STATUS          RESPONSE TIME    ADDRESS")
	result = append(result, header)

	// Each service as a row
	for _, item := range items {
		service, ok := item.(dockerService)
		if !ok {
			continue
		}

		// Format each service in a compact row
		statusColor := "240" // gray
		if service.status == "online" {
			statusColor = "46" // green
		} else if service.status == "offline" || service.status == "exited" {
			statusColor = "9" // red
		}

		serviceRow := fmt.Sprintf("%-22s  %-12s  %-13s  %s",
			truncateString(service.title, 22),
			lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(service.status),
			service.responseTime,
			service.address,
		)

		result = append(result, serviceRow)
	}

	return lipgloss.JoinVertical(lipgloss.Left, result...)
}

// Helper function to truncate strings
func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

func noteShortcutsTable(width int) string {
	// vertical liney table — only for fullscreen, as requested
	if width < 30 {
		width = 30
	}
	tableWidth := width - 4
	if tableWidth > 40 {
		tableWidth = 40
	}
	if tableWidth < 24 {
		tableWidth = 24
	}
	inner := tableWidth - 2
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Align(lipgloss.Center)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true).Align(lipgloss.Center)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Align(lipgloss.Center)

	top := borderStyle.Render("┌" + strings.Repeat("─", inner) + "┐")
	mid := borderStyle.Render("├" + strings.Repeat("─", inner) + "┤")
	bot := borderStyle.Render("└" + strings.Repeat("─", inner) + "┘")
	sep := borderStyle.Render("│" + strings.Repeat("─", inner) + "│")

	rows := [][]string{
		{"Ctrl+G", "Create new category"},
		{"Ctrl+S", "Save note"},
		{"Ctrl+L", "List categories"},
		{"Enter", "Select category"},
		{"e", "Edit / Rename"},
		{"c", "Change color"},
		{"d", "Delete category"},
		{"↑↓", "Navigate"},
		{"Esc", "Close / Back"},
	}
	var b strings.Builder
	b.WriteString(top + "\n")
	b.WriteString(borderStyle.Render("│") + headerStyle.Width(inner).Render("Shortcuts") + borderStyle.Render("│") + "\n")
	b.WriteString(mid + "\n")
	for i, r := range rows {
		b.WriteString(borderStyle.Render("│") + keyStyle.Width(inner).Render(truncateString(r[0], inner)) + borderStyle.Render("│") + "\n")
		b.WriteString(borderStyle.Render("│") + valStyle.Width(inner).Render(truncateString(r[1], inner)) + borderStyle.Render("│") + "\n")
		if i < len(rows)-1 {
			b.WriteString(sep + "\n")
		}
	}
	b.WriteString(bot)
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true).Align(lipgloss.Center).Width(tableWidth).Render("─ Note Shortcuts (fullscreen) ─")
	return lipgloss.JoinVertical(lipgloss.Top, title, b.String())
}

func isTaskLine(line string) bool {
	trim := strings.TrimSpace(line)
	return strings.HasPrefix(trim, "- [ ]") || strings.HasPrefix(trim, "- [x]") || strings.HasPrefix(trim, "- [X]") || strings.HasPrefix(trim, "☐") || strings.HasPrefix(trim, "☑")
}

func toggleTaskLine(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	leading := line[:len(line)-len(trimmed)]
	trim := strings.TrimSpace(trimmed)
	if strings.HasPrefix(trim, "- [ ]") {
		return leading + "- [x] " + strings.TrimSpace(trim[5:])
	}
	if strings.HasPrefix(trim, "- [x]") || strings.HasPrefix(trim, "- [X]") {
		return leading + strings.TrimSpace(trim[5:])
	}
	if strings.HasPrefix(trim, "☐") {
		return leading + "☑ " + strings.TrimSpace(trim[2:])
	}
	if strings.HasPrefix(trim, "☑") {
		return leading + strings.TrimSpace(trim[2:])
	}
	if trim == "" {
		return leading + "- [ ] "
	}
	return leading + "- [ ] " + trim
}

func renderStickyNotes(m model, width int) string {
	if len(m.noteCategories) == 0 {
		return ""
	}
	// each category = 1 sticky note box
	var stickies []string
	for _, cat := range m.noteCategories {
		tasks := []string{}
		for _, line := range strings.Split(cat.content, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			// show tasks and plain lines as sticky content, but highlight tasks
			tasks = append(tasks, truncateString(strings.TrimSpace(line), 28))
			if len(tasks) >= 5 {
				break
			}
		}
		if len(tasks) == 0 {
			tasks = []string{lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(empty)")}
		} else {
			// style tasks with checkbox colors
			for i, t := range tasks {
				if strings.HasPrefix(strings.TrimSpace(t), "- [ ]") || strings.HasPrefix(strings.TrimSpace(t), "☐") {
					tasks[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Render("☐ "+strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "- [ ]")))
					if strings.HasPrefix(strings.TrimSpace(t), "☐") {
						tasks[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Render(t)
					}
				} else if strings.HasPrefix(strings.TrimSpace(t), "- [x]") || strings.HasPrefix(strings.TrimSpace(t), "☑") {
					tasks[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("☑ "+strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "- [x]")), "☑")))
				}
			}
		}
		c := cat.color
		if c == "" {
			c = "229"
		}
		title := lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Bold(true).Align(lipgloss.Center).Width(28).Render(truncateString(cat.name, 28))
		body := lipgloss.JoinVertical(lipgloss.Left, tasks...)
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c)).
			Background(lipgloss.Color("229")).
			Foreground(lipgloss.Color("236")).
			Padding(0, 1).
			Width(30).
			Render(lipgloss.JoinVertical(lipgloss.Top, title, body))
		stickies = append(stickies, box)
	}
	// join stickies horizontally, wrap if needed
	if len(stickies) == 0 {
		return ""
	}
	// simple horizontal join, let lipgloss handle wrapping via width
	row := lipgloss.JoinHorizontal(lipgloss.Top, stickies...)
	// if too wide, just vertical stack
	if lipgloss.Width(row) > width-4 {
		row = lipgloss.JoinVertical(lipgloss.Top, stickies...)
	}
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true).Render(fmt.Sprintf("📌 Sticky Notes — %d categories (Ctrl+T in notes to make task)", len(stickies)))
	return lipgloss.JoinVertical(lipgloss.Top, header, row)
}

func (m model) View() string {
	if m.clockActive {
		return m.clock.View()
	}

	var logo string
	logoContent := logoStyle.Render(logoText)
	showLogo := m.width >= 80

	if m.htopActive {
		htopTitle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true).
			Align(lipgloss.Center).
			Render("HTOP")

		htopContent := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Width(52).
			Render(m.htopOutput)

		topRightContent := lipgloss.JoinVertical(lipgloss.Top, htopTitle, htopContent)

		htopPanelWidth := 54
		if showLogo {
			if m.width >= lipgloss.Width(logoContent)+htopPanelWidth {
				logoContainerWidth := m.width - htopPanelWidth
				centeredLogo := lipgloss.PlaceHorizontal(logoContainerWidth, lipgloss.Center, logoContent)

				htopPanel := lipgloss.NewStyle().Width(htopPanelWidth).Height(lipgloss.Height(logoContent)).Render(topRightContent)

				logo = lipgloss.JoinHorizontal(lipgloss.Top, centeredLogo, htopPanel)
			} else {
				logo = lipgloss.JoinVertical(lipgloss.Top, logoStyle.Width(m.width).Render(logoText), topRightContent)
			}
		} else {
			logo = topRightContent
		}
	} else {
		if showLogo {
			logo = logoStyle.Width(m.width).Render(logoText)
		} else {
			logo = logoStyle.Width(m.width).Render("IXI-FLOWER-TUI")
		}
	}

	var menuView string

	if m.projectsActive {
		menuView += menuTitleStyle.Render("PROJECTS") + "\n"
		menuView += menuItemStyle.Render("↑↓ - Select (arrow)") + "\n"
		menuView += menuItemStyle.Render("enter/t - Terminal") + "\n"
		menuView += menuItemStyle.Render("p - Pin/unpin (top)") + "\n"
		menuView += menuItemStyle.Render("o - opencode") + "\n"
		menuView += menuItemStyle.Render("h - hermes") + "\n"
		menuView += menuItemStyle.Render("l - claude") + "\n"
		menuView += menuItemStyle.Render("k - codex") + "\n"
		menuView += menuItemStyle.Render("m - ai-menu") + "\n"
		menuView += menuItemStyle.Render("n - nvim") + "\n"
		menuView += menuItemStyle.Render("c - VS Code") + "\n"
		menuView += menuItemStyle.Render("f - File manager") + "\n"
		menuView += menuItemStyle.Render("a - Add (top)") + "\n"
		menuView += menuItemStyle.Render("d - Delete") + "\n"
		menuView += menuItemStyle.Render("s - Search") + "\n"
		menuView += menuItemStyle.Render("x - Auto-discover") + "\n"
		menuView += menuItemStyle.Render("esc - Back") + "\n"
		if m.projectAddMode {
			menuView += "\n" + menuTitleStyle.Render("ADD PROJECT") + "\n"
			menuView += m.projectInputs[0].View() + "\n"
			menuView += m.projectInputs[1].View() + "\n"
		}
	} else {
		menuOptions := []string{
			"p - Projects",
			"d - Docker",
			"I - Images  (hub: h)",
			"n - Notes  (sticky)",
			"X - Xray  :10808/:10809",
			"Y - Tor   :9050",
			"c - Clock",
			"H - Htop",
			"s - Scripts",
		}

		menuView += menuTitleStyle.Render("MENU") + "\n"

		for _, opt := range menuOptions {
			menuView += menuItemStyle.Render(opt) + "\n\n"
		}

		separator := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", 20))
		menuView += separator + "\n"
		menuView += "\n" + menuTitleStyle.Render("XRAY STATUS") + "\n"
		// responsive: truncate long server names on small screens
		xrayMax := max(10, m.menuWidth-4)
		if m.xrayStatus == "running" {
			statusText := "Running"
			if m.xrayLastServer != "" {
				statusText += " (" + truncateString(m.xrayLastServer, xrayMax-10) + ")"
			}
			menuView += lipgloss.NewStyle().
				Foreground(lipgloss.Color("46")).
				PaddingLeft(1).
				Render(truncateString("● "+statusText, xrayMax)) + "\n"
			// responsive ports — short on small sidebar
			var xPort string
			if m.menuWidth < 22 {
				xPort = "↳ :10808/:10809"
			} else if m.menuWidth < 26 {
				xPort = "↳ 10808/10809"
			} else {
				xPort = "↳ HTTP:10808  SOCKS:10809"
			}
			portText := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).PaddingLeft(1).Render(truncateString(xPort+"  (xray)", xrayMax))
			menuView += portText + "\n"
		} else {
			menuView += lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")).
				PaddingLeft(1).
				Render("○ Stopped") + "\n"
			var xPort string
			if m.menuWidth < 22 {
				xPort = "↳ :10808/:10809"
			} else {
				xPort = "↳ HTTP:10808  SOCKS:10809"
			}
			portText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).PaddingLeft(1).Render(truncateString(xPort, xrayMax))
			menuView += portText + "\n"
		}
		menuView += menuItemStyle.Render("[X] Open Xray") + "\n"
		menuView += "\n" + menuTitleStyle.Render("TOR STATUS") + "\n"
		torMax := max(10, m.menuWidth-4)
		if m.torStatus == "running" {
			torText := "Running"
			if m.torIP != "" && m.menuWidth >= 22 {
				torText += " (" + truncateString(m.torIP, torMax-10) + ")"
			}
			menuView += lipgloss.NewStyle().
				Foreground(lipgloss.Color("51")).
				PaddingLeft(1).
				Render(truncateString("● "+torText, torMax)) + "\n"
			var tPort string
			if m.menuWidth < 22 {
				tPort = "↳ :9050"
			} else {
				tPort = "↳ SOCKS:9050"
			}
			portText := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).PaddingLeft(1).Render(truncateString(tPort, torMax))
			menuView += portText + "\n"
			if m.menuWidth >= 20 && m.torIP != "" {
				ipDetail := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).PaddingLeft(1).Render(truncateString("IP: "+m.torIP, torMax))
				menuView += ipDetail + "\n"
			}
		} else {
			menuView += lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")).
				PaddingLeft(1).
				Render("○ Stopped") + "\n"
			var tPort string
			if m.menuWidth < 22 {
				tPort = "↳ :9050"
			} else {
				tPort = "↳ SOCKS:9050"
			}
			portText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).PaddingLeft(1).Render(truncateString(tPort, torMax))
			menuView += portText + "\n"
		}
		menuView += menuItemStyle.Render("[Y] Tor") + "\n"
	}

	if m.addMode {
		menuView += "\n" + menuTitleStyle.Render("ADD SCRIPT") + "\n"
		menuView += m.addInputs[0].View() + "\n"
		menuView += m.addInputs[1].View() + "\n"
	} else if m.dockerHubActive {
		menuView += "\n" + menuTitleStyle.Render("DOCKER HUB") + "\n"
		menuView += menuItemStyle.Render("Hub: live search — type to search") + "\n"
		menuView += m.dockerHubInput.View() + "\n"
		if m.dockerHubSearching {
			menuView += menuItemStyle.Render("Searching...") + "\n"
		} else if len(m.dockerHubList.Items()) > 0 {
			menuView += menuItemStyle.Render(fmt.Sprintf("%d results (★ pulls desc)", len(m.dockerHubList.Items()))) + "\n"
			menuView += menuItemStyle.Render("enter/p/d - pull (download)") + "\n"
			menuView += menuItemStyle.Render("r - run (config dialog)") + "\n"
		} else if strings.TrimSpace(m.dockerHubInput.Value()) != "" {
			menuView += menuItemStyle.Render("keep typing... (live 600ms)") + "\n"
		}
		menuView += menuItemStyle.Render("s - focus search") + "\n"
		menuView += menuItemStyle.Render("tab - toggle input/list") + "\n"
		menuView += menuItemStyle.Render("esc - Back") + "\n"
	} else if m.dockerServicesActive {
		menuView += "\n" + menuTitleStyle.Render("DOCKER") + "\n"
		menuView += menuItemStyle.Render("g - Start service") + "\n"
		menuView += menuItemStyle.Render("x - Stop service") + "\n"
		menuView += menuItemStyle.Render("r - Restart service") + "\n"
		menuView += menuItemStyle.Render("enter - Restart service") + "\n"
		menuView += menuItemStyle.Render("i - Images") + "\n"
		menuView += menuItemStyle.Render("s / / - Filter containers") + "\n"
		menuView += menuItemStyle.Render("h - Search Docker Hub") + "\n"
		menuView += menuItemStyle.Render("esc - Back") + "\n"
	} else if m.dockerImagesActive {
		menuView += "\n" + menuTitleStyle.Render("DOCKER IMAGES") + "\n"
		menuView += menuItemStyle.Render("enter/r - Run (config dialog)") + "\n"
		menuView += menuItemStyle.Render("e - Exec into container") + "\n"
		menuView += menuItemStyle.Render("x - Remove image") + "\n"
		menuView += menuItemStyle.Render("i - Inspect image") + "\n"
		menuView += menuItemStyle.Render("p - Prune unused") + "\n"
		menuView += menuItemStyle.Render("d - Services") + "\n"
		menuView += menuItemStyle.Render("s / / - Filter images") + "\n"
		menuView += menuItemStyle.Render("h - Search Docker Hub") + "\n"
		menuView += menuItemStyle.Render("esc - Back") + "\n"
	} else if m.projectsActive {
		menuView += "\n" + menuTitleStyle.Render("PROJECTS") + "\n"
		menuView += menuItemStyle.Render("↑↓ - Select (arrow)") + "\n"
		menuView += menuItemStyle.Render("enter/t - Terminal") + "\n"
		menuView += menuItemStyle.Render("p - Pin/unpin (top)") + "\n"
		menuView += menuItemStyle.Render("o - opencode") + "\n"
		menuView += menuItemStyle.Render("h - hermes") + "\n"
		menuView += menuItemStyle.Render("l - claude") + "\n"
		menuView += menuItemStyle.Render("k - codex") + "\n"
		menuView += menuItemStyle.Render("m - ai-menu") + "\n"
		menuView += menuItemStyle.Render("n - nvim") + "\n"
		menuView += menuItemStyle.Render("c - VS Code") + "\n"
		menuView += menuItemStyle.Render("f - File manager") + "\n"
		menuView += menuItemStyle.Render("a - Add (top)") + "\n"
		menuView += menuItemStyle.Render("d - Delete") + "\n"
		menuView += menuItemStyle.Render("s - Search") + "\n"
		menuView += menuItemStyle.Render("x - Auto-discover") + "\n"
		menuView += menuItemStyle.Render("esc - Back") + "\n"
	} else if m.scriptActive {
		menuView += "\n" + menuTitleStyle.Render("SCRIPTS") + "\n"
		menuView += menuItemStyle.Render("a - Add script") + "\n"
		menuView += menuItemStyle.Render("d - Delete") + "\n"
		menuView += menuItemStyle.Render("e - Edit") + "\n"
		menuView += menuItemStyle.Render("s - Search") + "\n"
		menuView += menuItemStyle.Render("enter - Run") + "\n"
		menuView += menuItemStyle.Render("o - Run in new terminal") + "\n"
		menuView += menuItemStyle.Render("esc - Back") + "\n"
	} else if m.editMode {
		menuView += "\n" + menuTitleStyle.Render("EDIT SCRIPT") + "\n"

		// Show edit inputs for name, path, and key shortcut
		menuView += menuItemStyle.Render("Edit Name:") + "\n"
		menuView += m.editInputs[0].View() + "\n"
		menuView += menuItemStyle.Render("Edit Path:") + "\n"
		menuView += m.editInputs[1].View() + "\n"
		menuView += menuItemStyle.Render("Edit Key Shortcut:") + "\n"
		menuView += m.editKeyShortcutInput.View() + "\n\n"

		// Show toggle options for sudo and proxy
		if it, ok := m.list.SelectedItem().(item); ok {
			sudoChecked := "[ ]"
			if it.sudo {
				sudoChecked = "[x]"
			}
			sudoLine := fmt.Sprintf("%s Add Sudo | u", sudoChecked)
			if m.editSelection == 3 {
				sudoLine = "> " + sudoLine
			} else {
				sudoLine = "  " + sudoLine
			}
			menuView += menuItemStyle.Render(sudoLine) + "\n"

			proxyChecked := "[ ]"
			if it.setProxy {
				proxyChecked = "[x]"
			}
			proxyLine := fmt.Sprintf("%s Set Proxy | p", proxyChecked)
			if m.editSelection == 4 {
				proxyLine = "> " + proxyLine
			} else {
				proxyLine = "  " + proxyLine
			}
			menuView += menuItemStyle.Render(proxyLine) + "\n"

			menuView += "\n" + menuItemStyle.Render("Selected: "+it.title) + "\n"
			if it.keyShortcut != "" {
				menuView += menuItemStyle.Render("Key Shortcut: "+it.keyShortcut) + "\n"
			}
		} else {
			menuView += menuItemStyle.Render("No script selected") + "\n"
		}
	} else if m.lastError != nil {
		menuView += "\n" + errorStyle.Render("ERROR: "+m.lastError.Error())
	} else if m.lastAction != "" {
		menuView += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(m.lastAction)
	}

	var listView string
	var paneHeight int

	// Calculate rightWidth early for centering
	rightWidth := m.width - 4
	if !m.menuHidden {
		rightWidth = m.width - m.menuWidth - 1 - 4
		if rightWidth < 10 {
			rightWidth = 10
		}
	}

	activeList := m.list
	activeTitle := m.list.Title
	if m.dockerHubActive {
		activeList = m.dockerHubList
		activeTitle = "Docker Hub"
		titleView := activeList.Styles.Title.Render(activeTitle)
		hubView := m.dockerHubInput.View()
		if m.menuHidden {
			listWidth := activeList.Width()
			titleView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(activeTitle)
			hubView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(m.dockerHubInput.View())
		}
		if m.dockerHubSearching {
			hubView += "  Searching..."
		} else if m.dockerHubQuery != "" && len(m.dockerHubList.Items()) == 0 {
			hubView += "  (no results)"
		}
		listView = lipgloss.JoinVertical(lipgloss.Top, titleView, hubView, activeList.View())
		paneHeight = lipgloss.Height(listView)
	} else if m.bookmarksActive {
		activeList = m.bookmarkList
		activeTitle = "Bookmarks"
		titleView := activeList.Styles.Title.Render(activeTitle)
		searchView := m.searchInput.View()

		// Center the title and search when menu is hidden
		if m.menuHidden {
			listWidth := activeList.Width()
			titleView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(activeTitle)
			searchView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(m.searchInput.View())
		}

		listView = lipgloss.JoinVertical(lipgloss.Top, titleView, searchView, activeList.View())
		paneHeight = lipgloss.Height(listView)
	} else if m.projectsActive {
		activeList = m.projectsList
		activeTitle = "Projects"
		titleView := activeList.Styles.Title.Render(activeTitle)
		searchView := m.searchInput.View()

		// Center the title and search when menu is hidden
		if m.menuHidden {
			listWidth := activeList.Width()
			titleView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(activeTitle)
			searchView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(m.searchInput.View())
		}

		listView = lipgloss.JoinVertical(lipgloss.Top, titleView, searchView, activeList.View())
		paneHeight = lipgloss.Height(listView)
	} else if m.dockerServicesActive {
		activeList = m.dockerList
		activeTitle = "Docker Services"
		titleView := activeList.Styles.Title.Render(activeTitle)
		searchView := m.searchInput.View()

		if m.menuHidden {
			listWidth := activeList.Width()
			titleView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(activeTitle)
			searchView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(m.searchInput.View())
		}

		listView = lipgloss.JoinVertical(lipgloss.Top, titleView, searchView, activeList.View())
		paneHeight = lipgloss.Height(listView)
	} else if m.dockerImagesActive {
		activeList = m.dockerImageList
		activeTitle = "Docker Images"
		titleView := activeList.Styles.Title.Render(activeTitle)
		searchView := m.searchInput.View()

		if m.menuHidden {
			listWidth := activeList.Width()
			titleView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(activeTitle)
			searchView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(m.searchInput.View())
		}

		listView = lipgloss.JoinVertical(lipgloss.Top, titleView, searchView, activeList.View())
		paneHeight = lipgloss.Height(listView)
	} else if m.scriptActive {
		activeList = m.list
		activeTitle = "Scripts"
		if len(m.list.Items()) == 0 {
			hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Align(lipgloss.Center).Render("No scripts yet")
			hint2 := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Align(lipgloss.Center).Render("Press 'a' to add  ·  'p' for projects  ·  'd' for docker")
			listView = lipgloss.JoinVertical(lipgloss.Top, hint, "", hint2)
			if m.menuHidden {
				listWidth := max(20, m.width-4)
				listView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(listView)
			}
			paneHeight = lipgloss.Height(listView)
		} else {
			titleView := activeList.Styles.Title.Render(activeTitle)
			searchView := m.searchInput.View()
			if m.menuHidden {
				listWidth := activeList.Width()
				titleView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(activeTitle)
				searchView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(m.searchInput.View())
			}
			listView = lipgloss.JoinVertical(lipgloss.Top, titleView, searchView, activeList.View())
			paneHeight = lipgloss.Height(listView)
		}
	} else {
		// dashboard — scripts not shown as main, like other TUI options
		dashTitle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true).Align(lipgloss.Center).Render("IXI Dashboard")
		dashMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Align(lipgloss.Center).Render("p:Projects  d:Docker  n:Notes  s:Scripts  H:Htop  X:Xray  Y:Tor")
		dashHint := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Align(lipgloss.Center).Render("Select from MENU →")
		listView = lipgloss.JoinVertical(lipgloss.Top, dashTitle, "", dashMsg, dashHint)
		if m.menuHidden {
			listWidth := max(20, m.width-4)
			listView = lipgloss.NewStyle().Width(listWidth).Align(lipgloss.Center).Render(listView)
		}
		paneHeight = lipgloss.Height(listView)
	}

	topHeightLogo := 1
	if m.width >= 80 {
		topHeightLogo = lipgloss.Height(logoText)
	}
	topHeight := topHeightLogo + 3
	availableHeight := m.height - topHeight - 4
	if !m.footerHidden {
		availableHeight -= 1
	}
	if m.noteOpen {
		if m.noteFullscreen {

		}
		availableHeight -= 6
		if availableHeight < 5 {
			availableHeight = 5
		}
	}
	containerHeight := paneHeight
	if availableHeight > containerHeight {
		containerHeight = availableHeight
	}

	// Place the list content (will be centered when menu is hidden)
	rightPlaced := lipgloss.Place(rightWidth, containerHeight, lipgloss.Center, lipgloss.Center, listView)

	// Overlay shortcuts manager if open (after rightPlaced so it shows on top)
	if m.shortcutListOpen {
		shortcutListStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1).
			Width(min(80, m.width-4))

		shortcutTitle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Render("⚡ Key Shortcuts (A:add, D:delete, E/R:edit, Enter/Esc:close)")

		shortcutContent := lipgloss.JoinVertical(lipgloss.Top, shortcutTitle, activeList.View())
		shortcutOverlay := shortcutListStyle.Render(shortcutContent)

		// Center the overlay on screen
		rightPlaced = lipgloss.Place(rightWidth, containerHeight, lipgloss.Center, lipgloss.Center, shortcutOverlay)
	} else if m.shortcutAddMode {
		addShortcutStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("42")).
			Padding(1).
			Width(min(60, m.width-4))

		addTitle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true).
			Render("➕ ADD SHORTCUT")

		helpText := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("Tab: switch fields | Enter: save | Esc: cancel")

		addContent := lipgloss.JoinVertical(lipgloss.Top,
			addTitle,
			"",
			helpText,
			"",
			m.shortcutEditInput.View(),
			m.shortcutKeyInput.View())
		addOverlay := addShortcutStyle.Render(addContent)

		rightPlaced = lipgloss.Place(rightWidth, containerHeight, lipgloss.Center, lipgloss.Center, addOverlay)
	} else if m.shortcutEditMode {
		editShortcutStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("226")).
			Padding(1).
			Width(min(60, m.width-4))

		editTitle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Bold(true).
			Render("✏️  EDIT SHORTCUT")

		helpText := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("Tab: switch fields | Enter: save | Esc: cancel")

		scriptInfo := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(fmt.Sprintf("Script: %s", m.shortcutEditOriginal))

		editContent := lipgloss.JoinVertical(lipgloss.Top,
			editTitle,
			"",
			scriptInfo,
			"",
			helpText,
			"",
			m.shortcutKeyInput.View())
		editOverlay := editShortcutStyle.Render(editContent)

		rightPlaced = lipgloss.Place(rightWidth, containerHeight, lipgloss.Center, lipgloss.Center, editOverlay)
	} else if m.imageRunDialogOpen {
		runStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("45")).
			Padding(1).
			Width(min(60, m.width-4))

		runTitle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("45")).
			Bold(true).
			Render("🐳 RUN IMAGE: " + m.imageRunImageRef)

		helpText := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("Tab: switch fields | Enter: run | Esc: cancel")

		runInputViews := []string{runTitle, "", helpText, ""}
		labels := []string{"Container Name:", "Ports (-p):", "Volumes (-v):", "Extra Args:"}
		for i, input := range m.imageRunInputs {
			label := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(labels[i])
			prefix := "  "
			if i == m.imageRunFocus {
				prefix = "▸ "
			}
			runInputViews = append(runInputViews, prefix+label)
			runInputViews = append(runInputViews, "  "+input.View())
		}

		runContent := lipgloss.JoinVertical(lipgloss.Top, runInputViews...)
		runOverlay := runStyle.Render(runContent)

		rightPlaced = lipgloss.Place(rightWidth, containerHeight, lipgloss.Center, lipgloss.Center, runOverlay)
	}

	var content string

	if m.menuHidden {
		content = listPaneStyle.Width(rightWidth).Height(containerHeight).Render(rightPlaced)
	} else {
		leftPane := menuPaneStyle.
			Width(m.menuWidth).
			Height(containerHeight).
			Render(menuView)

		dividerRune := "│"
		dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		divider := dividerStyle.Width(1).Height(containerHeight).Render(dividerRune)

		content = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, divider, listPaneStyle.Width(rightWidth).Height(containerHeight).Render(rightPlaced))
	}

	mainView := lipgloss.JoinVertical(lipgloss.Top, logo, content)

	footerText := "[q] quit  [m] toggle menu  [M] hide menu & center  [k] keybinds  [s/S] search  [n] notes(esc close, ctrl+f fullscreen)  [enter] run  [o] new terminal  [p] projects  [d] docker  [I] images  [h] htop  [X] xray  [Y] tor  [3] edit  [esc] exit edit  [u] sudo toggle  (edit: [p] proxy)  [[]/[]] resize split  [t] smassh  [T] kitty terminal  [docker: g=start x=stop r=restart i=images]  [images: r=run x=remove i=inspect p=prune]  [hub: live search  enter/p=pull r=run]  [projects: ↑↓ select enter=term o=opencode h=hermes l=claude k=codex m=ai-menu]"
	footer := footerStyle.Width(max(10, m.width-2)).Render(footerText)

	if m.noteOpen {
		var noteView string

		// Show category list at the top if open
		if m.noteCategoryListOpen {
			categoryListStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(1).
				Width(m.width - 4)

			m.noteCategoryList.SetSize(m.width-6, 10)
			categoryTitle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Bold(true).
				Render("📂 Note Categories (Enter: select, Esc: close)")

			categoryContent := lipgloss.JoinVertical(lipgloss.Top, categoryTitle, m.noteCategoryList.View())
			noteView = categoryListStyle.Render(categoryContent) + "\n" + m.noteInput.View()
		} else if m.noteCategoryAddMode {
			// Show category creation input at the top
			createCategoryStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("42")).
				Padding(1).
				Width(m.width - 4)

			createTitle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true).
				Render("➕ Create New Category")

			createContent := lipgloss.JoinVertical(lipgloss.Top, createTitle, m.noteCategoryNameInput.View())
			noteView = createCategoryStyle.Render(createContent) + "\n" + m.noteInput.View()
		} else if m.noteCategoryMenuOpen {
			// Show category edit menu - make it responsive (like category list)
			menuStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("105")).
				Padding(1).
				Width(m.width - 4)

			menuTitle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("105")).
				Bold(true).
				Render("📝 Category Menu")

			// Menu options
			menuOptions := []string{"Rename", "Change Color", "Remove", "Save to Folder"}
			var menuItems []string
			for i, option := range menuOptions {
				if i == m.noteCategoryMenuIndex {
					menuItems = append(menuItems, lipgloss.NewStyle().
						Foreground(lipgloss.Color("255")).
						Bold(true).
						Render("▶ "+option))
				} else {
					menuItems = append(menuItems, lipgloss.NewStyle().
						Foreground(lipgloss.Color("240")).
						Render("  "+option))
				}
			}

			menuContent := lipgloss.JoinVertical(lipgloss.Top,
				menuTitle,
				"",
				menuItems[0],
				menuItems[1],
				menuItems[2],
				menuItems[3],
				"",
				lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("↑/↓: Navigate | Enter: Select | Esc: Cancel"))

			noteView = menuStyle.Render(menuContent) + "\n" + m.noteInput.View()
		} else if m.noteCategoryEditMode {
			// Show category edit input at the top
			editCategoryStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("226")).
				Padding(1).
				Width(m.width - 4)

			editTitle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("226")).
				Bold(true).
				Render("✏️  Edit Category")

			editContent := lipgloss.JoinVertical(lipgloss.Top,
				editTitle,
				m.noteCategoryEditInput.View(),
				m.noteCategoryColorInput.View())
			noteView = editCategoryStyle.Render(editContent) + "\n" + m.noteInput.View()
		} else {
			// Normal note view with category info at top
			var categoryInfo string
			if len(m.noteCategories) > 0 && m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
				// Add unsaved indicator if there are changes
				unsavedIndicator := ""
				if m.noteHasUnsavedChanges {
					unsavedIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("208")).
						Bold(true).
						Render(" ● UNSAVED")
				}
				categoryInfo = lipgloss.NewStyle().
					Foreground(lipgloss.Color("42")).
					Bold(true).
					Render(fmt.Sprintf("📁 Category: %s (%d/%d)%s | Ctrl+S: Save | Ctrl+L: List | Ctrl+G: New | Ctrl+N/P: Navigate",
						m.noteCategories[m.currentCategoryIndex].name,
						m.currentCategoryIndex+1,
						len(m.noteCategories),
						unsavedIndicator))
			} else if len(m.noteCategories) > 0 {
				categoryInfo = lipgloss.NewStyle().
					Foreground(lipgloss.Color("240")).
					Render(fmt.Sprintf("📂 %d categories | Ctrl+L: List | Ctrl+G: New | Ctrl+N/P: Navigate", len(m.noteCategories)))
			} else {
				categoryInfo = lipgloss.NewStyle().
					Foreground(lipgloss.Color("240")).
					Render("Press Ctrl+G to create your first note category")
			}
			noteView = categoryInfo + "\n" + m.noteInput.View()
		}

		// show keybinds only in fullscreen, vertical table at bottom
		if m.noteFullscreen && (m.noteCategoryListOpen || m.noteCategoryMenuOpen || m.noteCategoryAddMode || m.noteCategoryEditMode) {
			shortcutsTable := noteShortcutsTable(m.width)
			noteView = lipgloss.JoinVertical(lipgloss.Top, noteView, "", shortcutsTable)
		}

		if m.noteFullscreen {
			// Add border with category color (or white as default)
			borderColor := "255" // Default white

			if len(m.noteCategories) > 0 && m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
				categoryColor := m.noteCategories[m.currentCategoryIndex].color
				if categoryColor != "" {
					borderColor = categoryColor
				}
			}

			// Separate overlay menus from note content
			var overlayContent string
			var actualNoteContent string

			if m.noteCategoryListOpen {
				// Category list should be outside the border - make it responsive
				// Calculate available height for the list
				logoHeight := 3
				if m.width >= 80 {
					logoHeight = lipgloss.Height(logoText) + 3
				}
				footerHeight := 0
				if !m.footerHidden {
					footerHeight = 2
				}

				// Reserve space for: logo, category list box (with padding/border ~6 lines), note border (~3 lines), and note content
				// We need to be very conservative to avoid overflow
				availableForList := m.height - logoHeight - footerHeight - 20 // Reserve 20 lines for note area

				// Set very conservative bounds - max 5 items to ensure it fits
				maxListHeight := min(max(2, availableForList), 5) // Between 2-5 items visible

				categoryListStyle := lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("240")).
					Padding(1).
					Width(m.width - 4)

				m.noteCategoryList.SetSize(m.width-6, maxListHeight)
				categoryListTitle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("255")).
					Bold(true).
					Render("📂 Note Categories (Enter: select, Esc: close)")

				categoryContent := lipgloss.JoinVertical(lipgloss.Top, categoryListTitle, m.noteCategoryList.View())
				overlayContent = categoryListStyle.Render(categoryContent)
				actualNoteContent = m.noteInput.View()
			} else if m.noteCategoryAddMode {
				// Category creation should be outside the border
				createCategoryStyle := lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("42")).
					Padding(1).
					Width(m.width - 4)

				createTitle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("42")).
					Bold(true).
					Render("➕ Create New Category")

				createContent := lipgloss.JoinVertical(lipgloss.Top, createTitle, m.noteCategoryNameInput.View())
				overlayContent = createCategoryStyle.Render(createContent)
				actualNoteContent = m.noteInput.View()
			} else if m.noteCategoryMenuOpen {
				// Category menu should be outside the border - make it responsive (like category list)
				menuStyle := lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("105")).
					Padding(1).
					Width(m.width - 4)

				menuTitle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("105")).
					Bold(true).
					Render("📝 Category Menu")

				menuOptions := []string{"Rename", "Change Color", "Remove", "Save to Folder"}
				var menuItems []string
				for i, option := range menuOptions {
					if i == m.noteCategoryMenuIndex {
						menuItems = append(menuItems, lipgloss.NewStyle().
							Foreground(lipgloss.Color("255")).
							Bold(true).
							Render("▶ "+option))
					} else {
						menuItems = append(menuItems, lipgloss.NewStyle().
							Foreground(lipgloss.Color("240")).
							Render("  "+option))
					}
				}

				menuContent := lipgloss.JoinVertical(lipgloss.Top,
					menuTitle,
					"",
					menuItems[0],
					menuItems[1],
					menuItems[2],
					menuItems[3],
					"",
					lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("↑/↓: Navigate | Enter: Select | Esc: Cancel"))

				overlayContent = menuStyle.Render(menuContent)
				actualNoteContent = m.noteInput.View()
			} else if m.noteCategoryEditMode {
				// Edit mode should be outside the border
				editCategoryStyle := lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("226")).
					Padding(1).
					Width(m.width - 4)

				editTitle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("226")).
					Bold(true).
					Render("✏️  Edit Category")

				editContent := lipgloss.JoinVertical(lipgloss.Top,
					editTitle,
					m.noteCategoryEditInput.View(),
					m.noteCategoryColorInput.View())
				overlayContent = editCategoryStyle.Render(editContent)
				actualNoteContent = m.noteInput.View()
			} else {
				// Normal mode - just the note input, no category info line
				actualNoteContent = m.noteInput.View()
			}

			// Adjust note input height based on whether overlay menus are present
			if overlayContent != "" {
				// Reduce note height to make room for the menu
				// Calculate: total height - top margin - menu height - bottom spacing - borders - footer
				menuHeight := lipgloss.Height(overlayContent)
				topMarginHeight := 2
				bottomSpacingHeight := 1
				borderOverhead := 4 // Border padding (top + bottom)
				footerHeight := 0
				if !m.footerHidden {
					footerHeight = 2
				}

				// Available height for note content
				availableNoteHeight := m.height - topMarginHeight - menuHeight - bottomSpacingHeight - borderOverhead - footerHeight

				// Set minimum and maximum bounds - allow more space for notes
				noteHeight := min(max(5, availableNoteHeight), 20)
				m.noteInput.SetHeight(noteHeight)
				actualNoteContent = m.noteInput.View()
			} else {
				// No menu open - maximize note space in fullscreen
				logoHeight := 3
				if m.width >= 80 {
					logoHeight = lipgloss.Height(logoText) + 3
				}
				borderOverhead := 4 // Border padding (top + bottom)
				footerHeight := 0
				if !m.footerHidden {
					footerHeight = 2
				}

				// Calculate maximum available height for notes
				availableNoteHeight := m.height - logoHeight - borderOverhead - footerHeight

				// Use most of the available space
				noteHeight := max(10, availableNoteHeight)
				m.noteInput.SetHeight(noteHeight)
				actualNoteContent = m.noteInput.View()
			}

			// Create fullscreen border with category color
			fullscreenStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(borderColor)).
				Padding(1).
				Width(m.width - 4)

			borderedNoteView := fullscreenStyle.Render(actualNoteContent)

			// If there's overlay content (menus), hide logo to save space
			// Otherwise show logo in fullscreen
			var finalView string
			if overlayContent != "" {
				// Menu is open - hide logo to save space
				// Add top margin and spacing between overlay and note view
				topMargin := "\n\n"
				bottomSpacing := "\n"
				finalView = lipgloss.JoinVertical(lipgloss.Top, topMargin, overlayContent, bottomSpacing, borderedNoteView)
			} else {
				// No menu - show logo
				finalView = lipgloss.JoinVertical(lipgloss.Top, logo, borderedNoteView)
			}

			if m.footerHidden {
				return finalView
			}
			return lipgloss.JoinVertical(lipgloss.Top, finalView, footer)
		}

		sep := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", max(10, m.width-2)))
		if m.footerHidden {
			return lipgloss.JoinVertical(lipgloss.Top, mainView, sep, noteView)
		}
		return lipgloss.JoinVertical(lipgloss.Top, mainView, sep, noteView, footer)
	}
	// sticky notes on main page — each category = 1 sticky note box with tasks (Ctrl+T)
	if !m.noteOpen && len(m.noteCategories) > 0 {
		if sticky := renderStickyNotes(m, m.width); sticky != "" {
			mainView = lipgloss.JoinVertical(lipgloss.Top, mainView, "", sticky)
		}
	}
	if m.footerHidden {
		return mainView
	}
	return lipgloss.JoinVertical(lipgloss.Top, mainView, footer)
}

func (m *model) updateNoteTextareaBorderColor() {
	// Update textarea border color based on current category
	borderColor := "255" // Default white
	if len(m.noteCategories) > 0 && m.currentCategoryIndex >= 0 && m.currentCategoryIndex < len(m.noteCategories) {
		categoryColor := m.noteCategories[m.currentCategoryIndex].color
		if categoryColor != "" {
			borderColor = categoryColor
		}
	}

	// Update the focused style border color
	focusedStyle := m.noteInput.FocusedStyle
	focusedStyle.Base = focusedStyle.Base.BorderForeground(lipgloss.Color(borderColor))
	m.noteInput.FocusedStyle = focusedStyle
}

func pingService(address string) string {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		return "N/A"
	}
	defer conn.Close()
	return fmt.Sprintf("%dms", time.Since(start).Milliseconds())
}

type DockerCompose struct {
	Services map[string]struct {
		Ports []string `yaml:"ports"`
	} `yaml:"services"`
}

func getDockerServices() ([]list.Item, error) {
	// Check if the docker-compose file exists
	if _, err := os.Stat(dockerComposePath); os.IsNotExist(err) {
		// If the file doesn't exist, return an empty list with a helpful message
		return []list.Item{}, fmt.Errorf("docker-compose file not found at %s", dockerComposePath)
	}

	data, err := os.ReadFile(dockerComposePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read docker-compose.yml: %w", err)
	}

	var compose DockerCompose
	err = yaml.Unmarshal(data, &compose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose.yml: %w", err)
	}

	composeCmd, err := dockerComposeCommand()
	if err != nil {
		return nil, err
	}

	var services []list.Item

	// First, try to get all services status using docker compose ps
	cmd := exec.Command(composeCmd[0], append(composeCmd[1:], "-f", dockerComposePath, "ps", "--format", "json")...)
	output, err := cmd.Output()

	serviceStatusMap := make(map[string]string)

	if err == nil {
		// Parse the JSON output to get service statuses
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			var psInfo map[string]interface{}
			if err := json.Unmarshal([]byte(line), &psInfo); err != nil {
				continue
			}

			serviceName, ok := psInfo["Service"].(string)
			if !ok {
				continue
			}

			state, ok := psInfo["State"].(string)
			if !ok {
				continue
			}

			serviceStatusMap[serviceName] = state
		}
	} else {
		// If we can't get the status, log the error but continue
		fmt.Printf("Warning: Could not get service statuses: %v\n", err)
	}

	// Add all services defined in the compose file
	for name, serviceConfig := range compose.Services {
		status := "exited" // default status
		responseTime := "N/A"
		address := ""

		// Check if we got the status from the command
		if cmdStatus, exists := serviceStatusMap[name]; exists {
			status = cmdStatus
		} else {
			// If we couldn't get the status, try to get it individually
			cmd := exec.Command(composeCmd[0], append(composeCmd[1:], "-f", dockerComposePath, "ps", "-q", name)...)
			idOutput, idErr := cmd.Output()
			if idErr == nil && strings.TrimSpace(string(idOutput)) != "" {
				// Service exists, check its status
				cmd = exec.Command(composeCmd[0], append(composeCmd[1:], "-f", dockerComposePath, "ps", "--filter", fmt.Sprintf("name=%s", name), "--format", "json")...)
				psOutput, psErr := cmd.Output()
				if psErr == nil {
					psLines := strings.Split(strings.TrimSpace(string(psOutput)), "\n")
					for _, psLine := range psLines {
						psLine = strings.TrimSpace(psLine)
						if psLine == "" {
							continue
						}
						var psInfo map[string]interface{}
						if err := json.Unmarshal([]byte(psLine), &psInfo); err != nil {
							continue
						}
						serviceName, ok := psInfo["Service"].(string)
						if !ok {
							continue
						}
						if serviceName == name {
							state, ok := psInfo["State"].(string)
							if ok && state != "" {
								status = state
							}
							break
						}
					}
				}
			}
		}

		// Check if service is running and has ports exposed
		if status == "running" && len(serviceConfig.Ports) > 0 {
			portStr := strings.Split(serviceConfig.Ports[0], ":")[0]
			address = fmt.Sprintf("%s:%s", primaryIPv4(), portStr)
			responseTime = pingService(fmt.Sprintf("localhost:%s", portStr))
		}

		services = append(services, dockerService{title: name, status: status, responseTime: responseTime, address: address})
	}

	return services, nil
}

func dockerComposeCommand() ([]string, error) {
	if path, err := exec.LookPath("docker"); err == nil {
		return []string{path, "compose"}, nil
	}
	if path, err := exec.LookPath("docker-compose"); err == nil {
		return []string{path}, nil
	}
	return nil, fmt.Errorf("docker compose not found in PATH")
}

func runDockerServiceAction(service, action string) error {
	composeCmd, err := dockerComposeCommand()
	if err != nil {
		return err
	}
	args := append([]string{}, composeCmd[1:]...)
	args = append(args, "-f", dockerComposePath)
	switch action {
	case "start":
		args = append(args, "up", "-d", service)
	case "stop", "restart":
		args = append(args, action, service)
	default:
		return fmt.Errorf("unknown docker action: %s", action)
	}
	cmd := exec.Command(composeCmd[0], args...)
	cmd.Dir = filepath.Dir(dockerComposePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose %s %s failed: %w (%s)", action, service, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func getDockerImages() ([]list.Item, error) {
	cmd := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}\t{{.ID}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list docker images: %w", err)
	}

	var images []list.Item
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		if name == "<none>:<none>" {
			name = "<none> (" + parts[3] + ")"
		}
		images = append(images, dockerImage{
			title:   name,
			size:    parts[1],
			created: parts[2],
			id:      parts[3],
		})
	}
	return images, nil
}

func runDockerImageAction(image, action string) error {
	var cmd *exec.Cmd
	switch action {
	case "remove":
		cmd = exec.Command("docker", "rmi", image)
	case "prune":
		cmd = exec.Command("docker", "image", "prune", "-f")
	case "inspect":
		cmd = exec.Command("docker", "inspect", image)
	default:
		return fmt.Errorf("unknown image action: %s", action)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker image %s failed: %w (%s)", action, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runDockerImageWithOptions(name, ports, volumes, extra, image string) (string, error) {
	args := []string{"run", "-d"}
	if name != "" {
		args = append(args, "--name", name)
	}
	if ports != "" {
		for _, p := range strings.Split(ports, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				args = append(args, "-p", p)
			}
		}
	}
	if volumes != "" {
		for _, v := range strings.Split(volumes, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				args = append(args, "-v", v)
			}
		}
	}
	if extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	args = append(args, image)
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run failed: %s", strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func findContainerFromImage(image string) (string, error) {
	cmd := exec.Command("docker", "ps", "--filter", "ancestor="+image, "--format", "{{.Names}}", "--last", "1")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find container: %w", err)
	}
	name := strings.TrimSpace(string(output))
	if name == "" {
		return "", fmt.Errorf("no running container found for image: %s", image)
	}
	return name, nil
}

func execContainerCmd(containerName string) tea.Cmd {
	shellCmd := fmt.Sprintf("docker exec -it %s bash", containerName)
	return func() tea.Msg {
		var cmd *exec.Cmd
		if term, err := exec.LookPath("xdg-terminal-exec"); err == nil {
			cmd = exec.Command(term, "--hold", "--", "bash", "-c", shellCmd)
		} else if term, err := exec.LookPath("kitty"); err == nil {
			cmd = exec.Command(term, "bash", "-c", shellCmd)
		} else if term, err := exec.LookPath("konsole"); err == nil {
			cmd = exec.Command(term, "-e", "bash", "-c", shellCmd)
		} else if term, err := exec.LookPath("gnome-terminal"); err == nil {
			cmd = exec.Command(term, "--", "bash", "-c", shellCmd)
		} else if term, err := exec.LookPath("xterm"); err == nil {
			cmd = exec.Command(term, "-e", "bash", "-c", shellCmd)
		} else {
			return scriptFinishedMsg{Title: "Exec", Path: "", Err: fmt.Errorf("no terminal emulator found")}
		}
		err := cmd.Start()
		if err != nil {
			return scriptFinishedMsg{Title: "Exec", Path: "", Err: err}
		}
		return scriptFinishedMsg{Title: "Exec", Path: "", Err: nil}
	}
}

func listItemsToStored(items []list.Item) []storedItem {
	out := make([]storedItem, 0, len(items))
	for _, it := range items {
		i, ok := it.(item)
		if !ok {
			continue
		}
		out = append(out, storedItem{Title: i.title, Path: i.path, Sudo: i.sudo, SetProxy: i.setProxy, KeyShortcut: i.keyShortcut})
	}
	return out
}

func listProjectsToStored(items []list.Item) []storedProject {
	out := make([]storedProject, 0, len(items))
	for _, it := range items {
		p, ok := it.(project)
		if !ok {
			continue
		}
		out = append(out, storedProject{Name: p.name, Path: p.path, Pinned: p.pinned})
	}
	return out
}

func listBookmarksToStored(items []list.Item) []storedBookmark {
	out := make([]storedBookmark, 0, len(items))
	for _, it := range items {
		b, ok := it.(bookmark)
		if !ok {
			continue
		}
		out = append(out, storedBookmark{Title: b.title, Url: b.url})
	}
	return out
}

func isXrayRunning() bool {
	cmd := exec.Command("pgrep", "-x", "xray")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

func getXrayStatus() (string, string) {
	if !isXrayRunning() {
		return "stopped", ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "running", ""
	}

	configPath := filepath.Join(homeDir, ".xray-manager.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "running", ""
	}

	var appConfig struct {
		LastUsedServer string `json:"last_used_server"`
	}

	if err := json.Unmarshal(data, &appConfig); err != nil {
		return "running", ""
	}

	return "running", appConfig.LastUsedServer
}

func isTorRunning() bool {
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "is-active", "tor")
		out, _ := cmd.Output()
		if strings.TrimSpace(string(out)) == "active" {
			return true
		}
	}
	cmd := exec.Command("pgrep", "-x", "tor")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func getTorIP() string {
	// try via tor SOCKS first (like ip-changer.sh get_ip)
	proxyURL, _ := url.Parse("socks5h://127.0.0.1:9050")
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	resp, err := client.Get("https://checkip.amazonaws.com")
	if err == nil {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		ip := strings.TrimSpace(string(b))
		if matched, _ := exec.Command("sh", "-c", "echo "+ip+" | grep -oP '\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}'").Output(); len(strings.TrimSpace(string(matched))) > 0 {
			return strings.TrimSpace(string(matched))
		}
		if ip != "" && strings.Count(ip, ".") == 3 {
			return ip
		}
	}
	// fallback: direct check (shows non-tor IP if tor down)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://checkip.amazonaws.com", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err == nil {
		defer resp2.Body.Close()
		b, _ := io.ReadAll(resp2.Body)
		return strings.TrimSpace(string(b))
	}
	return ""
}

func getTorStatus() (string, string) {
	if !isTorRunning() {
		return "stopped", ""
	}
	ip := getTorIP()
	return "running", ip
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func primaryIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {

		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}

			if ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			return ip4.String()
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isLegacyDefaultList(items []storedItem) bool {
	legacy := []storedItem{
		{Title: "System Cleanup", Path: "/home/ixi_flower/scripts/cleanup.sh"},
		{Title: "Backup Database", Path: "/home/ixi_flower/scripts/db_backup.sh"},
		{Title: "Deploy App", Path: "/opt/deploy/run.sh"},
		{Title: "Network Check", Path: "/usr/local/bin/net_check.sh"},
		{Title: "Log Rotation", Path: "/etc/cron.daily/logrotate"},
	}
	if len(items) != len(legacy) {
		return false
	}
	for i := range legacy {
		if items[i].Title != legacy[i].Title || items[i].Path != legacy[i].Path {
			return false
		}
	}
	return true
}

func logCommand(cmd string) error {
	logDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("could not get user config dir: %w", err)
	}
	appLogDir := filepath.Join(logDir, "ixi-flower-tui")
	if err := os.MkdirAll(appLogDir, 0o755); err != nil {
		return fmt.Errorf("could not create app log directory: %w", err)
	}
	logFilePath := filepath.Join(appLogDir, "command.log")

	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("could not open command log file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("%s: %s\n", time.Now().Format(time.RFC3339), cmd))
	if err != nil {
		return fmt.Errorf("could not write to command log file: %w", err)
	}
	return nil
}

func main() {
	m := initialModel()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error starting program: %v", err)
		os.Exit(1)
	}
}
