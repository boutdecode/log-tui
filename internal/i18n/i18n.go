// Package i18n fournit la traduction (fr, en) des textes de l'interface.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Langues supportées.
const (
	FR = "fr"
	EN = "en"
)

var current atomic.Value // string

func init() { current.Store(EN) }

var catalogs = map[string]map[string]string{FR: fr, EN: en}

// Normalize convertit une valeur de locale ("fr", "fr_FR.UTF-8", "en-US"…)
// en langue supportée. Une chaîne vide ou "C"/"POSIX" retourne ("", nil).
func Normalize(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "c" || s == "posix" {
		return "", nil
	}
	for _, lang := range []string{FR, EN} {
		if s == lang || strings.HasPrefix(s, lang+"_") || strings.HasPrefix(s, lang+"-") || strings.HasPrefix(s, lang+".") {
			return lang, nil
		}
	}
	return "", fmt.Errorf("unsupported language %q (fr, en)", s)
}

// Detect lit la langue du système (LC_ALL, LC_MESSAGES, LANG) ; en par défaut.
func Detect() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		val := os.Getenv(v)
		if val == "" {
			continue
		}
		if lang, err := Normalize(val); err == nil && lang != "" {
			return lang
		}
		// Première variable non vide : elle fait foi (convention POSIX).
		return EN
	}
	return EN
}

// Set choisit la langue courante ("" = détection système).
func Set(lang string) error {
	if lang == "" {
		current.Store(Detect())
		return nil
	}
	l, err := Normalize(lang)
	if err != nil {
		return err
	}
	if l == "" {
		l = EN
	}
	current.Store(l)
	return nil
}

// Lang retourne la langue courante.
func Lang() string { return current.Load().(string) }

// Has indique si la clé existe dans la langue courante ou en anglais.
func Has(key string) bool {
	_, ok := lookup(key)
	return ok
}

func lookup(key string) (string, bool) {
	if s, ok := catalogs[Lang()][key]; ok {
		return s, true
	}
	s, ok := catalogs[EN][key]
	return s, ok
}

// T traduit la clé et applique fmt.Sprintf si des arguments sont fournis.
// Clé inconnue : la clé elle-même est retournée.
func T(key string, args ...any) string {
	s, ok := lookup(key)
	if !ok {
		return key
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// Or traduit la clé, ou retourne fallback si elle n'existe pas.
func Or(key, fallback string) string {
	if s, ok := lookup(key); ok {
		return s
	}
	return fallback
}

// Keys retourne les clés d'une langue (tests de parité).
func Keys(lang string) []string {
	var out []string
	for k := range catalogs[lang] {
		out = append(out, k)
	}
	return out
}
