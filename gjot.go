package gjot

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
// is a Djot document.
func Parse(src []byte, opts ...ParseOption) *Node {
	var cfg parseConfig
	for _, o := range opts {
		o(&cfg)
	}
	return parseStack(string(src), cfg.sourcePos)
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
	r.render(doc)
	if r.err != nil {
		return nil, r.err
	}
	return r.out, nil
}
