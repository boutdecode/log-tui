// Package notify envoie des notifications à l'utilisateur : notification du
// bureau via notify-send, sinon séquence de notification du terminal.
package notify

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Points d'injection pour les tests.
var (
	lookPath = exec.LookPath
	getenv   = os.Getenv
	run      = func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	}
)

const (
	maxTitle = 80
	maxBody  = 200
)

// Sanitize rend un texte issu des logs (non fiable) sûr pour une notification :
// séquences ANSI et caractères de contrôle retirés (ESC/BEL pourraient
// terminer ou détourner une séquence OSC), espaces compactés, longueur bornée.
func Sanitize(s string, max int) string {
	s = ansi.Strip(s)
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || (r >= 0x80 && r < 0xa0) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max-1]) + "…"
	}
	return s
}

func inSSH() bool { return getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" }

// Deliver tente la notification du bureau. Si elle n'est pas possible
// (session SSH, notify-send absent ou en échec), il retourne la séquence
// d'échappement à écrire sur le terminal en repli ; "" si le bureau a été notifié.
func Deliver(title, body string) (fallbackSeq string) {
	title, body = Sanitize(title, maxTitle), Sanitize(body, maxBody)

	// Sur un serveur distant, notify-send notifierait le bureau du serveur.
	if !inSSH() {
		if path, err := lookPath("notify-send"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := run(ctx, path, "-a", "log-tui", "-u", "critical", "-i", "dialog-error", "--", title, body)
			if err == nil {
				return ""
			}
		}
	}
	return terminalSequence(title, body)
}

func terminalSequence(title, body string) string {
	var b strings.Builder
	if getenv("KITTY_WINDOW_ID") != "" { // protocole OSC 99 de kitty
		b.WriteString(ansi.DesktopNotification(title, "i=logtui", "d=0", "p=title"))
		b.WriteString(ansi.DesktopNotification(body, "i=logtui", "d=1", "p=body"))
	} else {
		b.WriteString(ansi.Notify(title + " — " + body)) // OSC 9 (iTerm2, WezTerm, Ghostty…)
	}
	b.WriteString("\a") // bip : dernier recours visible dans la barre de titre/tmux
	return b.String()
}

// Send retourne une commande Bubble Tea qui envoie la notification.
func Send(title, body string) tea.Cmd {
	return func() tea.Msg {
		if seq := Deliver(title, body); seq != "" {
			return tea.Raw(seq)()
		}
		return nil
	}
}
