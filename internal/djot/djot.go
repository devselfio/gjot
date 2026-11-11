// Package djot is gjot's implementation: the block and inline parsers,
// document finalisation and source positions, and the HTML, AST text and
// Djot renderers. The gjot package wraps it with the public entry points;
// it reads and builds trees of package ast and never imports gjot.
package djot

import "github.com/devselfio/gjot/ast"

// MaxNesting is how deep the parser nests; gjot.MaxNesting documents it.
const MaxNesting = 1000

// Parse parses src into a tree whose root is a *ast.Section of Level 0,
// recording source ranges when sourcePos is set. tooDeep reports that the
// source nested past MaxNesting and the parser stopped nesting there; the
// tree is complete either way.
func Parse(src string, sourcePos bool) (doc ast.Node, tooDeep bool) {
	return parseStack(src, sourcePos)
}
