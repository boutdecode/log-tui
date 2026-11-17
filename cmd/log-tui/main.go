// Commande log-tui : dashboard TUI de monitoring de logs.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/i18n"
	"github.com/kevin/log-tui/internal/scheduler"
	"github.com/kevin/log-tui/internal/store"
	"github.com/kevin/log-tui/internal/tui"
)

// langArg cherche -lang / --lang (valeur séparée ou après « = ») dans args, sans
// parser les autres options : permet de localiser l'aide (-h) avant flag.Parse.
func langArg(args []string) string {
	for i, a := range args {
		a = strings.TrimLeft(a, "-")
		if a == "lang" && strings.HasPrefix(args[i], "-") && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "lang="); ok && strings.HasPrefix(args[i], "-") {
			return v
		}
	}
	return ""
}

func main() {
	// Langue : réglée une première fois pour l'aide, puis validée après le parsing.
	_ = i18n.Set(langArg(os.Args[1:]))

	cfgPath := flag.String("config", config.DefaultPath(), i18n.T("cli.flag.config"))
	transparent := flag.Bool("transparent", false, i18n.T("cli.flag.transparent"))
	lang := flag.String("lang", "", i18n.T("cli.flag.lang"))
	flag.Parse()

	if err := i18n.Set(*lang); err != nil {
		fmt.Fprintln(os.Stderr, "log-tui:", i18n.T("cli.badlang", *lang))
		os.Exit(2)
	}

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
			fmt.Fprintf(os.Stderr, "log-tui: %s\n", i18n.T("cli.configcreated", cfgPath))
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
