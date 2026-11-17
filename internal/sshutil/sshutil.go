// Package sshutil établit des connexions SSH à partir d'une config provider.
package sshutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kevin/log-tui/internal/config"
	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Dial ouvre une connexion SSH. Les valeurs absentes de la config sont
// complétées depuis ~/.ssh/config (HostName, User, Port, IdentityFile).
func Dial(ctx context.Context, p config.Provider) (*ssh.Client, error) {
	host, user, port, keyFile := p.Host, p.User, p.Port, p.KeyFile
	alias := p.Host
	if h := ssh_config.Get(alias, "HostName"); h != "" {
		host = h
	}
	if user == "" {
		user = ssh_config.Get(alias, "User")
	}
	if user == "" {
		user = os.Getenv("USER")
	}
	if port == 0 {
		if n, err := strconv.Atoi(ssh_config.Get(alias, "Port")); err == nil {
			port = n
		} else {
			port = 22
		}
	}
	if keyFile == "" {
		// ssh_config renvoie "~/.ssh/identity" par défaut : on ne le retient que s'il existe.
		if f := config.ExpandPath(ssh_config.Get(alias, "IdentityFile")); f != "" {
			if _, err := os.Stat(f); err == nil {
				keyFile = f
			}
		}
	}

	auth, err := authMethods(p, keyFile)
	if err != nil {
		return nil, err
	}
	hk, err := hostKeyCallback(p)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{User: user, Auth: auth, HostKeyCallback: hk, Timeout: 15 * time.Second}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var d net.Dialer
	d.Timeout = cfg.Timeout
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// Garde-fou: le handshake respecte l'annulation du contexte.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh %s: %w", addr, err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func authMethods(p config.Provider, keyFile string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	var files []string
	if keyFile != "" {
		// Une clé publique (.pub) est souvent indiquée par erreur : on utilise la clé privée voisine.
		if priv := strings.TrimSuffix(keyFile, ".pub"); priv != keyFile {
			if _, err := os.Stat(priv); err == nil {
				keyFile = priv
			} else {
				return nil, fmt.Errorf("clé %s: c'est une clé publique et la clé privée %s est introuvable", keyFile, priv)
			}
		}
		files = []string{keyFile}
	} else if home, err := os.UserHomeDir(); err == nil {
		for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			files = append(files, filepath.Join(home, ".ssh", n))
		}
	}
	var signers []ssh.Signer
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			if keyFile != "" {
				return nil, fmt.Errorf("clé %s: %w", f, err)
			}
			continue
		}
		s, err := ssh.ParsePrivateKey(data)
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			if p.Passphrase == "" {
				err = fmt.Errorf("clé protégée par une passphrase : renseignez « passphrase » ou chargez-la dans ssh-agent")
			} else {
				s, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(p.Passphrase))
			}
		}
		if err != nil {
			if keyFile != "" {
				return nil, fmt.Errorf("clé %s: %w", f, err)
			}
			continue
		}
		signers = append(signers, s)
	}
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	if p.Password != "" {
		methods = append(methods, ssh.Password(p.Password))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("aucune méthode d'authentification disponible (key, agent ou password)")
	}
	return methods, nil
}

func hostKeyCallback(p config.Provider) (ssh.HostKeyCallback, error) {
	if p.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	file := p.KnownHosts
	if file == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		file = filepath.Join(home, ".ssh", "known_hosts")
	}
	cb, err := knownhosts.New(file)
	if err != nil {
		return nil, fmt.Errorf("known_hosts %s: %w (ou insecure_host_key: true)", file, err)
	}
	return cb, nil
}
