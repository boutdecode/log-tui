// Package tui contient l'interface Bubble Tea de log-tui.
package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/i18n"
	"github.com/kevin/log-tui/internal/notify"
	"github.com/kevin/log-tui/internal/query"
	"github.com/kevin/log-tui/internal/scheduler"
	"github.com/kevin/log-tui/internal/store"
)

const rowLimit = 5000

type mode int

const (
	modeLog mode = iota
	modeSearch
	modeDetail
	modeStats
	modeHelp
	modeConfig
)

type levelPreset struct {
	label  string
	levels []string
}

// levelPresets est une fonction : le libellé « tous » dépend de la langue.
func levelPresets() []levelPreset {
	return []levelPreset{
		{i18n.T("level.all"), nil},
		{"≥error", []string{"error", "fatal"}},
		{"≥warn", []string{"warn", "error", "fatal"}},
		{"≥info", []string{"info", "warn", "error", "fatal"}},
	}
}

type pstatus struct {
	fetching bool
	paused   bool
	err      error
	last     time.Time
	gen      int
	added    int // lignes ajoutées depuis le lancement
	dur      time.Duration
}

// Messages internes.
type rowsMsg struct {
	seq    int
	rows   []store.Row
	total  int
	counts map[string]int
	err    error
}

type statsMsg struct {
	seq    int
	levels map[string]int
	hist   []int
	from   time.Time
	to     time.Time
	top    []store.MsgCount
	total  int
	err    error
}

// Model est le modèle racine.
type Model struct {
	ctx       context.Context
	runCtx    context.Context // annulé à chaque rechargement de la config
	runCancel context.CancelFunc
	focused   bool      // le terminal a le focus (faux après un BlurMsg)
	nt        *notifier // décide des notifications d'erreurs
	epoch     int       // incrémenté à chaque rechargement ; préfixe des gen
	run       *scheduler.Runner
	cfgPath   string
	bg        string // séquence ANSI du fond ; vide = transparent
	cfg       *config.Config

	editor    textarea.Model
	edSaved   string // contenu tel qu'enregistré sur disque
	edMsg     string
	edErr     bool
	edQuitArm bool
	st        *store.Store

	names  []string
	status map[string]*pstatus
	counts map[string]int
	sel    int // 0 = tous, i = names[i-1]
	side   bool

	rows   []store.Row
	cursor int
	top    int
	follow bool
	total  int

	queryStr string
	q        query.Query
	levelIdx int
	input    textinput.Model
	loadSeq  int
	loadErr  error

	mode   mode
	detail viewport.Model
	stats  statsMsg

	w, h int
}

// New construit le modèle.
func New(ctx context.Context, run *scheduler.Runner, st *store.Store, cfgPath string, cfg *config.Config, opts Options) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = i18n.T("search.placeholder")
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.SoftWrap = true

	ed := textarea.New()
	ed.ShowLineNumbers = true
	ed.MaxHeight = 0
	ed.CharLimit = 0

	runCtx, runCancel := context.WithCancel(ctx)
	m := Model{
		ctx: ctx, runCtx: runCtx, runCancel: runCancel, run: run, st: st,
		cfgPath: cfgPath, cfg: cfg, editor: ed, focused: true, nt: newNotifier(),
		names:  run.Names(),
		status: map[string]*pstatus{},
		counts: map[string]int{},
		follow: true,
		side:   true, // focus initial sur la liste des providers
		input:  ti,
		detail: vp,
		w:      100, h: 30,
	}
	for _, n := range m.names {
		follow := cfg.Follows(n)
		m.status[n] = &pstatus{fetching: follow, paused: !follow}
	}
	if !opts.Transparent {
		m.bg = bgSequence()
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.load()}
	for _, n := range m.names {
		if !m.status[n].paused { // seuls les providers suivis sont récupérés au lancement
			cmds = append(cmds, m.run.Fetch(m.runCtx, n, 0))
		}
	}
	return tea.Batch(cmds...)
}

// effective retourne la requête avec les filtres de la sidebar et du niveau.
func (m Model) effective() query.Query {
	q := m.q
	if m.sel > 0 {
		q.Provider = m.names[m.sel-1]
	}
	if len(q.Levels) == 0 {
		q.Levels = levelPresets()[m.levelIdx].levels
	}
	return q
}

func (m *Model) load() tea.Cmd {
	m.loadSeq++
	seq, q, st := m.loadSeq, m.effective(), m.st
	return func() tea.Msg {
		rows, err := st.Search(q, rowLimit)
		if err != nil {
			return rowsMsg{seq: seq, err: err}
		}
		total, err := st.Count(q)
		if err != nil {
			return rowsMsg{seq: seq, err: err}
		}
		counts, err := st.ProviderCounts()
		return rowsMsg{seq: seq, rows: rows, total: total, counts: counts, err: err}
	}
}

func (m *Model) loadStats() tea.Cmd {
	m.loadSeq++
	seq, q, st := m.loadSeq, m.effective(), m.st
	return func() tea.Msg {
		out := statsMsg{seq: seq}
		var err error
		if out.levels, err = st.LevelCounts(q); err != nil {
			out.err = err
			return out
		}
		if out.hist, out.from, out.to, err = st.Histogram(q, 60); err != nil {
			out.err = err
			return out
		}
		if out.top, err = st.TopMessages(q, 10); err != nil {
			out.err = err
			return out
		}
		out.total, out.err = st.Count(q)
		return out
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		if m.w < sideMinWidth {
			m.side = false // sidebar masquée : pas de focus invisible
		}
		m.input.SetWidth(max(10, m.w-6))
		m.detail.SetWidth(max(10, m.w-3))
		m.detail.SetHeight(max(1, m.bodyH()-2))
		m.editor.SetWidth(max(20, m.w-2))
		m.editor.SetHeight(max(1, m.bodyH()-2))
		m.clampScroll()
		return m, nil

	case scheduler.FetchedMsg:
		return m.onFetched(msg)

	case tea.FocusMsg:
		m.focused = true
		m.nt.onFocus()
		return m, nil

	case tea.BlurMsg:
		m.focused = false
		return m, nil

	case notifyFlushMsg:
		if msg.Epoch != m.epoch || !m.cfg.NotifyEnabled() {
			return m, nil
		}
		p, wait := m.nt.flush(msg.Name, m.focused, m.cfg.NotifyCooldown, time.Now())
		return m, m.notifyCmd(msg.Name, p, wait)

	case scheduler.TickMsg:
		s := m.status[msg.Name]
		if s == nil || s.gen != msg.Gen || s.paused || s.fetching {
			return m, nil
		}
		s.fetching = true
		return m, m.run.Fetch(m.runCtx, msg.Name, s.gen)

	case rowsMsg:
		if msg.seq != m.loadSeq {
			return m, nil
		}
		m.loadErr = msg.err
		if msg.err != nil {
			return m, nil
		}
		var curID int64 = -1
		if m.cursor >= 0 && m.cursor < len(m.rows) {
			curID = m.rows[m.cursor].ID
		}
		m.rows, m.total, m.counts = msg.rows, msg.total, msg.counts
		m.cursor = max(0, len(m.rows)-1)
		if !m.follow {
			for i := len(m.rows) - 1; i >= 0; i-- {
				if m.rows[i].ID == curID {
					m.cursor = i
					break
				}
			}
		}
		m.clampScroll()
		if m.mode == modeDetail {
			m.refreshDetail()
		}
		return m, nil

	case statsMsg:
		if msg.seq == m.loadSeq {
			m.stats = msg
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.onKey(msg)
	}

	// Autres messages (blink du curseur, paste…) : on les passe aux composants actifs.
	switch m.mode {
	case modeConfig:
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	case modeSearch:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case modeDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onFetched(msg scheduler.FetchedMsg) (tea.Model, tea.Cmd) {
	s := m.status[msg.Name]
	if s == nil || msg.Gen>>20 != m.epoch {
		return m, nil
	}
	cmds := []tea.Cmd{}
	if msg.Gen == s.gen {
		s.fetching = false
		if !s.paused {
			cmds = append(cmds, m.run.Tick(msg.Name, s.gen))
		}
	}
	s.err, s.last, s.dur = msg.Err, msg.At, msg.Dur
	s.added += msg.New
	if m.cfg.NotifyEnabled() {
		p, wait := m.nt.onFetch(msg, m.focused, m.cfg.NotifyCooldown, time.Now())
		cmds = append(cmds, m.notifyCmd(msg.Name, p, wait))
	}
	if msg.New > 0 {
		cmds = append(cmds, m.load())
		if m.mode == modeStats {
			cmds = append(cmds, m.loadStats())
		}
	}
	return m, tea.Batch(cmds...)
}

// refreshAll force une récupération immédiate de tous les providers non pausés.
func (m *Model) refreshAll() tea.Cmd {
	var cmds []tea.Cmd
	for _, n := range m.names {
		s := m.status[n]
		if s.paused {
			continue
		}
		s.gen++
		s.fetching = true
		cmds = append(cmds, m.run.Fetch(m.runCtx, n, s.gen))
	}
	return tea.Batch(cmds...)
}

func (m *Model) togglePause() tea.Cmd {
	targets := m.names
	if m.sel > 0 {
		targets = []string{m.names[m.sel-1]}
	}
	// Si au moins un provider ciblé est actif, on met tout en pause, sinon on reprend.
	pause := false
	for _, n := range targets {
		if !m.status[n].paused {
			pause = true
		}
	}
	var cmds []tea.Cmd
	for _, n := range targets {
		s := m.status[n]
		s.gen++
		s.paused = pause
		if !pause {
			s.fetching = true
			cmds = append(cmds, m.run.Fetch(m.runCtx, n, s.gen))
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeConfig:
		return m.onConfigKey(msg)
	case modeSearch:
		switch key {
		case "enter":
			m.queryStr = strings.TrimSpace(m.input.Value())
			m.q = query.Parse(m.queryStr)
			m.mode = modeLog
			m.input.Blur()
			m.follow = true
			return m, m.load()
		case "esc":
			m.input.SetValue(m.queryStr)
			m.input.Blur()
			m.mode = modeLog
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case modeHelp:
		m.mode = modeLog
		return m, nil

	case modeStats:
		switch key {
		case "esc", "s", "q", "enter":
			m.mode = modeLog
		}
		return m, nil

	case modeDetail:
		switch key {
		case "esc", "enter", "q":
			m.mode = modeLog
			return m, nil
		case "left", "h":
			m.moveCursor(-1)
			m.refreshDetail()
			return m, nil
		case "right", "l":
			m.moveCursor(1)
			m.refreshDetail()
			return m, nil
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	// modeLog
	if m.side {
		switch key {
		case "up", "k":
			if m.sel > 0 {
				m.sel--
				m.follow = true
				return m, m.load()
			}
			return m, nil
		case "down", "j":
			if m.sel < len(m.names) {
				m.sel++
				m.follow = true
				return m, m.load()
			}
			return m, nil
		case "tab", "enter", "esc", "right", "l":
			m.side = false
			return m, nil
		}
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "tab":
		m.side = m.w >= sideMinWidth
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		m.moveCursor(-m.listH() / 2)
	case "pgdown", "ctrl+d":
		m.moveCursor(m.listH() / 2)
	case "home", "g":
		m.cursor, m.follow = 0, false
		m.clampScroll()
	case "end", "G", "f":
		m.cursor, m.follow = max(0, len(m.rows)-1), true
		m.clampScroll()
	case "enter":
		if len(m.rows) > 0 {
			m.mode = modeDetail
			m.refreshDetail()
		}
	case "/":
		m.mode = modeSearch
		m.input.SetValue(m.queryStr)
		return m, m.input.Focus()
	case "esc":
		if m.queryStr != "" {
			m.queryStr, m.q = "", query.Query{}
			m.input.SetValue("")
			m.follow = true
			return m, m.load()
		}
	case "l":
		m.levelIdx = (m.levelIdx + 1) % len(levelPresets())
		m.follow = true
		return m, m.load()
	case "r":
		return m, m.refreshAll()
	case "p":
		return m, m.togglePause()
	case "s":
		m.mode = modeStats
		return m, m.loadStats()
	case "?":
		m.mode = modeHelp
	case "c":
		return m, m.openEditor()
	}
	return m, nil
}

func (m *Model) moveCursor(d int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+d, 0), len(m.rows)-1)
	m.follow = m.cursor == len(m.rows)-1
	m.clampScroll()
}

func (m Model) bodyH() int { return max(5, m.h-3) }

// listH est le nombre de lignes de log visibles (cadre déduit).
func (m Model) listH() int { return m.bodyH() - 2 }

func (m *Model) clampScroll() {
	h := m.listH()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	m.top = max(0, min(m.top, max(0, len(m.rows)-h)))
	if m.cursor < m.top {
		m.top = m.cursor
	}
}

func contextWithCancel(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// notifyCmd transforme la décision du notifier en commande : envoi immédiat,
// ou programmation d'un flush à l'échéance du délai de grâce.
func (m Model) notifyCmd(name string, p *notifyPayload, wait time.Duration) tea.Cmd {
	switch {
	case p != nil:
		return notify.Send(p.Title, p.Body)
	case wait > 0:
		epoch := m.epoch
		return tea.Tick(wait, func(time.Time) tea.Msg { return notifyFlushMsg{Name: name, Epoch: epoch} })
	}
	return nil
}
