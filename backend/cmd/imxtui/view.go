package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jhuynh06/internmaxx/backend/internal/jobsclient"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	faintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selStyle    = lipgloss.NewStyle().Reverse(true)
	newStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	borderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteByte('\n')
	if m.filtering {
		b.WriteString(m.filter.View())
		b.WriteByte('\n')
	}
	b.WriteString(m.listView())
	b.WriteByte('\n')
	b.WriteString(m.detailView())
	b.WriteByte('\n')
	b.WriteString(m.statusView())
	return b.String()
}

func (m model) headerView() string {
	upd := "—"
	if !m.lastUpdate.IsZero() {
		upd = m.lastUpdate.Format("15:04:05")
	}
	spin := " "
	if m.loading || m.loadingMore {
		spin = m.spin.View()
	}
	scope := "all time"
	if m.days > 0 {
		scope = fmt.Sprintf("last %dd", m.days)
	}
	if m.company != "" {
		scope = m.company
	}
	meta := fmt.Sprintf("· %d jobs · %s · updated %s %s", m.total, scope, upd, spin)
	return titleStyle.Render("internmaxx") + " " + faintStyle.Render(meta)
}

func (m model) listView() string {
	if len(m.items) == 0 {
		msg := "no jobs found"
		if m.company != "" {
			msg += fmt.Sprintf(" for %q", m.filterArg)
			if near := jobsclient.NearCompanies(m.filterArg, m.cfg.reg); len(near) > 0 {
				msg += "\n" + faintStyle.Render("did you mean: "+strings.Join(near, ", "))
			}
		}
		return padLines(msg, m.listHeight)
	}
	var rows []string
	end := min(m.top+m.listHeight, len(m.items))
	for i := m.top; i < end; i++ {
		rows = append(rows, m.rowView(i))
	}
	for len(rows) < m.listHeight { // pad so the layout below stays put
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

func (m model) rowView(i int) string {
	j := m.items[i]
	badge := "   "
	if m.newKeys[j.Key] {
		badge = "NEW"
	}
	prefix := 16 + 1 + 3 + 1 + 16 + 1 // ts + badge + company + gaps
	titleW := m.width - prefix
	if titleW < 10 {
		titleW = 10
	}
	line := fmt.Sprintf("%-16s %-3s %-16s %s",
		jobsclient.FormatSeen(j.FirstSeen), badge, truncate(j.Company, 16), truncate(j.Title, titleW))
	switch {
	case i == m.cursor:
		return selStyle.Render(fitWidth(line, m.width))
	case m.newKeys[j.Key]:
		return newStyle.Render(line)
	default:
		return line
	}
}

func (m model) detailView() string {
	w := m.width - 4 // inside border + padding
	if w < 10 {
		w = 10
	}
	var content string
	if j, ok := m.selected(); ok {
		content = fmt.Sprintf("%s\n%s\nfirst seen: %s",
			truncate(j.Company+" / "+j.Title, w),
			truncate(j.URL, w),
			jobsclient.FormatSeen(j.FirstSeen))
	} else {
		content = "—\n \n "
	}
	return borderStyle.Width(m.width - 2).Render(content)
}

func (m model) statusView() string {
	if m.showHelp {
		return faintStyle.Render("↑/k up · ↓/j down · g/G home/end · y yank url · / filter · r refresh · d days · ? help · q quit")
	}
	var left string
	switch {
	case m.errText != "":
		left = errStyle.Render("error: "+m.errText) + faintStyle.Render(" (r to retry)")
	case m.loading:
		left = "loading…"
	default:
		pos := 0
		if len(m.items) > 0 {
			pos = m.cursor + 1
		}
		left = fmt.Sprintf("%d/%d", pos, m.total)
		if m.nextOffset != nil {
			left += " · ↓ more"
		} else {
			left += " · end"
		}
	}
	if m.yankFlash != "" {
		left += "  " + newStyle.Render(m.yankFlash)
	}
	right := faintStyle.Render("y yank · / filter · r refresh · d days · ? help · q quit")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left // too narrow to show the hint bar
	}
	return left + strings.Repeat(" ", gap) + right
}

// ── small text helpers ──────────────────────────────────────────────

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func fitWidth(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func padLines(s string, n int) string {
	lines := strings.Count(s, "\n") + 1
	if lines >= n {
		return s
	}
	return s + strings.Repeat("\n", n-lines)
}
