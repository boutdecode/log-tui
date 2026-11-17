package i18n

import "testing"

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"fr": "fr", "fr_FR.UTF-8": "fr", "EN-us": "en", "en_GB.UTF-8": "en", "C": "", "": ""} {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := Normalize("xx"); err == nil {
		t.Error("xx should be rejected")
	}
}

func TestDetectOrder(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "fr_FR.UTF-8")
	if Detect() != FR {
		t.Error("LANG=fr")
	}
	t.Setenv("LC_MESSAGES", "en_US.UTF-8")
	if Detect() != EN {
		t.Error("LC_MESSAGES must win over LANG")
	}
	t.Setenv("LC_ALL", "fr_FR")
	if Detect() != FR {
		t.Error("LC_ALL must win")
	}
	t.Setenv("LC_ALL", "de_DE")
	if Detect() != EN {
		t.Error("unsupported falls back to en")
	}
}

func TestTFallback(t *testing.T) {
	defer Set(EN)
	Set(FR)
	if T("no.such.key") != "no.such.key" {
		t.Error("unknown key returns itself")
	}
	if Or("no.such.key", "x") != "x" {
		t.Error("Or fallback")
	}
}

func TestParity(t *testing.T) {
	for _, k := range Keys(FR) {
		if _, ok := en[k]; !ok {
			t.Errorf("key %q missing in en", k)
		}
	}
	for _, k := range Keys(EN) {
		if _, ok := fr[k]; !ok {
			t.Errorf("key %q missing in fr", k)
		}
	}
}
