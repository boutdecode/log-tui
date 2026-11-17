package tui

import (
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	dim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	bold     = lipgloss.NewStyle().Bold(true)
	errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	okStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnSt   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	title    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62")).Padding(0, 1)
	selected = lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("15"))
	selFocus = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("15"))

	levelStyles = map[string]lipgloss.Style{
		"trace": lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		"debug": lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
		"info":  lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		"warn":  lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		"error": lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		"fatal": lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("1")),
	}
	accent     = lipgloss.Color("99")
	borderIdle = lipgloss.Color("240")
	barBg      = lipgloss.Color("235")

	barStyle  = lipgloss.NewStyle().Background(barBg)
	brandSt   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("98")).Padding(0, 1)
	chipSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238")).Padding(0, 1)
	chipProv  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("61")).Padding(0, 1)
	chipLevel = lipgloss.NewStyle().Foreground(lipgloss.Color("232")).Background(lipgloss.Color("179")).Padding(0, 1)
	chipLive  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("232")).Background(lipgloss.Color("78")).Padding(0, 1)
	chipPause = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("240")).Padding(0, 1)
	chipWarn  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("232")).Background(lipgloss.Color("208")).Padding(0, 1)
	keySt     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	hintSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	modeColors = map[mode]string{
		modeLog: "62", modeSearch: "172", modeDetail: "31",
		modeStats: "97", modeHelp: "66", modeConfig: "166",
	}
	modeNames = map[mode]string{
		modeLog: "LOGS", modeSearch: "RECHERCHE", modeDetail: "DÉTAIL",
		modeStats: "STATS", modeHelp: "AIDE", modeConfig: "CONFIG",
	}

	provPalette = []string{"14", "13", "12", "10", "11", "9", "6", "5", "4", "2"}
)

func levelStyle(l string) lipgloss.Style {
	if s, ok := levelStyles[l]; ok {
		return s
	}
	return dim
}

func provStyle(name string) lipgloss.Style {
	h := fnv.New32a()
	h.Write([]byte(name))
	return lipgloss.NewStyle().Foreground(lipgloss.Color(provPalette[int(h.Sum32())%len(provPalette)]))
}

// fit tronque (avec …) ou complète par des espaces pour obtenir exactement w colonnes.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// clean retire les séquences ANSI et caractères de contrôle d'une ligne de log.
func clean(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r == '\n':
			return '⏎'
		case r < 32 || r == 127:
			return -1
		}
		return r
	}, s)
}

func modeBadge(m mode) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).
		Background(lipgloss.Color(modeColors[m])).Padding(0, 1).Render(modeNames[m])
}

// panel dessine un cadre arrondi avec un titre incrusté dans la bordure haute.
// lines contient le contenu intérieur (w-2 colonnes, h-2 lignes) ; le résultat
// fait exactement h lignes de w colonnes.
func panel(titleText string, lines []string, w, h int, c color.Color) []string {
	if w < 4 || h < 2 {
		return make([]string, max(h, 0))
	}
	bs := lipgloss.NewStyle().Foreground(c)
	inner := w - 2
	t := ""
	if titleText != "" {
		t = " " + ansi.Truncate(titleText, max(0, inner-4), "…") + " "
	}
	top := bs.Render("╭─") + bold.Foreground(c).Render(t) +
		bs.Render(strings.Repeat("─", max(0, inner-1-lipgloss.Width(t)))+"╮")
	out := make([]string, 0, h)
	out = append(out, top)
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, bs.Render("│")+fit(l, inner)+bs.Render("│"))
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

// keyHints met en valeur la touche de chaque indication "touche texte" / "touche: texte".
func keyHints(h string) string {
	parts := strings.Split(h, " · ")
	for i, p := range parts {
		k, rest := p, ""
		if j := strings.Index(p, ": "); j >= 0 {
			k, rest = p[:j], p[j+1:]
		} else if j := strings.IndexByte(p, ' '); j >= 0 {
			k, rest = p[:j], p[j:]
		}
		parts[i] = keySt.Render(k) + hintSt.Render(rest)
	}
	return strings.Join(parts, hintSt.Render("  "))
}
