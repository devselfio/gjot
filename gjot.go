package gjot

import "errors"

// MaxNesting is how deep the parser nests: at most MaxNesting block
// containers (block quotes, divs, lists and their items, footnote
// definitions, ...) and, inside a leaf, at most MaxNesting inline elements.
// A block marker, inline opener or attribute span that would nest deeper is
// text. Inline openers are counted while open, closed later or not, so more
// than MaxNesting unclosed openers in one leaf also stop there. zjot's
// limit is the same number.
const MaxNesting = 1000

// ErrTooDeep is ParseErr's error when the source nested past MaxNesting and
// the parser stopped nesting there.
var ErrTooDeep = errors.New("gjot: nesting deeper than MaxNesting")

// ParseOption configures Parse.
type ParseOption func(*parseConfig)

type parseConfig struct {
	sourcePos bool
}

// WithSourcePos records each node's source range in Node.Pos.
func WithSourcePos() ParseOption {
	return func(c *parseConfig) { c.sourcePos = true }
}

// Parse parses Djot source into an AST. It never fails: every byte sequence
// is a Djot document. Nesting past MaxNesting is read as text; ParseErr
// reports it.
func Parse(src []byte, opts ...ParseOption) *Node {
	doc, _ := ParseErr(src, opts...)
	return doc
}

// ParseErr is Parse that also reports ErrTooDeep when the source nested past
// MaxNesting. The tree is returned either way and is the one Parse returns.
func ParseErr(src []byte, opts ...ParseOption) (*Node, error) {
	var cfg parseConfig
	for _, o := range opts {
		o(&cfg)
	}
	doc, tooDeep := parseStack(string(src), cfg.sourcePos)
	if tooDeep {
		return doc, ErrTooDeep
	}
	return doc, nil
}

// HTMLOption configures HTML.
type HTMLOption func(*renderer)

// HTML renders doc as HTML, byte for byte as djot.js does. The error is nil
// unless a hook returns one.
func HTML(doc *Node, opts ...HTMLOption) ([]byte, error) {
	r := &renderer{}
	for _, o := range opts {
		o(r)
	}
	r.render(fromLegacyLinks(doc, &r.linkTexts))
	r.endnotes()
	if r.err != nil {
		return nil, r.err
	}
	return r.out, nil
}
