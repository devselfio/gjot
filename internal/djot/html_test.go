package djot

import "testing"

// TestJSArrayIndex checks which footnote labels JavaScript reads as array
// indexes, which order first among the endnotes (zjot.test L10160, L10192;
// gjot's TestFootnoteEndnotes checks the order they render in).
func TestJSArrayIndex(t *testing.T) {
	for _, c := range []struct {
		label string
		index int
		ok    bool
	}{
		{"0", 0, true}, {"10", 10, true}, {"4294967294", 4294967294, true},
		{"4294967295", 0, false}, {"01", 0, false}, {"-1", 0, false}, {"", 0, false}, {"a", 0, false}, {"12345678901", 0, false},
	} {
		if i, ok := jsArrayIndex(c.label); i != c.index || ok != c.ok {
			t.Errorf("jsArrayIndex(%q) = %d, %v; want %d, %v", c.label, i, ok, c.index, c.ok)
		}
	}
}
