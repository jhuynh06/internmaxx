// Command imxtui is an interactive terminal UI for browsing seen internship
// postings via the daemon's read-only /jobs API. It's the interactive companion
// to the one-shot imx CLI.
//
//	imxtui [--days N] [--limit N] [--refresh 30s] [--addr URL]
//
// Keys: j/k navigate · y yank selected URL to the local clipboard (OSC 52) ·
// / filter by company · r refresh · d toggle days window · ? help · q quit.
//
// It runs on the VM over `ssh -t` (the API binds localhost there); see
// deploy/imxtui.sh. Address resolves --addr > IMX_API_ADDR > API_ADDR >
// http://127.0.0.1:8080, same as imx. OSC 52 yank travels back through the SSH
// pty to your local terminal's clipboard (iTerm2/kitty/WezTerm/Ghostty work;
// inside tmux set `set -g set-clipboard on`).
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jhuynh06/internmaxx/backend/internal/jobsclient"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func main() {
	addr := flag.String("addr", "", "API base URL (default IMX_API_ADDR/API_ADDR or http://127.0.0.1:8080)")
	days := flag.Int("days", 0, "recency window in days (0 = all time); 'd' toggles at runtime")
	limit := flag.Int("limit", 50, "page size per request (server caps at 100)")
	refresh := flag.Duration("refresh", 30*time.Second, "auto-refresh interval (0 disables)")
	companies := flag.String("companies", "companies.yaml", "registry path for slug->name filter resolution")
	flag.Parse()

	if *limit < 1 {
		*limit = 1
	}
	reg, _ := registry.Load(*companies) // best-effort; nil if the file is absent

	toggle := 7
	if *days > 0 {
		toggle = *days
	}

	cfg := config{
		client:        jobsclient.New(*addr),
		companiesPath: *companies,
		reg:           reg,
		pageLimit:     *limit,
		refresh:       *refresh,
		daysToggle:    toggle,
		initialDays:   *days,
	}

	if _, err := tea.NewProgram(initialModel(cfg), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "imxtui:", err)
		os.Exit(1)
	}
}
