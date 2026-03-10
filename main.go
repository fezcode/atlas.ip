package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

var Version = "dev"

// --- Styles ---
var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D52A8"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}

	urlStyle = lipgloss.NewStyle().Foreground(special).Underline(true)
	docStyle = lipgloss.NewStyle().Padding(1, 2, 1, 2)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(highlight).
			Padding(0, 1).
			MarginBottom(1)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(highlight).
			Padding(0, 1).
			MarginBottom(1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight).
			MarginBottom(1)
)

// --- Models ---
type IPInfo struct {
	Query      string `json:"query"`
	Status     string `json:"status"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
	ISP        string `json:"isp"`
	AS         string `json:"as"`
	Timezone   string `json:"timezone"`
}

type InterfaceInfo struct {
	Name string
	IP   string
	MAC  string
	MTU  int
}

type model struct {
	loading    bool
	ipInfo     *IPInfo
	interfaces []InterfaceInfo
	err        error
	quitting   bool
}

type (
	ipMsg  *IPInfo
	intMsg []InterfaceInfo
	errMsg error
)

// --- Commands ---
func fetchIP() tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://ip-api.com/json")
		if err != nil {
			return errMsg(err)
		}
		defer resp.Body.Close()

		var info IPInfo
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return errMsg(err)
		}
		return ipMsg(&info)
	}
}

func fetchInterfaces() tea.Cmd {
	return func() tea.Msg {
		var results []InterfaceInfo
		ifaces, err := net.Interfaces()
		if err != nil {
			return errMsg(err)
		}
		for _, i := range ifaces {
			if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := i.Addrs()
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip.To4() != nil {
					results = append(results, InterfaceInfo{
						Name: i.Name,
						IP:   ip.String(),
						MAC:  i.HardwareAddr.String(),
						MTU:  i.MTU,
					})
				}
			}
		}
		return intMsg(results)
	}
}

// --- Lifecycle ---
func (m model) Init() tea.Cmd {
	return tea.Batch(fetchIP(), fetchInterfaces())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			m.loading = true
			m.ipInfo = nil
			return m, tea.Batch(fetchIP(), fetchInterfaces())
		}
	case ipMsg:
		m.ipInfo = msg
		m.loading = false
	case intMsg:
		m.interfaces = msg
	case errMsg:
		m.err = msg
		m.loading = false
	}
	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	// Header
	s.WriteString(headerStyle.Render(fmt.Sprintf(" ATLAS IP v%s ", Version)) + "\n")

	if m.err != nil {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(fmt.Sprintf("Error: %v", m.err)) + "\n")
	}

	// External Card
	externalCard := "Fetching data..."
	if m.ipInfo != nil {
		info := m.ipInfo
		externalCard = lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Public Network (WAN)"),
			fmt.Sprintf("%-10s %s", "IPv4:", urlStyle.Render(info.Query)),
			fmt.Sprintf("%-10s %s, %s, %s", "Location:", info.City, info.RegionName, info.Country),
			fmt.Sprintf("%-10s %s (%s)", "ISP:", info.ISP, info.AS),
			fmt.Sprintf("%-10s %s", "Timezone:", info.Timezone),
		)
	}
	s.WriteString(cardStyle.Render(externalCard) + "\n")

	// Local Card (Table)
	re := lipgloss.NewStyle().Foreground(special).Bold(true)
	
	rows := [][]string{}
	for _, i := range m.interfaces {
		rows = append(rows, []string{
			re.Render(i.Name),
			i.IP,
			i.MAC,
			fmt.Sprintf("%d", i.MTU),
		})
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(subtle)).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return lipgloss.NewStyle().Foreground(highlight).Bold(true).Align(lipgloss.Center)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		}).
		Headers("INTERFACE", "IP ADDRESS", "MAC ADDRESS", "MTU").
		Rows(rows...)

	localSection := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Local Interfaces (LAN)"),
		t.Render(),
	)
	
	s.WriteString(cardStyle.Render(localSection) + "\n")

	// Footer
	s.WriteString(lipgloss.NewStyle().Faint(true).Render("r: refresh • q: quit"))

	return docStyle.Render(s.String())
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Printf("atlas.ip v%s\n", Version)
		return
	}

	p := tea.NewProgram(model{loading: true})
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
