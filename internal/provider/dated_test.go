package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kevin/log-tui/internal/config"
)

func TestFetchDatedDaySwitch(t *testing.T) {
	dir := t.TempDir()
	pattern := filepath.Join(dir, "prod-%Y-%m-%d.log")
	day1 := filepath.Join(dir, "prod-2026-11-04.log")
	day2 := filepath.Join(dir, "prod-2026-11-05.log")
	t1 := time.Date(2026, 11, 4, 23, 59, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 5, 0, 0, 30, 0, time.UTC)
	fetch := func(now time.Time, st *State) []string {
		t.Helper()
		got, err := fetchDated(pattern, time.UTC, now, 1<<20, st, localSource)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	st := &State{}

	appendFile(t, day1, "a1\na2\n")
	if got := fetch(t1, st); !reflect.DeepEqual(got, []string{"a1", "a2"}) || st.File != day1 {
		t.Fatal(got, st.File)
	}
	// Lignes écrites juste avant minuit, la dernière sans retour à la ligne ; le nouveau fichier n'existe pas encore.
	appendFile(t, day1, "a3\na4")
	got := fetch(t2, st)
	if !reflect.DeepEqual(got, []string{"a3"}) || st.File != day1 {
		t.Fatal("avant création du fichier du jour:", got, st.File)
	}
	// Le fichier du jour apparaît : fin de l'ancien (dont la ligne partielle) puis tout le nouveau.
	appendFile(t, day2, "b1\nb2\n")
	got = fetch(t2, st)
	if !reflect.DeepEqual(got, []string{"a4", "b1", "b2"}) || st.File != day2 {
		t.Fatal("bascule:", got, st.File)
	}
	// Ensuite, lecture incrémentale du nouveau fichier uniquement, sans doublons.
	if got = fetch(t2, st); len(got) != 0 {
		t.Fatal("doublons:", got)
	}
	appendFile(t, day2, "b3\n")
	if got = fetch(t2, st); !reflect.DeepEqual(got, []string{"b3"}) {
		t.Fatal(got)
	}
}

func TestFetchDatedMissingFile(t *testing.T) {
	pattern := filepath.Join(t.TempDir(), "x-%Y.log")
	if _, err := fetchDated(pattern, time.UTC, time.Now(), 1<<20, &State{}, localSource); err == nil {
		t.Fatal("erreur attendue : fichier absent au premier tour")
	}
}

func TestFetchDatedLegacyState(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.log")
	appendFile(t, p, "one\n")
	st := &State{Init: true, Offset: 4} // état persisté avant l'ajout de File
	appendFile(t, p, "two\n")
	got, err := fetchDated(p, time.UTC, time.Now(), 1<<20, st, localSource)
	if err != nil || !reflect.DeepEqual(got, []string{"two"}) || st.File != p {
		t.Fatal(got, err, st.File)
	}
}

func TestSFTPDatedPath(t *testing.T) {
	var out atomic.Value
	host, port := startSSHServer(t, &out)
	dir := t.TempDir()
	today := filepath.Join(dir, time.Now().Format("2006-01-02")+".log")
	appendFile(t, today, "d1\nd2\n")

	p, _ := New(config.Provider{
		Name: "x", Type: "sftp", Host: host, Port: port, User: "u", Password: "pw",
		InsecureHostKey: true, Path: filepath.Join(dir, "%Y-%m-%d.log"), InitialBytes: 1 << 20,
	})
	defer p.Close()
	st := &State{}
	got, err := p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"d1", "d2"}) || st.File != today {
		t.Fatal(got, err, st.File)
	}
	_ = os.Remove(today)
}

func touch(t *testing.T, p string, ago time.Duration) {
	t.Helper()
	ts := time.Now().Add(-ago)
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func TestGlobFollowsNewestFile(t *testing.T) {
	dir := t.TempDir()
	pattern := filepath.Join(dir, "file-*.log")
	a, b := filepath.Join(dir, "file-a.log"), filepath.Join(dir, "file-b.log")
	fetch := func(st *State) []string {
		t.Helper()
		got, err := fetchDated(pattern, time.UTC, time.Now(), 1<<20, st, localSource)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	st := &State{}
	if _, err := fetchDated(pattern, time.UTC, time.Now(), 1<<20, st, localSource); err == nil {
		t.Fatal("aucune correspondance : erreur attendue")
	}

	appendFile(t, a, "a1\n")
	touch(t, a, time.Minute)
	if got := fetch(st); !reflect.DeepEqual(got, []string{"a1"}) || st.File != a {
		t.Fatal(got, st.File)
	}
	// Un fichier plus récent apparaît : fin de a puis tout b.
	appendFile(t, a, "a2\n")
	touch(t, a, time.Minute)
	appendFile(t, b, "b1\n")
	touch(t, b, 10*time.Second)
	if got := fetch(st); !reflect.DeepEqual(got, []string{"a2", "b1"}) || st.File != b {
		t.Fatal("bascule:", got, st.File)
	}
	if got := fetch(st); len(got) != 0 {
		t.Fatal("doublons:", got)
	}
	// a redevient le plus récent (écrit à nouveau) : reprise à sa position, sans relire a1/a2.
	appendFile(t, a, "a3\n")
	touch(t, a, time.Second)
	got := fetch(st)
	if !reflect.DeepEqual(got, []string{"a3"}) || st.File != a {
		t.Fatal("retour sur a:", got, st.File)
	}
	// Retour sur b : reprise sans relire b1.
	appendFile(t, b, "b2\n")
	touch(t, b, 0)
	if got = fetch(st); !reflect.DeepEqual(got, []string{"b2"}) || st.File != b {
		t.Fatal("retour sur b:", got, st.File)
	}
}

func TestGlobWithDate(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app-2026-11-04-0001.log")
	appendFile(t, f, "x\n")
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	got, err := fetchDated(filepath.Join(dir, "app-%Y-%m-%d-*.log"), time.UTC, now, 1<<20, &State{}, localSource)
	if err != nil || !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatal(got, err)
	}
}

func TestSFTPGlobPath(t *testing.T) {
	var out atomic.Value
	host, port := startSSHServer(t, &out)
	dir := t.TempDir()
	old, cur := filepath.Join(dir, "file-1.log"), filepath.Join(dir, "file-2.log")
	appendFile(t, old, "o1\n")
	touch(t, old, time.Hour)
	appendFile(t, cur, "c1\nc2\n")

	p, _ := New(config.Provider{
		Name: "x", Type: "sftp", Host: host, Port: port, User: "u", Password: "pw",
		InsecureHostKey: true, Path: filepath.Join(dir, "file-*.log"), InitialBytes: 1 << 20,
	})
	defer p.Close()
	st := &State{}
	got, err := p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"c1", "c2"}) || st.File != cur {
		t.Fatal(got, err, st.File)
	}
}
