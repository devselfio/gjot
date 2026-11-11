package djot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devselfio/gjot/ast"
)

// linkFields reaches a link's and an image's own fields, so reference
// resolution writes through it, and is not ok on any other kind.
func TestLinkFieldsWriteThrough(t *testing.T) {
	dest := "x"
	for _, n := range []ast.Node{&ast.Link{}, &ast.Image{}} {
		d, r, _, ok := linkFields(n)
		if !ok {
			t.Fatalf("%s: linkFields not ok", n.Tag())
		}
		*d, *r = &dest, "ref"
		got, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), `"destination":"x","reference":"ref"`) {
			t.Errorf("%s: linkFields did not write through: %s", n.Tag(), got)
		}
	}
	if _, _, _, ok := linkFields(&ast.URL{}); ok {
		t.Error("linkFields ok on a url")
	}
}
