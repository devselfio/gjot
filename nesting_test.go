package gjot

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestNestingLimit pins MaxNesting: at the limit the tree is complete and
// ParseErr returns nil; past it nesting stops at the limit, the marker that
// would go deeper is text, and ParseErr returns ErrTooDeep. Parse returns
// the same tree, and every renderer runs on it.
func TestNestingLimit(t *testing.T) {
	n := MaxNesting
	tests := []struct {
		name    string
		src     string
		wantErr bool
		check   func(t *testing.T, html string)
	}{
		{"blocks-at-limit", strings.Repeat("> ", n) + "a\n", false, func(t *testing.T, html string) {
			wantCount(t, html, "<blockquote>", n)
			if !strings.Contains(html, "<p>a</p>") {
				t.Errorf("no <p>a</p> in the innermost quote")
			}
		}},
		{"blocks-past-limit", strings.Repeat("> ", n+1) + "a\n", true, func(t *testing.T, html string) {
			wantCount(t, html, "<blockquote>", n)
			if !strings.Contains(html, "<p>&gt; a</p>") {
				t.Errorf("the marker past the limit is not text: no <p>&gt; a</p>")
			}
		}},
		{"inlines-at-limit", strings.Repeat("{_", n) + "x" + strings.Repeat("_}", n), false, func(t *testing.T, html string) {
			wantCount(t, html, "<em>", n)
			if !strings.Contains(html, "<em>x</em>") {
				t.Errorf("no <em>x</em> innermost")
			}
		}},
		{"inlines-past-limit", strings.Repeat("{_", n+1) + "x" + strings.Repeat("_}", n+1), true, func(t *testing.T, html string) {
			wantCount(t, html, "<em>", n)
			if !strings.Contains(html, "<em>{_x</em>") {
				t.Errorf("the opener past the limit is not text: no <em>{_x</em>")
			}
			if !strings.HasSuffix(strings.TrimSpace(html), "_}</p>") {
				t.Errorf("the unmatched closer is not trailing text: %q", html[max(0, len(html)-40):])
			}
		}},
		{"list-counts-two", strings.Repeat("- ", n/2) + "a\n", false, func(t *testing.T, html string) {
			wantCount(t, html, "<ul>", n/2)
			wantCount(t, html, "<li>", n/2)
		}},
		{"list-counts-two-past-limit", strings.Repeat("- ", n/2+1) + "a\n", true, func(t *testing.T, html string) {
			wantCount(t, html, "<ul>", n/2)
			wantCount(t, html, "<li>", n/2)
			if !strings.Contains(html, "<li>\n- a\n</li>") {
				t.Errorf("the marker past the limit is not the innermost item's text")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseErr([]byte(tt.src))
			if tt.wantErr {
				if !errors.Is(err, ErrTooDeep) {
					t.Fatalf("ParseErr error = %v, want ErrTooDeep", err)
				}
			} else if err != nil {
				t.Fatalf("ParseErr error = %v, want nil", err)
			}
			if doc == nil {
				t.Fatal("ParseErr returned no tree")
			}
			ast := ASTText(doc)
			if got := ASTText(Parse([]byte(tt.src))); !bytes.Equal(got, ast) {
				t.Errorf("Parse and ParseErr disagree on the tree")
			}
			html, herr := HTML(doc)
			if herr != nil {
				t.Fatalf("HTML: %v", herr)
			}
			tt.check(t, string(html))
			if len(Djot(doc)) == 0 {
				t.Errorf("Djot wrote nothing")
			}
			if _, jerr := json.Marshal(doc); jerr != nil {
				t.Errorf("json.Marshal: %v", jerr)
			}
		})
	}
}

func wantCount(t *testing.T, s, sub string, want int) {
	t.Helper()
	if got := strings.Count(s, sub); got != want {
		t.Errorf("%d of %q, want %d", got, sub, want)
	}
}
