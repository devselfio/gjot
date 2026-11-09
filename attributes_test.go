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
		// The list is djot.js's attribute object: id and class are keys in
		// their first position, with their last value; a `.c` joins the
		// classes, a `class=` key starts them over, and an empty `#` or `.`
		// names nothing (zjot.test L9844, L10252, L10264, L7000).
		{"id classes key comment", `{#id .a .b key="v" %c%}`,
			blockAttrs{id: "id", attrs: []Attr{{"id", "id"}, {"class", "a b"}, {"key", "v"}}}, true},
		{"id key over hash, in the first place", `{#y id=x}`,
			blockAttrs{id: "x", attrs: []Attr{{"id", "x"}}}, true},
		{"two hashes", `{#a #b}`, blockAttrs{id: "b", attrs: []Attr{{"id", "b"}}}, true},
		{"id keeps its place among keys", `{#i k=v #j .c}`,
			blockAttrs{id: "j", attrs: []Attr{{"id", "j"}, {"k", "v"}, {"class", "c"}}}, true},
		{"repeated key takes the last value", `{k=a k=b}`,
			blockAttrs{attrs: []Attr{{"k", "b"}}}, true},
		{"class key starts over", `{.a class="b c"}`,
			blockAttrs{attrs: []Attr{{"class", "b c"}}}, true},
		{"class key then dot joins", `{.a class=b .c}`,
			blockAttrs{attrs: []Attr{{"class", "b c"}}}, true},
		{"empty hash names nothing", `{#}`, blockAttrs{}, true},
		{"empty dot names nothing", `{.a .}`, blockAttrs{attrs: []Attr{{"class", "a"}}}, true},
		{"empty id key is an attribute and no id", `{id=""}`,
			blockAttrs{attrs: []Attr{{"id", ""}}}, true},
		{"bare value", `{k=v}`, blockAttrs{attrs: []Attr{{"k", "v"}}}, true},
		{"quoted value with escape and newline", "{key=\"bar\na\\$bim\"}",
			blockAttrs{attrs: []Attr{{"key", "bar a$bim"}}}, true},
		// A backslash goes before djot.js's set of punctuation and stays
		// before "<", "@" and a space; runs of spaces and line ends, an
		// escaped line end included, are one space (zjot.test L9333, L9420).
		{"quoted value escapes", `{k="x\,y\<z\@w\ v\\u\"q"}`,
			blockAttrs{attrs: []Attr{{"k", `x,y\<z\@w\ v\u"q`}}}, true},
		{"quoted value blanks collapse", "{k=\"x\\\ny  z \n w\"}",
			blockAttrs{attrs: []Attr{{"k", "x\\ y z w"}}}, true},
		{"comment only", `{%c%}`, blockAttrs{}, true},
		{"incomplete", `{#id`, blockAttrs{}, false},
		{"bad character", `{#id !}`, blockAttrs{}, false},
		{"class placeholder keeps first class position", `{a=1 .x b=2 .y}`,
			blockAttrs{attrs: []Attr{{"a", "1"}, {"class", "x y"}, {"b", "2"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, end, ok := parseInlineAttrs(tt.src, 0)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && end != len(tt.src) {
				t.Errorf("end = %d, want %d", end, len(tt.src))
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

// merge layers a later attribute over an earlier one as djot.js applies one
// attribute object to another: first position, last value; classes join,
// from a `.c` or a `class=` key alike (zjot.test difference 16, L6978, L10289).
func TestBlockAttrsMerge(t *testing.T) {
	tests := []struct {
		name, first, second string
		want                blockAttrs
	}{
		{"id class keys", `{#one .a k=1}`, `{#two .b k=2 j=3}`,
			blockAttrs{id: "two", attrs: []Attr{{"id", "two"}, {"class", "a b"}, {"k", "2"}, {"j", "3"}}}},
		{"earlier id kept", `{#one}`, `{j=3}`,
			blockAttrs{id: "one", attrs: []Attr{{"id", "one"}, {"j", "3"}}}},
		{"class then id", `{.x}`, `{#i}`,
			blockAttrs{id: "i", attrs: []Attr{{"class", "x"}, {"id", "i"}}}},
		{"key then keys", `{k=1}`, `{j=2 k=3}`,
			blockAttrs{attrs: []Attr{{"k", "3"}, {"j", "2"}}}},
		{"class key joins a later block attribute", `{.a}`, `{class=b}`,
			blockAttrs{attrs: []Attr{{"class", "a b"}}}},
		{"empty id key overrides", `{#x}`, `{id=""}`,
			blockAttrs{attrs: []Attr{{"id", ""}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := wholeAttrs(t, tt.first)
			b := wholeAttrs(t, tt.second)
			if got := a.merge(b); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// writesID is whether an id was written, empty included: an earlier id it
// overrides is registered all the same (zjot.test L10390), while an empty
// `#` writes none (L9844).
func TestBlockAttrsWritesID(t *testing.T) {
	for src, want := range map[string]bool{`{#x}`: true, `{id=""}`: true, `{#}`: false, `{.c}`: false} {
		ba := wholeAttrs(t, src)
		if got := ba.writesID(); got != want {
			t.Errorf("%s: writesID = %v, want %v", src, got, want)
		}
	}
}

// wholeAttrs parses src, which must be one whole attribute set, as
// parseInlineAttrs reads it.
func wholeAttrs(t *testing.T, src string) blockAttrs {
	t.Helper()
	ba, end, ok := parseInlineAttrs(src, 0)
	if !ok || end != len(src) {
		t.Fatalf("%s: ok=%v end=%d, want true %d", src, ok, end, len(src))
	}
	return ba
}
