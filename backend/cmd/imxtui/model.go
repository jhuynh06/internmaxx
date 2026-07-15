package main

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jhuynh06/internmaxx/backend/internal/jobsclient"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

type config struct {
	client        *jobsclient.Client
	companiesPath string
	reg           *registry.Registry // nil OK
	pageLimit     int
	refresh       time.Duration
	daysToggle    int // window 'd' toggles to (7, or the --days value)
	initialDays   int
}

type model struct {
	cfg config

	days      int    // current window (0 = all time)
	company   string // resolved display name applied to ?company= ("" = all)
	filterArg string // last raw filter text, for the "did you mean" hint

	items      []jobsclient.SeenJob
	total      int
	nextOffset *int
	seenKeys   map[string]bool // every key shown this session (new-row diff)
	newKeys    map[string]bool // keys first seen in the latest poll (highlighted)
	lastUpdate time.Time

	cursor int // selected index into items
	top    int // first visible row

	fetchSeq    int // bumped per fetch; stale responses are dropped
	debounceSeq int // bumped per filter keystroke
	loading     bool
	loadingMore bool

	filter    textinput.Model
	filtering bool
	spin      spinner.Model
	showHelp  bool
	errText   string
	yankFlash string

	width, height int
	listHeight    int
	keys          keymap
}

// ── messages ────────────────────────────────────────────────────────
type tickMsg time.Time
type debounceMsg struct{ seq int }
type jobsMsg struct {
	seq    int
	append bool
	resp   jobsclient.JobsResponse
}
type errMsg struct {
	seq int
	err error
}
type yankedMsg struct{}
type flashExpireMsg struct{}

func initialModel(cfg config) model {
	ti := textinput.New()
	ti.Placeholder = "company…"
	ti.Prompt = "filter: "
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return model{
		cfg:      cfg,
		days:     cfg.initialDays,
		seenKeys: map[string]bool{},
		newKeys:  map[string]bool{},
		filter:   ti,
		spin:     sp,
		loading:  true,
		fetchSeq: 1,
		keys:     defaultKeys(),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick, m.fetchCmd(0, false)}
	if m.cfg.refresh > 0 {
		cmds = append(cmds, tickCmd(m.cfg.refresh))
	}
	return tea.Batch(cmds...)
}

// ── commands ────────────────────────────────────────────────────────

func (m model) fetchCmd(offset int, appnd bool) tea.Cmd {
	seq := m.fetchSeq
	q := jobsclient.Query{Company: m.company, Days: m.days, Limit: m.cfg.pageLimit, Offset: offset}
	client := m.cfg.client
	return func() tea.Msg {
		resp, _, err := client.Jobs(context.Background(), q)
		if err != nil {
			return errMsg{seq: seq, err: err}
		}
		return jobsMsg{seq: seq, append: appnd, resp: resp}
	}
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func debounceCmd(seq int) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return debounceMsg{seq} })
}

// yankCmd copies url to the LOCAL clipboard via OSC 52. Emitted only from a
// tea.Cmd (never View/Update): the escape prints no glyphs and doesn't move the
// cursor, so a single Write can't corrupt the alt-screen frame. The sequence
// travels back through the SSH pty to the user's terminal, so it targets the
// local clipboard even though imxtui runs on the VM.
func yankCmd(url string) tea.Cmd {
	return func() tea.Msg {
		os.Stdout.WriteString("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(url)) + "\a")
		return yankedMsg{}
	}
}

func flashCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return flashExpireMsg{} })
}

// ── update ──────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.recalcListHeight()
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.loadingMore {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case jobsMsg:
		if msg.seq != m.fetchSeq {
			return m, nil // stale
		}
		m.applyJobs(msg)
		return m, nil

	case errMsg:
		if msg.seq != m.fetchSeq {
			return m, nil
		}
		m.loading, m.loadingMore = false, false
		m.errText = msg.err.Error()
		return m, nil

	case tickMsg:
		var cmds []tea.Cmd
		if m.cfg.refresh > 0 {
			cmds = append(cmds, tickCmd(m.cfg.refresh)) // re-arm here and ONLY here
		}
		// Auto-refresh only when idle and on page one, so we never clobber a
		// deep scroll, an in-flight fetch, or an open filter.
		if !m.loading && !m.loadingMore && !m.filtering && len(m.items) <= m.cfg.pageLimit {
			cmds = append(cmds, m.reload())
		}
		return m, tea.Batch(cmds...)

	case debounceMsg:
		if msg.seq != m.debounceSeq {
			return m, nil
		}
		return m.applyFilterText(m.filter.Value())

	case yankedMsg:
		m.yankFlash = "yanked ✓"
		return m, flashCmd()

	case flashExpireMsg:
		m.yankFlash = ""
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m *model) applyJobs(msg jobsMsg) {
	r := msg.resp
	if msg.append {
		m.items = append(m.items, r.Items...)
		for _, j := range r.Items {
			m.seenKeys[j.Key] = true
		}
		m.loadingMore = false
	} else {
		firstLoad := len(m.seenKeys) == 0
		m.newKeys = map[string]bool{}
		for _, j := range r.Items {
			if !firstLoad && !m.seenKeys[j.Key] {
				m.newKeys[j.Key] = true // new since last poll → highlight
			}
			m.seenKeys[j.Key] = true
		}
		m.items = r.Items
		m.lastUpdate = time.Now()
		m.loading = false
	}
	m.total = r.Total
	m.nextOffset = r.NextOffset
	m.errText = ""
	m.clampScroll()
}

func (m *model) recalcListHeight() {
	chrome := 1 + 5 + 1 // header + detail box (3 lines + border) + status
	if m.filtering {
		chrome++
	}
	h := m.height - chrome
	if h < 1 {
		h = 1
	}
	m.listHeight = h
	m.clampScroll()
}

func (m *model) clampScroll() {
	if m.listHeight < 1 {
		m.listHeight = 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+m.listHeight {
		m.top = m.cursor - m.listHeight + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m model) selected() (jobsclient.SeenJob, bool) {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor], true
	}
	return jobsclient.SeenJob{}, false
}

// reload fetches page one fresh with the spinner on — the shared path for
// manual refresh, the days toggle, and applying/clearing a filter.
func (m *model) reload() tea.Cmd {
	m.fetchSeq++
	m.loading = true
	return tea.Batch(m.spin.Tick, m.fetchCmd(0, false))
}

// maybeLoadMore appends the next page when the cursor nears the loaded tail.
func (m *model) maybeLoadMore() tea.Cmd {
	if m.nextOffset != nil && !m.loadingMore && m.cursor >= len(m.items)-5 {
		m.loadingMore = true
		m.fetchSeq++
		return tea.Batch(m.spin.Tick, m.fetchCmd(*m.nextOffset, true))
	}
	return nil
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		m.clampScroll()
		return m, m.maybeLoadMore()
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		m.clampScroll()
		return m, nil
	case key.Matches(msg, m.keys.Home):
		m.cursor, m.top = 0, 0
		return m, nil
	case key.Matches(msg, m.keys.End):
		m.cursor = max(0, len(m.items)-1)
		m.clampScroll()
		return m, m.maybeLoadMore()
	case key.Matches(msg, m.keys.Yank):
		if j, ok := m.selected(); ok {
			return m, yankCmd(j.URL)
		}
		return m, nil
	case key.Matches(msg, m.keys.Refresh):
		return m, m.reload()
	case key.Matches(msg, m.keys.Days):
		if m.days == 0 {
			m.days = m.cfg.daysToggle
		} else {
			m.days = 0
		}
		m.cursor, m.top = 0, 0
		return m, m.reload()
	case key.Matches(msg, m.keys.Filter):
		m.filtering = true
		m.filter.SetValue("")
		m.filter.Focus()
		m.recalcListHeight()
		return m, textinput.Blink
	}
	return m, nil
}

func (m model) updateFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filtering = false
		m.filter.Blur()
		m.filterArg = ""
		m.recalcListHeight()
		if m.company != "" { // clear an applied filter
			m.company = ""
			m.cursor, m.top = 0, 0
			return m, m.reload()
		}
		return m, nil
	case "enter":
		m.filtering = false
		m.filter.Blur()
		m.recalcListHeight()
		m.debounceSeq++ // cancel any pending debounce
		return m.applyFilterText(m.filter.Value())
	default:
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.debounceSeq++
		return m, tea.Batch(cmd, debounceCmd(m.debounceSeq))
	}
}

func (m model) applyFilterText(text string) (tea.Model, tea.Cmd) {
	m.filterArg = text
	if strings.TrimSpace(text) == "" {
		m.company = ""
	} else {
		m.company, _ = jobsclient.ResolveCompany(text, m.cfg.companiesPath)
	}
	m.cursor, m.top = 0, 0
	return m, m.reload()
}
