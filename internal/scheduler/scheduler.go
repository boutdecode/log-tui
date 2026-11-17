// Package scheduler orchestre la récupération des providers et leur stockage.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/parse"
	"github.com/kevin/log-tui/internal/provider"
	"github.com/kevin/log-tui/internal/store"
)

// FetchedMsg est émis à la fin de chaque récupération.
type FetchedMsg struct {
	Name string
	Gen  int
	New  int
	// Errors compte les lignes de niveau error/fatal du lot, Sample est le message de la première.
	Errors int
	Sample string
	Err    error
	At     time.Time
	Dur    time.Duration
}

// TickMsg signale que l'intervalle d'un provider est écoulé.
type TickMsg struct {
	Name string
	Gen  int
}

type entry struct {
	cfg  config.Provider
	prov provider.Provider
	mu   sync.Mutex // sérialise les récupérations d'un même provider
}

// Runner gère l'ensemble des providers.
type Runner struct {
	store     *store.Store
	retention time.Duration
	byName    map[string]*entry
	order     []string
}

// New instancie les providers de la config.
func New(cfg *config.Config, st *store.Store) (*Runner, error) {
	r := &Runner{store: st, retention: cfg.Retention, byName: map[string]*entry{}}
	for _, pc := range cfg.Providers {
		p, err := provider.New(pc)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", pc.Name, err)
		}
		r.byName[pc.Name] = &entry{cfg: pc, prov: p}
		r.order = append(r.order, pc.Name)
	}
	return r, nil
}

// Names retourne les providers dans l'ordre de la config.
func (r *Runner) Names() []string { return r.order }

// Interval retourne l'intervalle d'un provider.
func (r *Runner) Interval(name string) time.Duration { return r.byName[name].cfg.Interval }

// Kind retourne "type host" pour l'affichage.
func (r *Runner) Kind(name string) string {
	c := r.byName[name].cfg
	if c.Host != "" {
		return c.Type + " " + c.Host
	}
	return c.Type
}

// Close ferme les connexions.
func (r *Runner) Close() {
	for _, e := range r.byName {
		e.prov.Close()
	}
}

// Tick programme le prochain tick d'un provider.
func (r *Runner) Tick(name string, gen int) tea.Cmd {
	return tea.Tick(r.byName[name].cfg.Interval, func(time.Time) tea.Msg {
		return TickMsg{Name: name, Gen: gen}
	})
}

// Fetch récupère, analyse et stocke les nouvelles lignes d'un provider.
func (r *Runner) Fetch(ctx context.Context, name string, gen int) tea.Cmd {
	e := r.byName[name]
	return func() tea.Msg {
		e.mu.Lock()
		defer e.mu.Unlock()
		start := time.Now()
		res, err := r.fetch(ctx, e)
		return FetchedMsg{Name: name, Gen: gen, New: res.New, Errors: res.Errors, Sample: res.Sample,
			Err: err, At: time.Now(), Dur: time.Since(start)}
	}
}

type result struct {
	New, Errors int
	Sample      string
}

func (r *Runner) fetch(ctx context.Context, e *entry) (result, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var st provider.State
	if raw, err := r.store.LoadState(e.cfg.Name); err != nil {
		return result{}, err
	} else if raw != "" {
		if err := json.Unmarshal([]byte(raw), &st); err != nil {
			st = provider.State{} // état corrompu : on repart de zéro
		}
	}
	lines, err := e.prov.Fetch(ctx, &st)
	if err != nil {
		return result{}, err
	}
	now := time.Now()
	es := make([]parse.Entry, 0, len(lines))
	var res result
	for _, l := range lines {
		en := parse.Line(l, now)
		if en.Level == "error" || en.Level == "fatal" {
			if res.Errors == 0 {
				res.Sample = en.Msg
			}
			res.Errors++
		}
		es = append(es, en)
	}
	res.New = len(es)
	// Insertion puis sauvegarde de l'état : si l'insertion échoue, l'état n'avance pas.
	if err := r.store.Insert(e.cfg.Name, es); err != nil {
		return result{}, err
	}
	data, _ := json.Marshal(st)
	if err := r.store.SaveState(e.cfg.Name, string(data)); err != nil {
		return res, err
	}
	if r.retention > 0 {
		_ = r.store.Purge(now.Add(-r.retention))
	}
	return res, nil
}
