package sshutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevin/log-tui/internal/config"
	"golang.org/x/crypto/ssh"
)

func TestPubKeyFallsBackToPrivate(t *testing.T) {
	dir := t.TempDir()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	blk, _ := ssh.MarshalPrivateKey(priv, "")
	os.WriteFile(filepath.Join(dir, "k"), pem.EncodeToMemory(blk), 0o600)
	os.WriteFile(filepath.Join(dir, "k.pub"), []byte("ssh-ed25519 AAAA x"), 0o600)

	pub := filepath.Join(dir, "k.pub")
	if _, err := authMethods(config.Provider{}, pub); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, "k"))
	if _, err := authMethods(config.Provider{}, pub); err == nil {
		t.Fatal("erreur attendue sans clé privée")
	}
}
