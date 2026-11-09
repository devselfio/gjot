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

`make help` lists every target CI runs, including the drift check,
`make spec-drift` (see Tests).

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
| `Parse(src []byte, opts ...ParseOption) *Node` | Parse Djot into an AST. Never fails: every input is a document. Blocks nest at most `MaxNesting` (1,000) containers deep and a leaf's inlines at most `MaxNesting` deep; a marker that would nest deeper is text |
| `ParseErr(src []byte, opts ...ParseOption) (*Node, error)` | `Parse` that also returns `ErrTooDeep` when the limit stopped nesting; the tree is the same |
| `WithSourcePos() ParseOption` | Record each node's source range in `Node.Pos` |
| `HTML(doc *Node, opts ...HTMLOption) ([]byte, error)` | Render HTML, byte-identical to djot.js. The error comes only from a hook |
| `WithHook(class string, h Hook) HTMLOption` | Render divs and spans of that class with `h`; `WalkSkipChildren` drops the default child rendering |
| `ASTText(doc *Node) []byte` | Render the AST in djot.js's indented text format |
| `Djot(doc *Node) []byte` | Write the AST back as Djot. Re-parses to the same HTML and AST; reference links keep their labels with the definitions collected at the end, footnote definitions keep their labels |
| `(*Node).HasClass(string) bool`, `(*Node).Attr(string) string` | Class and attribute lookup for hooks |

The nesting limit bounds the tree: it is never deeper than about twice
`MaxNesting` plus a few levels for leaves, table rows and sections, which
`HTML`, `ASTText`, `Djot` and `encoding/json` all recurse through safely, so
untrusted input cannot exhaust the stack. A block quote, div, list, list item
or footnote counts one container (a list and its item two), as does a leaf
block other than a paragraph. In a leaf, each inline opener counts while it
is open, whether or not it closes later, and the span an inline attribute
makes around a word counts one more; so more than `MaxNesting` unclosed `[`
in one paragraph also report `ErrTooDeep`, though nothing nests. Where zjot
returns `error.TooDeep` and no document, gjot returns the tree; check
`errors.Is(err, gjot.ErrTooDeep)` from `ParseErr` to reject or flag such input.

`Node` carries `encoding/json` struct tags; `json.Marshal(doc)` round-trips
through `json.Unmarshal` to the same AST text and HTML. A smart quote keeps the
quote it was written as in `"source"` (`Node.Source`, empty on every other
node), which alt text, heading ids and the writer read. An ordered list keeps
djot.js's style in `"style"` (`"1."`, `"(a)"`, `"I)"`) and its first marker's
text in `"text"`, from which the HTML `start` is read. Source positions are kept only
when parsed with `WithSourcePos`. Blocks carry them at any nesting depth, as do
the inline nodes inside them. A definition list item's term and definition
carry none, as zjot's AST prints none. Footnote definitions stand after the
document's blocks as `footnote` nodes with their label in `"text"`; the HTML
renderer numbers the endnotes from them.

One thing the writer cannot express, because the syntax has no form for it: an
empty verbatim or math span in the middle of a paragraph (it is written as a
trailing unclosed backtick).

## Tests

```
go test ./...
make test       # the same, as CI runs it
make check      # gofmt (fails listing unformatted files), go vet, go mod tidy -diff
```

CI runs `make check`, `make build`, `make test`, `make spec-count` and
`make spec-drift` on every push to `main` and every pull request. `make help`
lists the targets.

`TestSpec` runs the `.test` files under `testdata/spec/`: the 28 `.test` files
of zjot v2.0.0 (the 26 djot.js suite files it vendors plus its own `grid.test`
and `zjot.test`), 1066 cases vendored byte for byte and pinned in
`testdata/spec/ORIGIN.md`. Each case must render its expected output, and must
render it again after being written back to Djot by `Djot` and re-parsed (for
AST cases, the AST must not change); a failing case fails the run. The 3 cases
of `filters.test` skip because they need a Lua filter.

The pass count is what the run prints, not a number kept here:

```
make spec-count
# or, without make:
go test -run 'TestSpec$' -v . | grep 'corpus:'
```

`TestJSONRoundTrip` marshals every case's AST to JSON and back and compares the
AST text and the HTML, with and without source positions.

`make spec-drift` (or `go run ./cmd/driftcheck`) checks that corpus against the
tag pinned in `ORIGIN.md`, which must be a `vN.N.N` tag, never a branch.
Offline it compares the files with their checked-in record,
`testdata/spec/SHA256SUMS`; when the network answers it also fetches zjot at
the tag and compares with that. It exits 1 naming each file added, removed or
changed, and also when `ORIGIN.md` pins a tag that `SHA256SUMS` does not
record, so bumping the pin without re-recording the corpus is red offline and
online. `go run ./cmd/driftcheck -record` writes `SHA256SUMS` after an online
match with zjot at the pinned tag; `go run ./cmd/driftcheck REF` measures the
gap to another ref without moving the pin. `go test ./cmd/driftcheck` covers all
of it against local test servers, and its `TestRecordRealCorpus` runs the
offline check on the real corpus. CI runs `make spec-drift` as the last step of
the `check` job, where the network answers, so it fetches zjot at the tag;
bumping `Ref:` without re-recording the corpus is the one way to turn that step
red on purpose. `TestWorkflowRunsDriftcheck` fails if that step leaves the
workflow.

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
gjot.go          Public API: Parse, ParseErr, MaxNesting, WithSourcePos, HTML
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
spec_test.go     Corpus runner over testdata/spec/*.test
cmd/gjot         CLI
cmd/driftcheck   Corpus drift check against zjot
```

## Changes

- 2026-11-09. Removed from the public API: the `Tag` constants
  `ReferenceDefinition`, `Escape`, `LeftSingleQuote`, `RightSingleQuote`,
  `LeftDoubleQuote` and `RightDoubleQuote`, which `Parse` never produced and no
  renderer needed; and the `Node.Classes` field (JSON `classes`), which `Parse`
  never set. A node's classes are its `class` entry in `Attrs`, read by
  `HasClass`. `json.Unmarshal` now ignores a `classes` key in a `Node`
  document; gjot never wrote one.
