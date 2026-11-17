package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/kevin/log-tui/internal/scheduler"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func fm(name string, errs int, sample string) scheduler.FetchedMsg {
	return scheduler.FetchedMsg{Name: name, New: errs, Errors: errs, Sample: sample}
}

func TestNotifierSeedsHistory(t *testing.T) {
	n := newNotifier()
	if p, _ := n.onFetch(fm("api", 5, "old"), false, 10*time.Second, t0); p != nil {
		t.Fatal("l'historique ne doit pas notifier")
	}
	p, _ := n.onFetch(fm("api", 2, "boom"), false, 10*time.Second, t0)
	if p == nil || p.Title != "log-tui · api" || p.Body != "2 erreurs — boom" {
		t.Fatalf("%+v", p)
	}
}

func TestNotifierFailedFirstFetchDoesNotSeed(t *testing.T) {
	n := newNotifier()
	n.onFetch(scheduler.FetchedMsg{Name: "api", Err: errors.New("ssh")}, false, time.Second, t0)
	if p, _ := n.onFetch(fm("api", 3, "hist"), false, time.Second, t0); p != nil {
		t.Fatal("la 1re récupération réussie reste l'historique")
	}
}

func TestNotifierFocusedAndNoErrors(t *testing.T) {
	n := newNotifier()
	n.onFetch(fm("api", 0, ""), false, time.Second, t0) // seed
	if p, w := n.onFetch(fm("api", 3, "x"), true, time.Second, t0); p != nil || w != 0 {
		t.Fatal("focus : pas de notification")
	}
	if p, _ := n.onFetch(fm("api", 0, ""), false, time.Second, t0); p != nil {
		t.Fatal("sans erreur : pas de notification")
	}
}

func TestNotifierCooldownAndFlush(t *testing.T) {
	n := newNotifier()
	cd := 10 * time.Second
	n.onFetch(fm("api", 0, ""), false, cd, t0) // seed
	if p, _ := n.onFetch(fm("api", 1, "a"), false, cd, t0); p == nil || p.Body != "1 erreur — a" {
		t.Fatalf("%+v", p)
	}
	// Dans le délai de grâce : mis en attente, avec le temps restant.
	p, wait := n.onFetch(fm("api", 2, "b"), false, cd, t0.Add(4*time.Second))
	if p != nil || wait != 6*time.Second {
		t.Fatalf("%v %v", p, wait)
	}
	p, _ = n.onFetch(fm("api", 1, "c"), false, cd, t0.Add(5*time.Second))
	if p != nil {
		t.Fatal("toujours en attente")
	}
	// Flush à l'échéance : 3 erreurs cumulées, 1er message conservé.
	p, _ = n.flush("api", false, cd, t0.Add(10*time.Second))
	if p == nil || p.Body != "3 erreurs — b" {
		t.Fatalf("%+v", p)
	}
	if p, _ = n.flush("api", false, cd, t0.Add(30*time.Second)); p != nil {
		t.Fatal("rien en attente")
	}
	// Providers indépendants.
	n.onFetch(fm("db", 0, ""), false, cd, t0)
	if p, _ := n.onFetch(fm("db", 1, "z"), false, cd, t0.Add(time.Second)); p == nil {
		t.Fatal("db n'est pas limité par api")
	}
}

func TestNotifierFocusClearsPendingAndResetReseeds(t *testing.T) {
	n := newNotifier()
	cd := 10 * time.Second
	n.onFetch(fm("api", 0, ""), false, cd, t0)
	n.onFetch(fm("api", 1, "a"), false, cd, t0)
	n.onFetch(fm("api", 1, "b"), false, cd, t0.Add(time.Second)) // en attente
	n.onFocus()
	if p, _ := n.flush("api", false, cd, t0.Add(20*time.Second)); p != nil {
		t.Fatal("pending vidé au focus")
	}
	n.reset()
	if p, _ := n.onFetch(fm("api", 4, "hist"), false, cd, t0.Add(time.Minute)); p != nil {
		t.Fatal("après reset, l'historique est de nouveau ignoré")
	}
}
