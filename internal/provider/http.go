package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/kevin/log-tui/internal/config"
)

// HTTP récupère un fichier de logs par URL. Utilise Range quand le serveur le
// supporte, sinon retombe sur un téléchargement complet dédupliqué.
type HTTP struct {
	cfg    config.Provider
	client http.Client
}

func (h *HTTP) Close() error { return nil }

var contentRangeRe = regexp.MustCompile(`bytes (\d+)-(\d+)/(\d+|\*)`)

func (h *HTTP) Fetch(ctx context.Context, st *State) ([]string, error) {
	if h.client.Timeout == 0 {
		h.client.Timeout = 60 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.cfg.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range h.cfg.Headers {
		req.Header.Set(k, v)
	}
	ranged := false
	switch {
	case st.NoRange:
	case !st.Init:
		req.Header.Set("Range", fmt.Sprintf("bytes=-%d", h.cfg.InitialBytes))
		ranged = true
	default:
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", st.Offset))
		ranged = true
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		// Soit rien de nouveau, soit le fichier a été tronqué (rotation).
		if m := regexp.MustCompile(`bytes \*/(\d+)`).FindStringSubmatch(resp.Header.Get("Content-Range")); m != nil {
			if total, _ := strconv.ParseInt(m[1], 10, 64); total < st.Offset {
				st.Init, st.Offset = false, 0
			}
		}
		return nil, nil
	case resp.StatusCode == http.StatusPartialContent:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxReadPerFetch))
		if err != nil {
			return nil, err
		}
		m := contentRangeRe.FindStringSubmatch(resp.Header.Get("Content-Range"))
		if m == nil {
			return nil, fmt.Errorf("Content-Range invalide: %q", resp.Header.Get("Content-Range"))
		}
		first, _ := strconv.ParseInt(m[1], 10, 64)
		skipPartial := !st.Init && first > 0
		if skipPartial {
			if i := indexByte(body, '\n'); i >= 0 {
				body, first = body[i+1:], first+int64(i+1)
			} else {
				body = nil
			}
		}
		end := lastIndexByte(body, '\n')
		var lines []string
		if end >= 0 {
			lines = splitLines(body[:end])
			st.Offset = first + int64(end) + 1
		} else if !st.Init {
			st.Offset = first
		}
		st.Init = true
		return lines, nil
	case resp.StatusCode == http.StatusOK:
		if ranged {
			st.NoRange = true
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, err
		}
		return dedupeTail(splitLines(body), st), nil
	}
	return nil, fmt.Errorf("HTTP %s", resp.Status)
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func lastIndexByte(b []byte, c byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == c {
			return i
		}
	}
	return -1
}
