// Commande log-tui : dashboard TUI de monitoring de logs.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/scheduler"
	"github.com/kevin/log-tui/internal/store"
	"github.com/kevin/log-tui/internal/tui"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "chemin du fichier de configuration YAML")
	transparent := flag.Bool("transparent", false, "garde le fond du terminal (pas de fond coloré)")
	flag.Parse()

	if err := run(*cfgPath, tui.Options{Transparent: *transparent}); err != nil {
		fmt.Fprintln(os.Stderr, "log-tui:", err)
		os.Exit(1)
	}
}

func run(cfgPath string, opts tui.Options) error {
	// Pour le chemin par défaut uniquement : créer un exemple si absent.
	if cfgPath == config.DefaultPath() {
		created, err := config.EnsureExists(cfgPath)
		if err != nil {
			return err
		}
		if created {
			fmt.Fprintf(os.Stderr, "log-tui: configuration d'exemple créée dans %s\n", cfgPath)
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DB)
	if err != nil {
		return err
	}
	defer st.Close()
	runner, err := scheduler.New(cfg, st)
	if err != nil {
		return err
	}
	defer runner.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	_, err = tea.NewProgram(tui.New(ctx, runner, st, cfgPath, cfg, opts), tea.WithContext(ctx)).Run()
	cancel() // interrompt les récupérations en cours avant la fermeture
	return err
}
