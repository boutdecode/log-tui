package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/i18n"
	"github.com/kevin/log-tui/internal/scheduler"
)

// openEditor charge le fichier de configuration dans l'éditeur intégré.
func (m *Model) openEditor() tea.Cmd {
	data, err := os.ReadFile(m.cfgPath)
	if err != nil {
		data = config.Example(i18n.Lang()) // fichier supprimé entre-temps : repartir de l'exemple
		m.edMsg, m.edErr = i18n.T("cfg.unreadable", err), true
		m.edSaved = ""
	} else {
		m.edMsg, m.edErr = "", false
		m.edSaved = string(data)
	}
	m.editor.SetValue(string(data))
	m.editor.MoveToBegin()
	m.edQuitArm = false
	m.mode = modeConfig
	return m.editor.Focus()
}

func (m Model) onConfigKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		m.edQuitArm = false
		return m.saveConfig()
	case "esc":
		if m.editor.Value() != m.edSaved && !m.edQuitArm {
			m.edQuitArm = true
			m.edMsg, m.edErr = i18n.T("cfg.unsaved"), true
			return m, nil
		}
		m.editor.Blur()
		m.mode = modeLog
		return m, nil
	}
	m.edQuitArm = false
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

// saveConfig valide, écrit puis applique la configuration. Rien n'est écrit si elle est invalide.
func (m Model) saveConfig() (tea.Model, tea.Cmd) {
	text := m.editor.Value()
	cfg, err := config.Parse([]byte(text))
	if err != nil {
		m.edMsg, m.edErr = i18n.T("cfg.invalid", err), true
		return m, nil
	}
	if err := config.Save(m.cfgPath, []byte(text)); err != nil {
		m.edMsg, m.edErr = i18n.T("cfg.write", err), true
		return m, nil
	}
	m.edSaved = text

	// Reconstruire les providers avant de toucher à l'état courant.
	newRun, err := scheduler.New(cfg, m.st)
	if err != nil {
		m.edMsg, m.edErr = i18n.T("cfg.notapplied", err), true
		return m, nil
	}
	m.runCancel() // interrompt les récupérations de l'ancienne configuration
	old := m.run
	go old.Close() // peut attendre une récupération en cours : hors boucle UI

	m.runCtx, m.runCancel = contextWithCancel(m.ctx)
	m.run, m.epoch = newRun, m.epoch+1
	m.names = newRun.Names()
	oldStatus := m.status
	m.status = map[string]*pstatus{}
	m.counts = map[string]int{}
	m.sel, m.side = 0, false
	m.nt.reset() // nouveaux providers : leur historique ne doit pas notifier

	cmds := []tea.Cmd{m.load()}
	for _, n := range m.names {
		gen := m.epoch << 20
		// Un provider déjà connu garde son état suivi/en pause ; un nouveau suit la config.
		follow := cfg.Follows(n)
		if old, ok := oldStatus[n]; ok {
			follow = !old.paused
		}
		m.status[n] = &pstatus{fetching: follow, paused: !follow, gen: gen}
		if follow {
			cmds = append(cmds, m.run.Fetch(m.runCtx, n, gen))
		}
	}

	note := i18n.T("cfg.applied", len(m.names))
	if cfg.DB != m.cfg.DB {
		note += i18n.T("cfg.dbrestart")
		cfg.DB = m.cfg.DB
	}
	m.cfg = cfg
	m.edMsg, m.edErr = note, false
	return m, tea.Batch(cmds...)
}
