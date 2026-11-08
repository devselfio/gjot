package gjot

// ParseOption configures Parse.
type ParseOption func(*parseConfig)

type parseConfig struct {
	sourcePos bool
	// stack selects the container stack (stack.go) over the window parser
	// while the stack is built beside it (farraway/gridapp#12).
	stack bool
}

// WithSourcePos records each node's source range in Node.Pos.
func WithSourcePos() ParseOption {
	return func(c *parseConfig) { c.sourcePos = true }
}

// withStack makes Parse use the container stack. Unexported: for the tests,
// until the stack replaces the window parser.
func withStack() ParseOption {
	return func(c *parseConfig) { c.stack = true }
}

// Parse parses Djot source into an AST. It never fails: every byte sequence
// is a Djot document.
func Parse(src []byte, opts ...ParseOption) *Node {
	var cfg parseConfig
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.stack {
		return parseStack(string(src), cfg.sourcePos)
	}
	p := newParser(string(src), newSharedState())
	p.trackPos = cfg.sourcePos
	return p.parseDoc()
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
