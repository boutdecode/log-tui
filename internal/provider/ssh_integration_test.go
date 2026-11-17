package provider

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/kevin/log-tui/internal/config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// startSSHServer lance un serveur SSH local (mot de passe "pw") qui supporte
// le sous-système sftp et exec (renvoie la valeur de execOut).
func startSSHServer(t *testing.T, execOut *atomic.Value) (host string, port int) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if string(p) == "pw" {
				return nil, nil
			}
			return nil, os.ErrPermission
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					ch, creqs, err := nch.Accept()
					if err != nil {
						continue
					}
					go func() {
						for r := range creqs {
							switch r.Type {
							case "subsystem":
								r.Reply(true, nil)
								go func() {
									srv, _ := sftp.NewServer(ch)
									srv.Serve()
									ch.Close()
								}()
							case "exec":
								r.Reply(true, nil)
								_ = binary.BigEndian.Uint32(r.Payload) // longueur de la commande
								out, _ := execOut.Load().(string)
								ch.Write([]byte(out))
								ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								ch.Close()
							default:
								r.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	a := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", a.Port
}

func TestSFTPProvider(t *testing.T) {
	var out atomic.Value
	host, port := startSSHServer(t, &out)
	file := filepath.Join(t.TempDir(), "remote.log")
	appendFile(t, file, "r1\nr2\n")

	p, _ := New(config.Provider{
		Name: "x", Type: "sftp", Host: host, Port: port, User: "u", Password: "pw",
		InsecureHostKey: true, Path: file, InitialBytes: 1 << 20,
	})
	defer p.Close()
	st := &State{}
	got, err := p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"r1", "r2"}) {
		t.Fatal(got, err)
	}
	appendFile(t, file, "r3\n")
	got, err = p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"r3"}) {
		t.Fatal("incrémental", got, err)
	}
	// Rotation
	os.WriteFile(file, []byte("fresh\n"), 0o644)
	got, err = p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"fresh"}) {
		t.Fatal("rotation", got, err)
	}
}

func TestSFTPBadPassword(t *testing.T) {
	var out atomic.Value
	host, port := startSSHServer(t, &out)
	p, _ := New(config.Provider{Name: "x", Type: "sftp", Host: host, Port: port, User: "u",
		Password: "nope", InsecureHostKey: true, Path: "/x"})
	if _, err := p.Fetch(context.Background(), &State{}); err == nil {
		t.Fatal("erreur d'authentification attendue")
	}
}

func TestSSHCmdProvider(t *testing.T) {
	var out atomic.Value
	out.Store("a\nb\nc\n")
	host, port := startSSHServer(t, &out)
	p, _ := New(config.Provider{Name: "x", Type: "ssh", Host: host, Port: port, User: "u",
		Password: "pw", InsecureHostKey: true, Command: "tail -n 3 /x"})
	defer p.Close()
	st := &State{}
	got, err := p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatal(got, err)
	}
	out.Store("b\nc\nd\ne\n")
	got, err = p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"d", "e"}) {
		t.Fatal(got, err)
	}
}
