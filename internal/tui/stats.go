package tui

import (
	"fmt"
	"strings"
	"time"
)

var sparkChars = []rune("▁▂▃▄▅▆▇█")

func sparkline(vals []int) string {
	mx := 0
	for _, v := range vals {
		mx = max(mx, v)
	}
	var b strings.Builder
	for _, v := range vals {
		if v == 0 || mx == 0 {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(sparkChars[min(len(sparkChars)-1, v*len(sparkChars)/(mx+1))])
	}
	return b.String()
}

func (m Model) statsLines(w int) []string {
	s := m.stats
	if s.err != nil {
		return []string{errStyle.Render(" " + s.err.Error())}
	}
	lines := []string{bold.Render(fmt.Sprintf(" Statistiques — %d entrées", s.total)), ""}

	lines = append(lines, bold.Render(" Par niveau"))
	order := []string{"fatal", "error", "warn", "info", "debug", "trace", ""}
	barW := max(10, min(50, w-30))
	mx := 0
	for _, c := range s.levels {
		mx = max(mx, c)
	}
	for _, l := range order {
		c, ok := s.levels[l]
		if !ok {
			continue
		}
		name := l
		if name == "" {
			name = "?"
		}
		n := 0
		if mx > 0 {
			n = max(1, c*barW/mx)
		}
		lines = append(lines, fmt.Sprintf("  %-6s %s %d", name,
			levelStyle(l).Render(strings.Repeat("█", n)), c))
	}

	lines = append(lines, "", bold.Render(" Activité"))
	if !s.from.IsZero() {
		sp := sparkline(s.hist)
		lines = append(lines, "  "+okStyle.Render(sp))
		lines = append(lines, dim.Render(fmt.Sprintf("  %s → %s  (%d buckets de %s)",
			s.from.Format("01-02 15:04:05"), s.to.Format("01-02 15:04:05"),
			len(s.hist), (s.to.Sub(s.from)/time.Duration(max(1, len(s.hist)))).Round(100*time.Millisecond))))
	} else {
		lines = append(lines, dim.Render("  (aucune donnée)"))
	}

	lines = append(lines, "", bold.Render(" Messages les plus fréquents"))
	for _, t := range s.top {
		lines = append(lines, fmt.Sprintf("  %6d  %s", t.Count, clean(t.Msg)))
	}
	return lines
}
