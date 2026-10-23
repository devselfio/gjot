package gjot

import (
	"reflect"
	"testing"
)

func TestParseAttrs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want blockAttrs
		ok   bool
	}{
		{"id classes key comment", `{#id .a .b key="v" %c%}`,
			blockAttrs{id: "id", attrs: []Attr{{"class", "a b"}, {"key", "v"}}}, true},
		{"bare value", `{k=v}`, blockAttrs{attrs: []Attr{{"k", "v"}}}, true},
		{"quoted value with escape and newline", "{key=\"bar\na\\$bim\"}",
			blockAttrs{attrs: []Attr{{"key", "bar a$bim"}}}, true},
		{"comment only", `{%c%}`, blockAttrs{}, true},
		{"incomplete", `{#id`, blockAttrs{}, false},
		{"bad character", `{#id !}`, blockAttrs{}, false},
		{"class placeholder keeps first class position", `{a=1 .x b=2 .y}`,
			blockAttrs{attrs: []Attr{{"a", "1"}, {"class", "x y"}, {"b", "2"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseAttrsFromStr(tt.src)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseInlineAttrsEnd(t *testing.T) {
	ba, end, ok := parseInlineAttrs("x{.c} y", 1)
	if !ok || end != 5 {
		t.Fatalf("ok=%v end=%d, want true 5", ok, end)
	}
	if want := []Attr{{"class", "c"}}; !reflect.DeepEqual(ba.attrs, want) {
		t.Errorf("attrs = %+v, want %+v", ba.attrs, want)
	}
	if _, _, ok := parseInlineAttrs("x{.c y", 1); ok {
		t.Error("unterminated attributes parsed as ok")
	}
}

func TestTryParseBlockAttr(t *testing.T) {
	if _, ok := tryParseBlockAttr("  {#id}  "); !ok {
		t.Error("surrounding whitespace rejected")
	}
	if _, ok := tryParseBlockAttr("{#id} tail"); ok {
		t.Error("trailing text accepted")
	}
}

func TestBlockAttrsMerge(t *testing.T) {
	a, _ := parseAttrsFromStr(`{#one .a k=1}`)
	b, _ := parseAttrsFromStr(`{#two .b k=2 j=3}`)
	got := a.merge(b)
	want := blockAttrs{id: "two", attrs: []Attr{{"k", "2"}, {"j", "3"}, {"class", "a b"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := a.merge(blockAttrs{attrs: []Attr{{"j", "3"}}}); got.id != "one" {
		t.Errorf("id = %q, want earlier id kept when later has none", got.id)
	}
}
