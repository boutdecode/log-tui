// Package provider définit les sources de logs et leur récupération incrémentale.
package provider

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/datepath"
)

// State est l'état persistant d'un provider entre deux récupérations.
type State struct {
	File string `json:"file,omitempty"` // chemin résolu actuellement suivi (chemins datés / jokers)
	// Known mémorise la position de lecture des fichiers précédemment suivis,
	// pour reprendre sans doublon si l'on revient sur l'un d'eux.
	Known   map[string]int64 `json:"known,omitempty"`
	Init    bool             `json:"init"`
	Offset  int64            `json:"offset"`
	Head    string           `json:"head,omitempty"`     // empreinte des premiers octets (détection de rotation)
	Tail    []string         `json:"tail,omitempty"`     // dernières lignes (providers sans offset)
	NoRange bool             `json:"no_range,omitempty"` // http: le serveur ignore Range
}

// Provider récupère de nouvelles lignes depuis une source.
type Provider interface {
	// Fetch retourne les lignes apparues depuis le dernier appel et met à jour st.
	Fetch(ctx context.Context, st *State) ([]string, error)
	Close() error
}

// New construit le provider correspondant à la config.
func New(c config.Provider) (Provider, error) {
	switch c.Type {
	case "local":
		return &Local{cfg: c, loc: c.Location()}, nil
	case "sftp":
		return &SFTP{cfg: c, loc: c.Location()}, nil
	case "ssh":
		return &SSHCmd{cfg: c}, nil
	case "http":
		return &HTTP{cfg: c}, nil
	}
	return nil, fmt.Errorf("type %q inconnu", c.Type)
}

const (
	maxReadPerFetch = 16 << 20
	headLen         = 64
	tailKeep        = 5
)

// readIncremental lit les lignes complètes ajoutées à un fichier depuis st.Offset.
// Gère la première lecture (queue du fichier), la rotation (troncature ou
// changement du début de fichier) et les lignes partielles en fin de fichier.
func readIncremental(r io.ReaderAt, size int64, initialBytes int64, st *State) ([]string, error) {
	// Rotation ?
	if st.Init && (size < st.Offset || headChanged(r, size, st)) {
		st.Init, st.Offset, st.Head = false, 0, ""
		return readIncremental(r, size, size, st) // nouveau fichier : tout lire
	}
	start := st.Offset
	skipPartial := false
	if !st.Init {
		if size > initialBytes {
			start = size - initialBytes
			skipPartial = true
		} else {
			start = 0
		}
	}
	n := size - start
	if n > maxReadPerFetch {
		n = maxReadPerFetch
	}
	buf := make([]byte, n)
	if n > 0 {
		if _, err := r.ReadAt(buf, start); err != nil && err != io.EOF {
			return nil, err
		}
	}
	consumedBase := start
	if skipPartial {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
			consumedBase += int64(i + 1)
		} else {
			buf = nil // une seule ligne partielle
		}
	}
	end := bytes.LastIndexByte(buf, '\n')
	var lines []string
	if end >= 0 {
		lines = splitLines(buf[:end])
		st.Offset = consumedBase + int64(end) + 1
	} else if !st.Init || skipPartial {
		st.Offset = consumedBase
	}
	st.Init = true
	setHead(r, size, st)
	return lines, nil
}

func splitLines(b []byte) []string {
	var out []string
	for _, l := range bytes.Split(b, []byte{'\n'}) {
		l = bytes.TrimRight(l, "\r")
		if len(l) > 0 {
			out = append(out, string(l))
		}
	}
	return out
}

func headChanged(r io.ReaderAt, size int64, st *State) bool {
	if st.Head == "" {
		return false
	}
	want, err := hex.DecodeString(st.Head)
	if err != nil || int64(len(want)) > size {
		return err == nil // fichier plus petit que l'empreinte : rotation
	}
	got := make([]byte, len(want))
	if _, err := r.ReadAt(got, 0); err != nil && err != io.EOF {
		return false
	}
	return !bytes.Equal(got, want)
}

func setHead(r io.ReaderAt, size int64, st *State) {
	if st.Head != "" || size < headLen {
		return
	}
	b := make([]byte, headLen)
	if _, err := r.ReadAt(b, 0); err == nil || err == io.EOF {
		st.Head = hex.EncodeToString(b)
	}
}

// dedupeTail retire des lines le préfixe déjà vu, d'après les dernières lignes
// mémorisées. Utilisé par les sources qui renvoient toujours une fenêtre
// glissante (tail -n, journalctl, HTTP sans Range).
func dedupeTail(lines []string, st *State) []string {
	out := lines
	// Cherche le plus long suffixe des lignes mémorisées (au moins 2, ou
	// toutes si moins) retrouvé dans la nouvelle fenêtre ; on garde ce qui suit.
	n := len(st.Tail)
	minK := n
	if minK > 2 {
		minK = 2
	}
search:
	for k := n; k >= minK && k > 0; k-- {
		for j := len(lines); j >= k; j-- {
			if equal(lines[j-k:j], st.Tail[n-k:]) {
				out = lines[j:]
				break search
			}
		}
	}
	if len(lines) > 0 {
		from := len(lines) - tailKeep
		if from < 0 {
			from = 0
		}
		st.Tail = append([]string(nil), lines[from:]...)
	}
	st.Init = true
	return out
}

func equal(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// opener ouvre un fichier pour lecture aléatoire.
type opener func(path string) (r io.ReaderAt, size int64, closeFn func() error, err error)

// source regroupe les accès à un système de fichiers (local ou distant).
type source struct {
	open opener
	// newest retourne, parmi les fichiers correspondant au motif à jokers,
	// celui modifié le plus récemment.
	newest func(pattern string) (string, error)
}

const maxKnown = 32

// fetchDated lit un fichier dont le nom peut contenir des directives de date
// (%Y-%m-%d) et/ou des jokers (*, ?, [..]) : dans ce dernier cas le fichier le
// plus récemment modifié est suivi.
//
// Quand le fichier suivi change, la fin de l'ancien est lue d'abord, puis le
// nouveau est lu (depuis le début, ou depuis la dernière position connue si on
// y revient). Si le nouveau fichier n'existe pas encore, on continue sur
// l'ancien et on réessaie au tour suivant.
func fetchDated(pattern string, loc *time.Location, now time.Time, initialBytes int64, st *State, src source) ([]string, error) {
	expanded := datepath.Expand(pattern, now.In(loc))
	glob := datepath.HasGlob(expanded)
	resolved := expanded
	if glob {
		p, err := src.newest(expanded)
		switch {
		case err == nil:
			resolved = p
		case st.File != "":
			resolved = st.File // plus de correspondance : on reste sur le fichier courant
		default:
			return nil, err
		}
	}

	if st.File == "" || st.File == resolved {
		r, size, closeFn, err := src.open(resolved)
		if err != nil {
			return nil, err
		}
		defer closeFn()
		lines, err := readIncremental(r, size, initialBytes, st)
		if err != nil {
			return nil, err
		}
		st.File = resolved
		return lines, nil
	}

	// Changement de fichier.
	nr, nsize, nclose, nerr := src.open(resolved)
	if nerr == nil {
		defer nclose()
	}
	var old []string
	oldOK := false
	if r, size, closeFn, err := src.open(st.File); err == nil {
		oldOK = true
		old, _ = readIncremental(r, size, initialBytes, st)
		// Dernière ligne sans retour à la ligne : on ne la considère terminée
		// que pour un changement de date (l'ancien fichier est clos), pas avec
		// des jokers où il pourrait encore être écrit.
		if nerr == nil && !glob && size > st.Offset {
			if tail := readTail(r, st.Offset, size); tail != "" {
				old = append(old, tail)
			}
		}
		closeFn()
	}
	if nerr != nil {
		if !oldOK {
			return nil, nerr
		}
		return old, nil // le nouveau fichier n'est pas encore créé
	}

	known := map[string]int64{}
	for k, v := range st.Known {
		known[k] = v
	}
	if oldOK {
		known[st.File] = st.Offset
	}
	next := State{File: resolved, Known: known}
	if off, ok := known[resolved]; ok {
		next.Init, next.Offset = true, off
		delete(known, resolved)
	}
	for k := range known { // borne la taille de la mémoire
		if len(known) <= maxKnown {
			break
		}
		delete(known, k)
	}
	*st = next
	lines, err := readIncremental(nr, nsize, nsize, st)
	if err != nil {
		return nil, err
	}
	return append(old, lines...), nil
}

func readTail(r io.ReaderAt, from, to int64) string {
	if to-from > 1<<20 {
		from = to - 1<<20
	}
	buf := make([]byte, to-from)
	if _, err := r.ReadAt(buf, from); err != nil && err != io.EOF {
		return ""
	}
	return strings.TrimSpace(string(bytes.TrimRight(buf, "\r\n")))
}
