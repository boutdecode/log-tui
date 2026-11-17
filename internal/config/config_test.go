package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureExists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if ok, err := EnsureExists(p); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("l'exemple doit être valide: %v", err)
	}
	os.WriteFile(p, []byte("custom"), 0o600)
	if ok, _ := EnsureExists(p); ok {
		t.Fatal("ne doit pas recréer")
	}
	if b, _ := os.ReadFile(p); string(b) != "custom" {
		t.Fatal("fichier écrasé")
	}
}

func TestDatedPathValidation(t *testing.T) {
	ok := "providers:\n  - {name: a, type: local, path: /tmp/p-%Y-%m-%d.log, timezone: Europe/Paris}\n"
	c, err := Parse([]byte(ok))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]byte("providers:\n  - {name: a, type: local, path: '/tmp/file-*.log'}\n")); err != nil {
		t.Fatal(err)
	}
	if c.Providers[0].Location().String() != "Europe/Paris" {
		t.Fatal(c.Providers[0].Location())
	}
	for _, bad := range []string{
		"providers:\n  - {name: a, type: local, path: /tmp/p-%Q.log}\n",
		"providers:\n  - {name: a, type: local, path: /tmp/p.log, timezone: Nope/Zone}\n",
		"providers:\n  - {name: a, type: local, path: '/tmp/p-[.log'}\n",
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("devrait échouer : %s", bad)
		}
	}
}

func TestFollowDefaultsToFalse(t *testing.T) {
	c, err := Parse([]byte("providers:\n  - {name: a, type: local, path: /tmp/a}\n  - {name: b, type: local, path: /tmp/b, follow: true}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Follows("a") || !c.Follows("b") || c.Follows("inconnu") {
		t.Fatal("follow doit valoir false par défaut")
	}
}
