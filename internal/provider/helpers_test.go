package provider

import (
	"io"
	"strings"
	"time"
)

var time0 time.Time

// strReader renvoie un ReadSeeker sur le contenu courant (relu à chaque appel).
func strReader(s string) io.ReadSeeker { return strings.NewReader(s) }
