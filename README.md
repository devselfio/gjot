# gjot

A [Djot](https://djot.net/) library for Go: parser, HTML renderer with a
div/span hook, Djot writer, and a JSON-serialisable AST. gjot is a port of
[zjot](https://github.com/quiteclose/zjot) and is tested against the corpus
zjot ships: the 28 `.test` files of zjot v2.0.0 (the 26
[djot.js](https://github.com/jgm/djot.js) suite files it vendors plus its own
`grid.test` and `zjot.test`), pinned in `testdata/spec/ORIGIN.md`.

## Build

Requires Go 1.26 or later.

```
go build ./...
make build      # the same, as CI runs it
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
when parsed with `WithSourcePos`. Blocks carry them at any nesting depth, as do
the inline nodes inside them; a definition list item's term and definition and
the footnote section built at the end do not yet.

Two things the writer cannot express, because the syntax has no form for them:
an empty verbatim span in the middle of a paragraph (it is written as a trailing
unclosed backtick) and math whose content starts or ends with a backtick.

## Tests

```
go test ./...
make test       # the same, as CI runs it
make check      # gofmt (fails listing unformatted files), go vet, go mod tidy -diff
```

CI runs `make check`, `make build`, `make test` and `make spec-count` on every
push to `main` and every pull request. `make help` lists the targets.

`TestSpec` runs the `.test` files under `testdata/spec/`: the 28 `.test` files
of zjot v2.0.0 (the 26 djot.js suite files it vendors plus its own `grid.test`
and `zjot.test`), vendored byte for byte and pinned in
`testdata/spec/ORIGIN.md`. Each case must render its expected output, and must
render it again after being written back to Djot by `Djot` and re-parsed (for
AST cases, the AST must not change). The 3 cases of `filters.test` skip because
they need a Lua filter.

`testdata/spec/known-failures.txt` lists the cases gjot does not yet pass, one
per line as `<file>.test L<line>`, the line being that of the case's opening
fence (e.g. `zjot.test L1234`), sorted by file then line; `#` comments and blank
lines are allowed. Against that list:

- a listed case that fails is reported as a known failure (a skipped subtest
  with the entry and the diff) and does not fail the run;
- a listed case that passes fails the run, naming the entry to delete;
- an unlisted case that fails fails the run, naming the entry it would need;
- an entry that names no case fails the run.

The list only shrinks. The pass count is what the run prints, not a number kept
here:

```
make spec-count
# or, without make:
go test -run 'TestSpec$' -v . | grep 'corpus:'
```

`TestJSONRoundTrip` marshals every case's AST to JSON and back and compares the
AST text, with and without source positions.

`go run ./cmd/driftcheck` compares that corpus with zjot's at the ref pinned in
`ORIGIN.md` and exits non-zero naming each file that differs. It needs the
network, so it is not part of `go test`.

## Architecture

gjot's block parser is a container stack, in the shape of zjot v2.0.0's
`Parser.zig` with the Zig allocator plumbing and the streaming removed. Each
line is run against the open containers from the outermost in; while the
innermost container it continues holds blocks, the line may open new ones; what
remains is lazy text for an open paragraph, a blank line, or text for the
innermost container. A container closes when a line neither continues it nor is
lazy text for it. One pass through the input builds the whole `Node` tree. Ids
are registered where they stand; references, footnotes and sections are resolved
after the pass.

```
gjot.go          Public API: Parse, WithSourcePos, HTML
node.go          AST types (Node, Tag, Attr, SourcePos, Range) and hook helpers
parser.go        Block parsing: the container stack over lines
leaf.go          What one line says (fences, markers, table rows, ...)
document.go      Definitions, ids, references, sections, footnotes
inline.go        Inline parsing (emphasis, links, smart quotes, ...)
attributes.go    Attribute parser ({#id .class key=value})
linemap.go       Source position mapping for joined inline text
html.go          HTML renderer and the class hook
ast.go           AST text renderer
writer.go        Djot writer
spec_test.go     Corpus runner, judged against testdata/spec/known-failures.txt
cmd/gjot         CLI
cmd/driftcheck   Corpus drift check against zjot
```
