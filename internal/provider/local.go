package provider

import (
	"context"
	"errors"
	"github.com/kevin/log-tui/internal/i18n"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kevin/log-tui/internal/config"
)

// Local lit un fichier du disque (le chemin peut contenir des directives de date).
type Local struct {
	cfg config.Provider
	loc *time.Location
}

func (l *Local) Close() error { return nil }

func (l *Local) Fetch(_ context.Context, st *State) ([]string, error) {
	loc := l.loc
	if loc == nil {
		loc = time.Local
	}
	return fetchDated(l.cfg.Path, loc, time.Now(), l.cfg.InitialBytes, st, localSource)
}

var localSource = source{open: openLocal, newest: newestLocal}

// newestLocal retourne le fichier correspondant au motif modifié le plus récemment.
func newestLocal(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	best, bestT := "", time.Time{}
	for _, m := range matches {
		fi, err := os.Stat(m)
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
}

func openLocal(path string) (io.ReaderAt, int64, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, nil, err
	}
	return f, fi.Size(), f.Close, nil
}
