package tui

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kevin/log-tui/internal/parse"
)

var (
	jKey  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	jStr  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	jNum  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	jLit  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	jPunc = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// prettyJSON indente et colore un document JSON. Les chaînes qui contiennent
// elles-mêmes du JSON (objets/tableaux sérialisés) sont décodées récursivement.
// Les clés sont triées.
func prettyJSON(doc string) (string, bool) {
	var v any
	dec := json.NewDecoder(strings.NewReader(doc))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	var b strings.Builder
	writeJSON(&b, expandNested(v, 0), 0)
	return b.String(), true
}

func expandNested(v any, depth int) any {
	if depth > 6 {
		return v
	}
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = expandNested(e, depth+1)
		}
	case []any:
		for i, e := range x {
			x[i] = expandNested(e, depth+1)
		}
	case string:
		t := strings.TrimSpace(x)
		if len(t) > 1 && (t[0] == '{' || t[0] == '[') {
			var inner any
			d := json.NewDecoder(strings.NewReader(t))
			d.UseNumber()
			if d.Decode(&inner) == nil && !d.More() {
				return expandNested(inner, depth+1)
			}
		}
	}
	return v
}

func writeJSON(b *strings.Builder, v any, depth int) {
	ind := strings.Repeat("  ", depth)
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			b.WriteString(jPunc.Render("{}"))
			return
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(jPunc.Render("{") + "\n")
		for i, k := range keys {
			b.WriteString(ind + "  " + jKey.Render(strconv.Quote(k)) + jPunc.Render(": "))
			writeJSON(b, x[k], depth+1)
			if i < len(keys)-1 {
				b.WriteString(jPunc.Render(","))
			}
			b.WriteString("\n")
		}
		b.WriteString(ind + jPunc.Render("}"))
	case []any:
		if len(x) == 0 {
			b.WriteString(jPunc.Render("[]"))
			return
		}
		b.WriteString(jPunc.Render("[") + "\n")
		for i, e := range x {
			b.WriteString(ind + "  ")
			writeJSON(b, e, depth+1)
			if i < len(x)-1 {
				b.WriteString(jPunc.Render(","))
			}
			b.WriteString("\n")
		}
		b.WriteString(ind + jPunc.Render("]"))
	case string:
		b.WriteString(jStr.Render(strconv.Quote(x)))
	case json.Number:
		b.WriteString(jNum.Render(x.String()))
	case bool:
		b.WriteString(jLit.Render(strconv.FormatBool(x)))
	case nil:
		b.WriteString(jLit.Render("null"))
	}
}

// compactFields résume les champs JSON non méta d'une ligne en "k=v k=v".
func compactFields(doc string) string {
	var obj map[string]any
	d := json.NewDecoder(strings.NewReader(doc))
	d.UseNumber()
	if d.Decode(&obj) != nil {
		return ""
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		if !parse.IsMetaKey(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		var val string
		switch x := obj[k].(type) {
		case string:
			val = x
		default:
			bs, _ := json.Marshal(x)
			val = string(bs)
		}
		parts = append(parts, k+"="+val)
	}
	return strings.Join(parts, " ")
}
