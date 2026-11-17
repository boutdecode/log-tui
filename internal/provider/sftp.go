package provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/kevin/log-tui/internal/i18n"
	"io"
	"sync"
	"time"

	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/sshutil"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTP lit incrémentalement un fichier distant (équivalent d'un scp répété,
// mais en ne transférant que les octets nouveaux).
type SFTP struct {
	cfg config.Provider
	loc *time.Location
	mu  sync.Mutex
	ssh *ssh.Client
	cli *sftp.Client
}

func (s *SFTP) connect(ctx context.Context) error {
	if s.cli != nil {
		return nil
	}
	c, err := sshutil.Dial(ctx, s.cfg)
	if err != nil {
		return err
	}
	cli, err := sftp.NewClient(c)
	if err != nil {
		c.Close()
		return err
	}
	s.ssh, s.cli = c, cli
	return nil
}

func (s *SFTP) reset() {
	if s.cli != nil {
		s.cli.Close()
	}
	if s.ssh != nil {
		s.ssh.Close()
	}
	s.cli, s.ssh = nil, nil
}

func (s *SFTP) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset()
	return nil
}

func (s *SFTP) Fetch(ctx context.Context, st *State) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.connect(ctx); err != nil {
		return nil, err
	}
	lines, err := s.fetch(st)
	if err != nil {
		// La connexion est peut-être morte : on la recrée au prochain tour.
		s.reset()
		return nil, err
	}
	return lines, nil
}

func (s *SFTP) fetch(st *State) ([]string, error) {
	loc := s.loc
	if loc == nil {
		loc = time.Local
	}
	src := source{
		open: func(path string) (io.ReaderAt, int64, func() error, error) {
			f, err := s.cli.Open(path)
			if err != nil {
				return nil, 0, nil, fmt.Errorf("open %s: %w", path, err)
			}
			fi, err := f.Stat()
			if err != nil {
				f.Close()
				return nil, 0, nil, err
			}
			return f, fi.Size(), f.Close, nil
		},
		newest: func(pattern string) (string, error) {
			matches, err := s.cli.Glob(pattern)
			if err != nil {
				return "", err
			}
			best, bestT := "", time.Time{}
			for _, m := range matches {
				fi, err := s.cli.Stat(m)
				if err != nil || fi.IsDir() {
					continue
				}
				if best == "" || fi.ModTime().After(bestT) || (fi.ModTime().Equal(bestT) && m > best) {
					best, bestT = m, fi.ModTime()
				}
			}
			if best == "" {
				return "", errors.New(i18n.T("err.nomatch", pattern))
			}
			return best, nil
		},
	}
	return fetchDated(s.cfg.Path, loc, time.Now(), s.cfg.InitialBytes, st, src)
}
