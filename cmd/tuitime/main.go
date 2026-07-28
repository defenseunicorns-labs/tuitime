package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tuitime/internal/clicktime"
	"tuitime/internal/tui"
)

func main() {
	token := strings.TrimSpace(os.Getenv("CLICKTIME_TOKEN"))
	if token == "" {
		fmt.Fprintln(os.Stderr, "tuitime: CLICKTIME_TOKEN is not set")
		fmt.Fprintln(os.Stderr, "export CLICKTIME_TOKEN='your-clicktime-api-token'")
		os.Exit(2)
	}

	program := tea.NewProgram(
		tui.New(clicktime.New(token)),
		tea.WithAltScreen(),
	)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tuitime: %v\n", err)
		os.Exit(1)
	}
}
