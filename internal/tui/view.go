package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kevin/log-tui/internal/i18n"
	"github.com/kevin/log-tui/internal/store"
)

const (
	sideWidth    = 30
	sideMinWidth = 90 // largeur de terminal minimale pour afficher la sidebar
)

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.header() + "\n")

	bodyH := m.bodyH()
	var body []string
	switch m.mode {
	case modeDetail:
		dl := strings.Split(m.detail.View(), "\n")
		for i := range dl {
			dl[i] = " " + dl[i]
		}
		body = panel(i18n.T("detail.title"), dl, m.w, bodyH, accent)
	case modeStats:
		body = panel(i18n.T("stats.panel"), m.statsLines(m.w-2), m.w, bodyH, accent)
	case modeHelp:
		body = panel(i18n.T("help.title"), helpLines(), m.w, bodyH, accent)
	case modeConfig:
		body = panel(i18n.T("config.title")+" · "+m.cfgPath, strings.Split(m.editor.View(), "\n"), m.w, bodyH, accent)
	default:
		body = m.logAndSide(bodyH)
	}
	for i := 0; i < bodyH; i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		b.WriteString(fit(line, m.w) + "\n")
	}
	b.WriteString(fit(modeBadge(m.mode)+" "+m.footer(), m.w) + "\n")
	b.WriteString(fit(keyHints(m.hints()), m.w))

	out := b.String()
	if m.bg != "" {
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			lines[i] = paintBackground(l, m.bg)
		}
		out = strings.Join(lines, "\n")
	}
	v := tea.NewView(out)
	if m.mode == modeConfig {
		v.Cursor = m.editor.Cursor()
		if v.Cursor != nil {
			v.Cursor.X++    // bordure gauche
			v.Cursor.Y += 2 // en-tête + bordure haute
		}
	}
	v.AltScreen = true
	v.ReportFocus = true // FocusMsg / BlurMsg, pour les notifications
	v.WindowTitle = "log-tui"
	return v
}

func (m Model) header() string {
	chips := []string{brandSt.Render("◆ log-tui")}
	n := i18n.T("header.entries", m.total)
	if len(m.rows) < m.total {
		n += i18n.T("header.shown", len(m.rows))
	}
	chips = append(chips, chipSt.Render(n))
	if m.sel > 0 {
		chips = append(chips, chipProv.Render(i18n.T("header.provider")+" "+m.names[m.sel-1]))
	}
	if m.levelIdx > 0 {
		chips = append(chips, chipLevel.Render(i18n.T("header.level")+" "+levelPresets()[m.levelIdx].label))
	}
	if m.queryStr != "" {
		chips = append(chips, chipWarn.Render(i18n.T("header.filter")))
	}
	switch {
	case m.mode == modeConfig && m.editor.Value() != m.edSaved:
		chips = append(chips, chipWarn.Render(i18n.T("header.modified")))
	case m.mode == modeDetail:
		chips = append(chips, chipSt.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.rows))))
	}
	if m.follow {
		chips = append(chips, chipLive.Render(i18n.T("header.follow")))
	} else {
		chips = append(chips, chipPause.Render(i18n.T("header.frozen")))
	}
	line := strings.Join(chips, barStyle.Render(" "))
	if pad := m.w - lipgloss.Width(line); pad > 0 {
		line += barStyle.Render(strings.Repeat(" ", pad))
	}
	return fit(line, m.w)
}

func (m Model) footer() string {
	switch {
	case m.mode == modeConfig:
		if m.edErr {
			return errStyle.Render("✗ " + clean(m.edMsg))
		}
		if m.edMsg != "" {
			return okStyle.Render("✓ " + clean(m.edMsg))
		}
		return dim.Render(m.cfgPath)
	case m.mode == modeSearch:
		return m.input.View()
	case m.loadErr != nil:
		return errStyle.Render(i18n.T("footer.search") + " " + m.loadErr.Error())
	}
	// Première erreur de provider pertinente.
	for _, n := range m.names {
		if m.sel > 0 && m.names[m.sel-1] != n {
			continue
		}
		if s := m.status[n]; s.err != nil {
			return errStyle.Render("✗ " + n + ": " + clean(s.err.Error()))
		}
	}
	if m.queryStr != "" {
		return warnSt.Render(i18n.T("footer.filter")+" ") + m.queryStr + dim.Render("   "+i18n.T("footer.clear"))
	}
	if m.mode != modeLog {
		return ""
	}
	return dim.Render(i18n.T("footer.hint"))
}

func (m Model) hints() string {
	switch m.mode {
	case modeSearch:
		return i18n.T("hints.search")
	case modeDetail:
		return i18n.T("hints.detail")
	case modeConfig:
		return i18n.T("hints.config")
	case modeStats, modeHelp:
		return i18n.T("hints.back")
	}
	return i18n.T("hints.default")
}

// logAndSide compose la sidebar (si la place le permet) et la liste de logs, chacune dans son cadre.
func (m Model) logAndSide(h int) []string {
	logW := m.w
	var side []string
	if m.w >= sideMinWidth {
		logW = m.w - sideWidth
		sc := borderIdle
		if m.side {
			sc = accent
		}
		side = panel(i18n.T("side.title"), m.sidebar(sideWidth-2, h-2), sideWidth, h, sc)
	}
	lc := accent
	if m.side && side != nil {
		lc = borderIdle
	}
	ttl := i18n.T("logs.title")
	if m.queryStr != "" {
		ttl += " · " + m.queryStr
	}
	logPanel := panel(ttl, m.logLines(logW-2, h-2), logW, h, lc)
	if side == nil {
		return logPanel
	}
	out := make([]string, h)
	for i := range out {
		out[i] = side[i] + logPanel[i]
	}
	return out
}

func (m Model) logLines(w, h int) []string {
	out := make([]string, h)
	if len(m.rows) == 0 {
		msg := i18n.T("empty.none")
		if m.total == 0 && m.queryStr == "" && m.sel == 0 && m.levelIdx == 0 {
			msg = i18n.T("empty.waiting")
			if m.noneFollowed() {
				msg = i18n.T("empty.nofollow")
			}
		}
		out[0] = dim.Render(" " + msg)
		return out
	}
	for i := 0; i < h; i++ {
		idx := m.top + i
		if idx >= len(m.rows) {
			out[i] = ""
			continue
		}
		out[i] = rowLine(m.rows[idx], w, idx == m.cursor, m.sel == 0 && len(m.names) > 1)
	}
	return out
}

func rowLine(r store.Row, w int, sel, showProv bool) string {
	ts := r.TS.Format("01-02 15:04:05")
	lvl := fmt.Sprintf("%-5s", strings.ToUpper(r.Level))
	if len(lvl) > 5 {
		lvl = lvl[:5]
	}
	msg := clean(r.Msg)
	extra := ""
	if r.Fields != "" {
		extra = clean(compactFields(r.Fields))
	}
	prov := ""
	if showProv {
		prov = fit(clean(r.Provider), 10)
	}

	gut := " "
	if r.Level != "" {
		gut = levelStyle(r.Level).Render("▌")
	}
	if sel {
		gut = levelStyle(r.Level).Background(lipgloss.Color("238")).Render("▌")
		if r.Level == "" {
			gut = selected.Render(" ")
		}
		line := " " + ts + " "
		if showProv {
			line += prov + " "
		}
		line += lvl + " " + msg
		if extra != "" {
			line += "  " + extra
		}
		return gut + selected.Render(fit(line, w-1))
	}
	line := gut + " " + dim.Render(ts) + " "
	if showProv {
		line += provStyle(r.Provider).Render(prov) + " "
	}
	line += levelStyle(r.Level).Render(lvl) + " " + msg
	if extra != "" {
		line += "  " + dim.Render(extra)
	}
	return fit(line, w)
}

func (m Model) sidebar(w, h int) []string {
	lines := []string{}
	total := 0
	for _, c := range m.counts {
		total += c
	}
	add := func(idx int, dot, name string, count int) {
		label := fmt.Sprintf(" %s %s", dot, name)
		cnt := fmt.Sprintf("%d ", count)
		pad := w - lipgloss.Width(label) - lipgloss.Width(cnt)
		if pad < 1 {
			label = fit(label, w-lipgloss.Width(cnt)-1)
			pad = 1
		}
		line := label + strings.Repeat(" ", pad) + cnt
		if idx == m.sel {
			if m.side {
				line = selFocus.Render(fit(ansiPlain(line), w))
			} else {
				line = selected.Render(fit(ansiPlain(line), w))
			}
		}
		lines = append(lines, line)
	}
	add(0, " ", i18n.T("side.all"), total)
	for i, n := range m.names {
		add(i+1, m.dot(n), n, m.counts[n])
	}

	if m.sel > 0 {
		n := m.names[m.sel-1]
		s := m.status[n]
		lines = append(lines, "", bold.Render(" "+n), dim.Render(" "+m.run.Kind(n)))
		lines = append(lines, dim.Render(i18n.T("side.every", m.run.Interval(n))))
		switch {
		case s.paused && s.last.IsZero():
			lines = append(lines, dim.Render(i18n.T("side.notfollowed")))
		case s.paused:
			lines = append(lines, warnSt.Render(i18n.T("side.paused")))
		case s.fetching:
			lines = append(lines, warnSt.Render(i18n.T("side.fetching")))
		case !s.last.IsZero():
			lines = append(lines, dim.Render(i18n.T("side.last", ago(s.last), s.dur.Round(time.Millisecond))))
		}
		lines = append(lines, dim.Render(i18n.T("side.added", s.added)))
		if s.err != nil {
			for _, l := range wrap(clean(s.err.Error()), w-2) {
				lines = append(lines, errStyle.Render(" "+l))
			}
		}
	}
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = fit(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// noneFollowed indique qu'aucun provider n'a jamais été récupéré ni n'est en cours.
func (m Model) noneFollowed() bool {
	for _, s := range m.status {
		if !s.paused || !s.last.IsZero() {
			return false
		}
	}
	return true
}

func (m Model) dot(name string) string {
	s := m.status[name]
	switch {
	case s.paused && s.last.IsZero():
		return dim.Render("○")
	case s.paused:
		return warnSt.Render("○") // en pause après avoir été suivi : même pastille, en jaune
	case s.fetching:
		return warnSt.Render("◌")
	case s.err != nil:
		return errStyle.Render("●")
	case s.last.IsZero():
		return dim.Render("○")
	}
	return okStyle.Render("●")
}

func ago(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func wrap(s string, w int) []string {
	if w < 8 {
		return []string{s}
	}
	var out []string
	r := []rune(s)
	for len(r) > w {
		out = append(out, string(r[:w]))
		r = r[w:]
	}
	return append(out, string(r))
}

// ansiPlain retire les styles pour pouvoir appliquer un fond homogène.
func ansiPlain(s string) string { return clean(s) }

func helpLines() []string {
	t := i18n.T
	return []string{
		bold.Render(" " + t("help.keys")),
		"",
		"  ↑/↓ j/k        " + t("help.nav"),
		"  g / G          " + t("help.topbottom"),
		"  enter          " + t("help.enter"),
		"  tab            " + t("help.tab"),
		"  /              " + t("help.search"),
		"  l              " + t("help.level"),
		"  r              " + t("help.refresh"),
		"  s              " + t("help.stats"),
		"  q              " + t("help.quit"),
		"",
		bold.Render(" " + t("help.notif")),
		"",
		"  " + t("help.notif1"),
		"  " + t("help.notif2"),
		"  " + t("help.notif3"),
		"",
		bold.Render(" " + t("help.query")),
		"",
		"  timeout               " + t("help.q.substr"),
		`  "connection reset"    ` + t("help.q.phrase"),
		"  -healthcheck          " + t("help.q.exclude"),
		"  level:error,warn      " + t("help.q.level"),
		"  provider:api          " + t("help.q.provider"),
		"  since:15m             " + t("help.q.since"),
		"  user.id:42            " + t("help.q.field"),
	}
}
