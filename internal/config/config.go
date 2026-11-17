// Package config charge la configuration YAML de log-tui.
package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kevin/log-tui/internal/datepath"

	"gopkg.in/yaml.v3"
)

// Provider décrit une source de logs.
type Provider struct {
	Name     string        `yaml:"name"`
	Type     string        `yaml:"type"` // sftp | ssh | http | local
	Interval time.Duration `yaml:"interval"`

	// sftp / ssh
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	KeyFile         string `yaml:"key"`
	Passphrase      string `yaml:"passphrase"`
	KnownHosts      string `yaml:"known_hosts"`
	InsecureHostKey bool   `yaml:"insecure_host_key"`

	// sftp / local. Peut contenir des directives de date (%Y %m %d %H %j), ex. prod-%Y-%m-%d.log
	Path string `yaml:"path"`
	// Fuseau horaire utilisé pour résoudre les dates du chemin (défaut : local), ex. Europe/Paris.
	Timezone string `yaml:"timezone"`
	// ssh
	Command string `yaml:"command"`
	// http
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`

	// Follow démarre le suivi du provider au lancement. Défaut : non suivi
	// (le provider reste en pause jusqu'à ce qu'on appuie sur « p »).
	Follow bool `yaml:"follow"`

	// Octets lus à la première récupération (sftp/local/http). Défaut 1 MiB.
	InitialBytes int64 `yaml:"initial_bytes"`
}

// Config est la configuration racine.
type Config struct {
	DB        string        `yaml:"db"`
	Retention time.Duration `yaml:"retention"` // 0 = illimité
	// Notifications d'erreurs quand le terminal n'a pas le focus (défaut : activées).
	Notify         *bool         `yaml:"notify"`
	NotifyCooldown time.Duration `yaml:"notify_cooldown"` // délai min. entre deux notifications d'un provider (défaut 10s)
	Providers      []Provider    `yaml:"providers"`
}

//go:embed example.yaml
var exampleYAML []byte

// Example retourne la configuration d'exemple.
func Example() []byte { return exampleYAML }

// EnsureExists crée le fichier avec la configuration d'exemple s'il n'existe pas.
// Retourne true si le fichier vient d'être créé.
func EnsureExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	// O_EXCL : ne jamais écraser un fichier créé entre-temps.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Write(exampleYAML)
	return err == nil, err
}

// Follows indique si le provider name est suivi dès le lancement.
func (c *Config) Follows(name string) bool {
	for _, p := range c.Providers {
		if p.Name == name {
			return p.Follow
		}
	}
	return false
}

// NotifyEnabled indique si les notifications sont actives.
func (c *Config) NotifyEnabled() bool { return c.Notify == nil || *c.Notify }

// Location retourne le fuseau horaire du provider (local si non défini ou invalide).
func (p Provider) Location() *time.Location {
	if p.Timezone != "" {
		if loc, err := time.LoadLocation(p.Timezone); err == nil {
			return loc
		}
	}
	return time.Local
}

// DefaultPath retourne le chemin de configuration par défaut.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "log-tui", "config.yaml")
}

// Load lit et valide le fichier de configuration.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse décode et valide une configuration YAML.
func Parse(data []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save écrit le fichier de façon atomique (fichier temporaire + rename), en 0600.
func Save(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil { // CreateTemp crée en 0600
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ExpandPath remplace ~ et les variables d'environnement.
func ExpandPath(p string) string {
	if p == "" {
		return p
	}
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func (c *Config) normalize() error {
	if c.DB == "" {
		dir := os.Getenv("XDG_DATA_HOME")
		if dir == "" {
			dir = filepath.Join(os.Getenv("HOME"), ".local", "share")
		}
		c.DB = filepath.Join(dir, "log-tui", "logs.db")
	}
	c.DB = ExpandPath(c.DB)
	if c.NotifyCooldown <= 0 {
		c.NotifyCooldown = 10 * time.Second
	}
	if len(c.Providers) == 0 {
		return fmt.Errorf("aucun provider défini")
	}
	seen := map[string]bool{}
	for i := range c.Providers {
		p := &c.Providers[i]
		if p.Name == "" {
			return fmt.Errorf("providers[%d]: name requis", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("provider %q défini deux fois", p.Name)
		}
		seen[p.Name] = true
		if p.Interval <= 0 {
			p.Interval = 30 * time.Second
		}
		if p.InitialBytes <= 0 {
			p.InitialBytes = 1 << 20
		}
		p.Password = os.ExpandEnv(p.Password)
		p.Passphrase = os.ExpandEnv(p.Passphrase)
		p.KeyFile = ExpandPath(p.KeyFile)
		p.KnownHosts = ExpandPath(p.KnownHosts)
		for k, v := range p.Headers {
			p.Headers[k] = os.ExpandEnv(v)
		}
		if p.Timezone != "" {
			if _, err := time.LoadLocation(p.Timezone); err != nil {
				return fmt.Errorf("provider %q: timezone %q invalide: %w", p.Name, p.Timezone, err)
			}
		}
		if (p.Type == "sftp" || p.Type == "local") && p.Path != "" {
			if err := datepath.Check(p.Path); err != nil {
				return fmt.Errorf("provider %q: path: %w", p.Name, err)
			}
			if _, err := filepath.Match(p.Path, ""); err != nil {
				return fmt.Errorf("provider %q: path: motif invalide %q: %w", p.Name, p.Path, err)
			}
		}
		switch p.Type {
		case "sftp":
			if p.Host == "" || p.Path == "" {
				return fmt.Errorf("provider %q: host et path requis", p.Name)
			}
		case "ssh":
			if p.Host == "" || p.Command == "" {
				return fmt.Errorf("provider %q: host et command requis", p.Name)
			}
		case "http":
			if p.URL == "" {
				return fmt.Errorf("provider %q: url requis", p.Name)
			}
		case "local":
			if p.Path == "" {
				return fmt.Errorf("provider %q: path requis", p.Name)
			}
			p.Path = ExpandPath(p.Path)
		default:
			return fmt.Errorf("provider %q: type %q inconnu (sftp|ssh|http|local)", p.Name, p.Type)
		}
	}
	return nil
}
