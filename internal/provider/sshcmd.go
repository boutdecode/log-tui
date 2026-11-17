package provider

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/sshutil"
	"golang.org/x/crypto/ssh"
)

// SSHCmd exécute une commande distante (tail -n 500 …, journalctl …) et
// n'émet que les lignes nouvelles par rapport à l'exécution précédente.
type SSHCmd struct {
	cfg config.Provider
	mu  sync.Mutex
	cli *ssh.Client
}

func (s *SSHCmd) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cli != nil {
		s.cli.Close()
		s.cli = nil
	}
	return nil
}

func (s *SSHCmd) Fetch(ctx context.Context, st *State) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cli == nil {
		c, err := sshutil.Dial(ctx, s.cfg)
		if err != nil {
			return nil, err
		}
		s.cli = c
	}
	out, err := s.run(ctx)
	if err != nil {
		s.cli.Close()
		s.cli = nil
		return nil, err
	}
	return dedupeTail(splitLines(out), st), nil
}

func (s *SSHCmd) run(ctx context.Context) ([]byte, error) {
	sess, err := s.cli.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	var stdout, stderr bytes.Buffer
	sess.Stdout, sess.Stderr = &stdout, &stderr
	stop := context.AfterFunc(ctx, func() { sess.Close() })
	defer stop()
	if err := sess.Run(s.cfg.Command); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
