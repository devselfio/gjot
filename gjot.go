// Package gjot parses Djot markup into an AST and renders it as HTML, as
// djot.js AST text, or back to Djot. It is a port of zjot and is proven
// against zjot's vendored djot.js corpus (testdata/spec).
//
// This package holds the entry points and their options. The tree they
// produce and read is declared in package github.com/devselfio/gjot/ast:
// the node kinds, Walk, WalkStatus and UnmarshalNode.
package gjot

import (
	"errors"
	"io"

	"github.com/devselfio/gjot/ast"
	"github.com/devselfio/gjot/internal/djot"
)

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

// WithSourcePos records each node's source range in its header's Pos.
func WithSourcePos() ParseOption {
	return func(c *parseConfig) { c.sourcePos = true }
}

// Parse parses Djot source into an AST, whose root is an *ast.Section of
// Level 0. It never fails: every byte sequence is a Djot document. Nesting
// past MaxNesting is read as text; ParseErr reports it.
func Parse(src []byte, opts ...ParseOption) ast.Node {
	doc, _ := ParseErr(src, opts...)
	return doc
}

// ParseErr is Parse that also reports ErrTooDeep when the source nested past
// MaxNesting. The tree is returned either way and is the one Parse returns.
func ParseErr(src []byte, opts ...ParseOption) (ast.Node, error) {
	var cfg parseConfig
	for _, o := range opts {
		o(&cfg)
	}
	doc, tooDeep := djot.Parse(string(src), cfg.sourcePos)
	if tooDeep {
		return doc, ErrTooDeep
	}
	return doc, nil
}

// HTMLOption configures HTML.
type HTMLOption func(*htmlConfig)

type htmlConfig struct {
	hooks map[string]djot.Hook
}

// Hook renders a div or span in place of the default markup. It is called
// with entering true before the children and entering false after them;
// the exit call's WalkStatus is ignored. Returning an error aborts the
// render and HTML returns it.
type Hook func(w io.Writer, n ast.Node, entering bool) (ast.WalkStatus, error)

// WithHook registers h for every div or span that has class. When a node
// has several registered classes, the first in its class list wins.
func WithHook(class string, h Hook) HTMLOption {
	return func(c *htmlConfig) {
		if c.hooks == nil {
			c.hooks = map[string]djot.Hook{}
		}
		c.hooks[class] = djot.Hook(h)
	}
}

// HTML renders doc as HTML, byte for byte as djot.js does. The error is nil
// unless a hook returns one.
func HTML(doc ast.Node, opts ...HTMLOption) ([]byte, error) {
	var cfg htmlConfig
	for _, o := range opts {
		o(&cfg)
	}
	return djot.HTML(doc, cfg.hooks)
}

// ASTText renders doc in djot.js's indented AST text format, the format the
// corpus compares against for its "a" cases.
func ASTText(doc ast.Node) []byte {
	return djot.ASTText(doc)
}

// Djot writes doc back as Djot source. The result re-parses to the same
// HTML and AST (positions aside); it is not the original text. Reference
// definitions are written as inline destinations and footnote definitions
// with their labels, where the ids their headings generated demand it
// (due), else after the document.
func Djot(doc ast.Node) []byte {
	return djot.Djot(doc)
}
