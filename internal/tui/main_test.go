package tui

import (
	"os"
	"testing"

	"github.com/kevin/log-tui/internal/i18n"
)

// Les assertions de ce paquet portent sur les textes français.
func TestMain(m *testing.M) {
	_ = i18n.Set(i18n.FR)
	os.Exit(m.Run())
}
