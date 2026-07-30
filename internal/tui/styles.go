package tui

import "github.com/charmbracelet/lipgloss"

var (
	accent = lipgloss.AdaptiveColor{Light: "#0078D4", Dark: "#38A8FF"}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent)

	subtitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#334155", Dark: "#CBD5E1"})

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"})

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94A3B8"})

	activeLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accent)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#64748B"})

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#047857", Dark: "#6EE7B7"})

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FCA5A5"})

	buttonStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#94A3B8", Dark: "#475569"})

	buttonActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#071013")).
				Background(accent).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(accent)

	tableBorderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#CBD5E1", Dark: "#334155"})

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1).
				Foreground(lipgloss.AdaptiveColor{Light: "#334155", Dark: "#CBD5E1"})

	tableSelectedHeaderStyle = lipgloss.NewStyle().
					Bold(true).
					Padding(0, 1).
					Foreground(accent)

	tableCellStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.AdaptiveColor{Light: "#334155", Dark: "#CBD5E1"})

	tableSelectedCellStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1).
				Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#071013"}).
				Background(accent)

	tableSelectedLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1).
				Foreground(accent)

	tableTotalStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(accent)

	detailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#334155", Dark: "#CBD5E1"})
)

func (m Model) appFrame(content string) string {
	framed := lipgloss.NewStyle().Padding(1, 2).Render(content)
	if m.width <= 0 || m.height <= 0 {
		return framed
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, framed)
}
