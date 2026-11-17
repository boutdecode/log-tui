package tui

import (
	"fmt"
	"time"

	"github.com/kevin/log-tui/internal/i18n"
	"github.com/kevin/log-tui/internal/scheduler"
)

// notifyFlushMsg demande d'envoyer les erreurs mises en attente par le délai de grâce.
type notifyFlushMsg struct {
	Name  string
	Epoch int
}

type notifyPayload struct{ Title, Body string }

type pendingErrs struct {
	n      int
	sample string
}

// notifier décide quand notifier les erreurs : historique ignoré, lots
// regroupés par provider, délai minimum entre deux notifications d'un même provider.
type notifier struct {
	last    map[string]time.Time
	pending map[string]pendingErrs
	seeded  map[string]bool
}

func newNotifier() *notifier {
	n := &notifier{}
	n.reset()
	return n
}

// reset oublie tout (providers reconstruits après un rechargement de config).
func (n *notifier) reset() {
	n.last = map[string]time.Time{}
	n.pending = map[string]pendingErrs{}
	n.seeded = map[string]bool{}
}

// onFetch traite un résultat de récupération. Retourne soit une notification à
// envoyer, soit un délai au bout duquel appeler flush (erreurs en attente).
func (n *notifier) onFetch(msg scheduler.FetchedMsg, focused bool, cooldown time.Duration, now time.Time) (*notifyPayload, time.Duration) {
	if msg.Err != nil {
		return nil, 0
	}
	if !n.seeded[msg.Name] { // première récupération réussie = historique
		n.seeded[msg.Name] = true
		return nil, 0
	}
	if focused || msg.Errors == 0 {
		return nil, 0
	}
	p := n.pending[msg.Name]
	p.n += msg.Errors
	if p.sample == "" {
		p.sample = msg.Sample
	}
	n.pending[msg.Name] = p
	return n.trySend(msg.Name, cooldown, now)
}

// flush envoie les erreurs en attente si le délai est écoulé et que l'app n'a toujours pas le focus.
func (n *notifier) flush(name string, focused bool, cooldown time.Duration, now time.Time) (*notifyPayload, time.Duration) {
	if focused {
		delete(n.pending, name)
		return nil, 0
	}
	return n.trySend(name, cooldown, now)
}

// onFocus : l'utilisateur est revenu, il voit les erreurs à l'écran.
func (n *notifier) onFocus() { n.pending = map[string]pendingErrs{} }

func (n *notifier) trySend(name string, cooldown time.Duration, now time.Time) (*notifyPayload, time.Duration) {
	p := n.pending[name]
	if p.n == 0 {
		return nil, 0
	}
	if last, ok := n.last[name]; ok {
		if wait := cooldown - now.Sub(last); wait > 0 {
			return nil, wait
		}
	}
	delete(n.pending, name)
	n.last[name] = now
	title := fmt.Sprintf("log-tui · %s", name)
	count := i18n.T("notif.errors", p.n)
	if p.n == 1 {
		count = i18n.T("notif.error1")
	}
	return &notifyPayload{Title: title, Body: count + " — " + p.sample}, 0
}
