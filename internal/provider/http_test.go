package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/kevin/log-tui/internal/config"
)

func TestHTTPRange(t *testing.T) {
	content := "l1\nl2\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.log", time0, strReader(content))
	}))
	defer srv.Close()
	p, _ := New(config.Provider{Name: "h", Type: "http", URL: srv.URL, InitialBytes: 1 << 20})
	st := &State{}
	got, err := p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"l1", "l2"}) {
		t.Fatal(got, err)
	}
	content += "l3\nl4\n"
	got, err = p.Fetch(context.Background(), st)
	if err != nil || !reflect.DeepEqual(got, []string{"l3", "l4"}) {
		t.Fatal(got, err)
	}
	if got, err = p.Fetch(context.Background(), st); err != nil || len(got) != 0 {
		t.Fatal("rien de neuf attendu", got, err)
	}
}

func TestHTTPNoRange(t *testing.T) {
	content := "a\nb\nc\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content)) // ignore Range
	}))
	defer srv.Close()
	p, _ := New(config.Provider{Name: "h", Type: "http", URL: srv.URL, InitialBytes: 1 << 20})
	st := &State{}
	if got, _ := p.Fetch(context.Background(), st); len(got) != 3 {
		t.Fatal(got)
	}
	content += "d\n"
	if got, _ := p.Fetch(context.Background(), st); !reflect.DeepEqual(got, []string{"d"}) {
		t.Fatal(got)
	}
}
