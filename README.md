# gjot

A [Djot](https://djot.net/) library for Go: parser, HTML renderer with a
div/span hook, Djot writer, and a JSON-serialisable AST. gjot is a port of
[zjot](https://github.com/quiteclose/zjot) and passes the 261 test cases of
the canonical [djot.js](https://github.com/jgm/djot.js) suite that zjot vendors.

## Build

Requires Go 1.26 or later.

```
go build ./...
```

## CLI usage

```
gjot [OPTIONS] [FILE]
```

If `FILE` is omitted, reads from stdin.

**Options:**

| Flag | Description |
| --- | --- |
| `--ast` | Output AST text instead of HTML |
| `--sourcepos` | Include source positions in AST output |
| `--json` | Output the AST as JSON |
| `--djot` | Output the AST written back as Djot |
| `-h`, `--help` | Show help |

**Examples:**

```sh
# Render Djot to HTML
echo 'Hello *world*' | go run ./cmd/gjot

# Render a file
go run ./cmd/gjot document.dj

# View the AST, with source positions
go run ./cmd/gjot --ast --sourcepos document.dj

# Normalise a document by writing its AST back out
go run ./cmd/gjot --djot document.dj
```

## Library usage

```sh
go get github.com/quiteclose/gjot
```

```go
import "github.com/quiteclose/gjot"

doc := gjot.Parse([]byte("Hello *world*"))
html, err := gjot.HTML(doc)

// Render fenced divs of a class yourself: `::: chess {fen=...}`
board := func(w io.Writer, n *gjot.Node, entering bool) (gjot.WalkStatus, error) {
	if entering {
		fmt.Fprintf(w, "<chess-board fen=%q></chess-board>\n", n.Attr("fen"))
	}
	return gjot.WalkSkipChildren, nil
}
html, err = gjot.HTML(doc, gjot.WithHook("chess", board))
```

| Function | Description |
| --- | --- |
| `Parse(src []byte, opts ...ParseOption) *Node` | Parse Djot into an AST. Never fails: every input is a document |
| `WithSourcePos() ParseOption` | Record each node's source range in `Node.Pos` |
| `HTML(doc *Node, opts ...HTMLOption) ([]byte, error)` | Render HTML, byte-identical to djot.js. The error comes only from a hook |
| `WithHook(class string, h Hook) HTMLOption` | Render divs and spans of that class with `h`; `WalkSkipChildren` drops the default child rendering |
| `ASTText(doc *Node) []byte` | Render the AST in djot.js's indented text format |
| `Djot(doc *Node) []byte` | Write the AST back as Djot. Re-parses to the same HTML and AST; reference links keep their labels with the definitions collected at the end, footnotes are renumbered |
| `(*Node).HasClass(string) bool`, `(*Node).Attr(string) string` | Class and attribute lookup for hooks |

`Node` carries `encoding/json` struct tags; `json.Marshal(doc)` round-trips
through `json.Unmarshal` to the same AST text. Source positions are kept only
when parsed with `WithSourcePos`, and are recorded for paragraphs and text at
any nesting depth.

Two things the writer cannot express, because the syntax has no form for them:
an empty verbatim span in the middle of a paragraph (it is written as a trailing
unclosed backtick) and math whose content starts or ends with a backtick.

## Tests

```
go test ./...
```

`TestSpec` runs the 26 `.test` files under `testdata/spec/`, vendored byte for
byte from zjot (see `testdata/spec/ORIGIN.md`): 261 cases pass, the 3 cases of
`filters.test` skip because they need a Lua filter. `TestWriterRoundTripsCorpus`
re-parses every case through `Djot` and compares the HTML again. `TestJSONRoundTrip`
does the same through JSON.

## Architecture

gjot is a single-phase recursive descent parser, file for file the shape of
zjot with the Zig allocator plumbing removed. One pass through the input builds
the `Node` tree directly; references, footnotes and sections are resolved after
the block pass.

```
gjot.go          Public API: Parse, WithSourcePos, HTML
node.go          AST types (Node, Tag, Attr, SourcePos, Range) and hook helpers
parser.go        Block parsing, references, footnotes, sections, position tracking
lists.go         Bullet, ordered, task and definition lists
tables.go        Pipe tables and captions
inline.go        Inline parsing (emphasis, links, smart quotes, ...)
attributes.go    Attribute parser ({#id .class key=value})
linemap.go       Source position mapping for joined inline text
html.go          HTML renderer and the class hook
ast.go           AST text renderer
writer.go        Djot writer
spec_test.go     Corpus runner
cmd/gjot         CLI
```
