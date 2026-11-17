package tui

import (
	"os"
	"strconv"
	"strings"
)

// Options configure l'apparence de la TUI.
type Options struct {
	// Transparent conserve le fond du terminal au lieu de peindre un fond coloré.
	Transparent bool
}

// bgSequence retourne la séquence ANSI du fond de l'application.
func bgSequence() string {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return "\x1b[48;2;26;27;38m" // #1a1b26
	}
	return "\x1b[48;5;234m"
}

// paintBackground applique le fond bg à toute la ligne : la séquence est
// réémise après chaque séquence SGR qui remet le fond par défaut (0, 49), de
// sorte que les styles imbriqués ne « percent » pas le fond.
func paintBackground(line, bg string) string {
	var b strings.Builder
	b.WriteString(bg)
	for i := 0; i < len(line); {
		if line[i] != 0x1b || i+1 >= len(line) || line[i+1] != '[' {
			b.WriteByte(line[i])
			i++
			continue
		}
		// Séquence CSI : jusqu'à l'octet final (0x40–0x7e).
		j := i + 2
		for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
			j++
		}
		if j >= len(line) {
			b.WriteString(line[i:])
			break
		}
		seq := line[i : j+1]
		b.WriteString(seq)
		if line[j] == 'm' && resetsBackground(line[i+2:j]) {
			b.WriteString(bg)
		}
		i = j + 1
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// resetsBackground indique si les paramètres SGR remettent le fond par défaut.
func resetsBackground(params string) bool {
	if params == "" {
		return true
	}
	parts := strings.Split(params, ";")
	for k := 0; k < len(parts); k++ {
		n, err := strconv.Atoi(parts[k])
		if err != nil && parts[k] != "" {
			continue
		}
		switch {
		case parts[k] == "" || n == 0 || n == 49:
			return true
		case n == 38 || n == 48: // couleur étendue : sauter ses arguments
			if k+1 < len(parts) && parts[k+1] == "5" {
				k += 2
			} else if k+1 < len(parts) && parts[k+1] == "2" {
				k += 4
			}
		}
	}
	return false
}
