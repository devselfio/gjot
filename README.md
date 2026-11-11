# gjot

A [Djot](https://djot.net/) library for Go: parser, HTML renderer with a
div/span hook, Djot writer, and a JSON-serialisable AST. gjot is a port of
[zjot](https://github.com/devselfio/zjot) and is tested against the corpus
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
go get github.com/devselfio/gjot
```

```go
import (
	"github.com/devselfio/gjot"
	"github.com/devselfio/gjot/ast"
)

doc := gjot.Parse([]byte("Hello *world*"))
html, err := gjot.HTML(doc)

// Render fenced divs of a class yourself: `::: chess {fen=...}`
board := func(w io.Writer, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		fmt.Fprintf(w, "<chess-board fen=%q></chess-board>\n", n.Attr("fen"))
	}
	return ast.WalkSkipChildren, nil
}
html, err = gjot.HTML(doc, gjot.WithHook("chess", board))

// Read the tree: one Go type per node kind, behind the Node interface.
ast.Walk(doc, func(n ast.Node) ast.WalkStatus {
	switch n := n.(type) {
	case *ast.Heading:
		fmt.Println("heading", n.Level, n.ID)
	case *ast.Link:
		if n.Destination != nil {
			fmt.Println("link to", *n.Destination)
		}
	case *ast.CodeBlock:
		return ast.WalkSkipChildren // a leaf; shown for the skip
	}
	return ast.WalkContinue
})
```

Two packages make the library, as `go/parser` and `go/ast` make Go's:
`github.com/devselfio/gjot` parses and renders (`Parse`, `ParseErr`, `HTML`,
`ASTText`, `Djot`, their options and the hook), and
`github.com/devselfio/gjot/ast` declares the tree they produce and read (the
node kinds, `Walk`, `WalkStatus` and `UnmarshalNode`). A program that only
reads or decodes trees needs only `ast`.

The tree is shaped as `go/ast`'s: `ast.Node` is an interface, and each kind of
node is its own struct (`*ast.Para`, `*ast.Str`, `*ast.Link`, ... the 49
kinds `Parse` produces, named as djot.js's tags are: `bullet_list` is
`BulletList`) carrying only the fields that kind has. Every kind embeds an
`ast.Header` with the node's `ID`, `Attrs` and `Pos`, so `n.Attr`,
`n.HasClass` and `n.Tag()` (the djot.js tag, as `ASTText` and the JSON print
it) work on any `Node`. The root `Parse` returns is an `*ast.Section` of
`Level` 0. Containers hold `Children []ast.Node`; `ast.Walk` visits a node,
then its children in order, the footnote definitions after the body. A hook
is called with an `*ast.Div` or an `*ast.Span`.

In package `gjot`:

| Function | Description |
| --- | --- |
| `Parse(src []byte, opts ...ParseOption) ast.Node` | Parse Djot into an AST, whose root is an `*ast.Section` of level 0. Never fails: every input is a document. Blocks nest at most `MaxNesting` (1,000) containers deep and a leaf's inlines at most `MaxNesting` deep; a marker that would nest deeper is text |
| `ParseErr(src []byte, opts ...ParseOption) (ast.Node, error)` | `Parse` that also returns `ErrTooDeep` when the limit stopped nesting; the tree is the same |
| `WithSourcePos() ParseOption` | Record each node's source range in its `Header.Pos` |
| `HTML(doc ast.Node, opts ...HTMLOption) ([]byte, error)` | Render HTML, byte-identical to djot.js. The error comes only from a hook |
| `WithHook(class string, h Hook) HTMLOption` | Render divs and spans of that class with `h func(w io.Writer, n ast.Node, entering bool) (ast.WalkStatus, error)`; `ast.WalkSkipChildren` drops the default child rendering |
| `ASTText(doc ast.Node) []byte` | Render the AST in djot.js's indented text format |
| `Djot(doc ast.Node) []byte` | Write the AST back as Djot. Re-parses to the same HTML and AST; reference links keep their labels with the definitions collected at the end, footnote definitions keep their labels |

In package `ast`:

| Function | Description |
| --- | --- |
| `Walk(n Node, visit func(Node) WalkStatus)` | Visit `n` and its descendants in document order; `WalkSkipChildren` skips a node's children |
| `UnmarshalNode(data []byte) (Node, error)` | Decode the JSON form of a node of any kind, switching on `"tag"` |
| `HasClass(string) bool`, `Attr(string) string`, `Tag() Tag` | On every node (`HasClass` and `Attr` through its `Header`): class and attribute lookup, and the djot.js tag name |
| `HeaderOf(n Node) *Header` | The `Header` `n` embeds, itself and not a copy |
| `Children(n Node) []Node` | `n`'s children in order, the node's own slice; nil for a leaf |
| `(*Header).WrittenID() (id string, written bool)` | The id written on the node; `written` tells `id=""` from no id. `ID` is the id the document gave it, written or generated |
| `(*Header).ClassList() []string` | The classes the node renders with, in written order |

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

Every kind marshals itself: `json.Marshal(doc)` writes one object per node,
its kind in `"tag"` and only the fields it has, byte for byte the JSON that
gjot wrote before the tree had a type per kind (the field names and the
`"tag"` discriminator are unchanged, so documents marshalled then decode
now). `ast.UnmarshalNode(data)` decodes a node of any kind, switching on
`"tag"`; `json.Unmarshal` into a concrete kind (`var sec ast.Section` for a
whole document) works too, and fails if the JSON is of another kind. The
JSON round-trips to the same AST text and HTML. A smart quote keeps the
quote it was written as in `"source"` (`Str.Source`, empty on every other
text node), which alt text, heading ids and the writer read. An ordered list
keeps djot.js's style in `"style"` (`"1."`, `"(a)"`, `"I)"`) and its first
marker's text in `"text"` (`OrderedList.Style` and `OrderedList.Marker`),
from which the HTML `start` is read. Where a Go field's name differs from
its wire key, the wire key stays: `Footnote.Label`, `FootnoteReference.Label`
and `Symb.Alias` are `"text"`, and `RawBlock.Format` and `RawInline.Format`
are `"lang"`. Source positions are kept only when parsed with
`WithSourcePos`. Blocks carry them at any nesting depth, as do the inline
nodes inside them. A definition list item's term and definition carry none,
as zjot's AST prints none. Footnote definitions stand after the document's
blocks as `footnote` nodes with their label in `"text"`; the HTML renderer
numbers the endnotes from them.

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
lists the targets. `make bench` (see Benchmarks) is not a CI gate.

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
`TestTreeJSONIdentical` pins the wire form to the JSON gjot wrote before the
tree had a type per kind, and checks that every case's JSON decodes to a tree
that encodes to the same bytes.
`TestWriterRoundTripWithSourcePos` checks that a tree parsed with source
positions is written byte for byte as one parsed without them, and reads back
to the source's HTML, over every case and the bench page.

The corpus, writer, JSON, hook and nesting tests sit at the root and go
through the public entry points. `layout_test.go` holds the root to one
non-test Go file, `gjot.go`, and pins the exported names of the root and of
`ast`; `api_test.go` uses both packages as an outside program would. The
tree's own tests (kinds, sizes, accessors, `Walk`, JSON encoding and
decoding) are in `ast`, and the parser's unit tests are in `internal/djot`.

`make spec-drift` (or `go run ./cmd/driftcheck`) checks that corpus against the
tag pinned in `ORIGIN.md`, which must be a `vN.N.N` tag, never a branch.
Offline it compares the files with their checked-in record,
`testdata/spec/SHA256SUMS`; when the network answers it also fetches zjot at
the tag and compares with that. It exits 1 naming each file added, removed or
changed, and also when `ORIGIN.md` pins a tag that `SHA256SUMS` does not
record, so bumping the pin without re-recording the corpus is red offline and
online. `go run ./cmd/driftcheck -record` writes `SHA256SUMS` after an online
match with zjot at the pinned tag; `go run ./cmd/driftcheck REF` measures the
gap to another ref without moving the pin. CI runs `make spec-drift` as the
last step of the `check` job, so a corpus that falls behind zjot at the pinned
tag turns CI red; bumping `Ref:` without re-recording the corpus is the one way
to turn that step red on purpose. `TestWorkflowRunsDriftcheck` fails if the
step leaves the workflow or names a target the Makefile does not define.
`go test ./cmd/driftcheck` covers all
of it against local test servers, and its `TestRecordRealCorpus` runs the
offline check on the real corpus.

## Benchmarks

```
make bench
# or, without make:
go test -run '^$' -bench . -benchmem ./...
```

`BenchmarkCorpus` pushes the whole corpus (the 1066 cases `TestSpec` reads,
8,025 nodes) through five paths; `BenchmarkPage` pushes one large document,
`testdata/bench/page.dj` (52,021 bytes, 6,160 nodes), through the same five.
An op is the whole corpus or the one page. Both are in the root package;
`make bench` also runs `internal/djot`'s `BenchmarkInlineOpenerStack`, which
times the inline opener stack at three sizes. `Parse` times parsing; `HTML`,
`ASTTextPos` (`ASTText` over a tree parsed `WithSourcePos`), `Djot` and `JSON`
(`json.Marshal` then `json.Unmarshal`) time only that step, over trees parsed
before the timer. Beside Go's `ns/op`, `B/op` and `allocs/op`, each line reports
`ns/case` (the op divided by its case count), `allocs/node` and `B/node` (the
op's heap allocations divided by the node count from a walk of the tree) and
`nodes` (that count).

The page is generated by `generateBenchPage` in `bench_test.go`: numbered
sections, each with a heading carrying attributes, a paragraph of emphasis,
strong, verbatim, links, a reference, a span, a footnote and smart quotes,
a nested bullet list, an ordered list, a block quote, a table with a caption
and alignments, a fenced code block, a footnote and a reference definition. The
file is committed and the benchmark reads it; `TestBenchPageCurrent` fails
unless it is the generator's output byte for byte, and
`go test -run TestBenchPageCurrent -write-bench-page .` rewrites it.

`go test -v -run TestNodeSize ./ast` prints the size of every node kind and,
on a 64-bit platform, fails if a common kind (`Str`, `Para`, `Emph`, `Strong`,
`Link`, `Image`, `Heading`, `Section`, `ListItem`, `BulletList`, `Span`,
`Verbatim`) is 100 bytes or more. Today the kinds are 48 to 112 bytes: `Str`
80, `Para` and `Emph` 72, `Link` 96, `OrderedList` 112 (the one kind over
100). `Str` was 88 bytes until its parse-time offset moved into the inline
parser (swarm-gridapp#17). The single `Node` struct they replaced was 272
bytes for every node.

With a type per kind, from one `make bench` run at commit
22c973a07e698b4471d8fb249aef54ddf378caf0 (the per-kind tree, swarm-gridapp#8),
same machine as below:

Corpus (an op is 1066 cases, 8,025 nodes):

| Path       |      ns/op | ns/case | allocs/node | B/node |
|------------|-----------:|--------:|------------:|-------:|
| Parse      | 11,061,356 |  10,377 |       5.799 |  576.4 |
| HTML       |  1,915,792 |   1,797 |      0.8273 |  52.33 |
| ASTTextPos |  2,599,515 |   2,439 |      0.7767 |  84.63 |
| Djot       |  7,860,847 |   7,374 |       5.659 |  175.3 |
| JSON       | 33,372,764 |  31,307 |       6.272 |  658.4 |

Page (an op is 1 case, 6,160 nodes):

| Path       |      ns/op |    ns/case | allocs/node | B/node |
|------------|-----------:|-----------:|------------:|-------:|
| Parse      |  6,567,603 |  6,567,603 |       3.181 |  506.2 |
| HTML       |  1,306,543 |  1,306,543 |     0.04383 |  87.00 |
| ASTTextPos |  2,989,972 |  2,989,972 |    0.004714 |  321.6 |
| Djot       | 17,231,027 | 17,231,027 |       6.424 |  280.2 |
| JSON       | 23,329,655 | 23,329,655 |       4.038 |  870.2 |

Against the single-struct tree (50a3232), `benchstat` over ten interleaved
`-count 1` runs of each on that machine found no `sec/op` change at p<0.05 on
any corpus path or on the page's `Parse`, `HTML`, `ASTTextPos` or `JSON`, with
every sample varying ±30 to 50%, and `Page/Djot` +51% (p=0.043; allocations
+0.3%, so likely noise, unconfirmed). Parsing allocates 22% fewer bytes
(corpus 5.66 to 4.41 MiB, page 3.87 to 2.97 MiB). `JSON` allocates 71%
(corpus) and 128% (page) more bytes, and on the page measured alone is slower:
`json.Marshal` 11.2 ms against 7.1 ms, because `encoding/json` re-scans and
compacts the bytes a `MarshalJSON` returns (the encoder itself takes 2 ms),
and decoding 24.5 ms against 21.0 ms, through an intermediate tree.

#7's numbers, for the single `Node` struct:

Corpus (an op is 1066 cases, 8,025 nodes):

| Path       |      ns/op | ns/case | allocs/node | B/node |
|------------|-----------:|--------:|------------:|-------:|
| Parse      | 15,064,292 |  14,132 |       5.813 |  769.4 |
| HTML       |  1,518,544 |   1,425 |      0.8273 |  52.33 |
| ASTTextPos |  2,356,054 |   2,210 |      0.7767 |  84.63 |
| Djot       | 11,263,231 |  10,566 |       5.626 |  176.7 |
| JSON       | 41,341,341 |  38,782 |       4.675 |  417.7 |

Page (an op is 1 case, 6,160 nodes):

| Path       |      ns/op |    ns/case | allocs/node | B/node |
|------------|-----------:|-----------:|------------:|-------:|
| Parse      | 12,771,469 | 12,771,469 |       3.181 |  678.0 |
| HTML       |  1,679,116 |  1,679,116 |     0.04383 |  87.00 |
| ASTTextPos |  3,970,511 |  3,970,511 |    0.004713 |  321.6 |
| Djot       | 18,147,377 | 18,147,377 |       6.407 |  277.1 |
| JSON       | 27,950,287 | 27,950,287 |       3.533 |  409.8 |

Measured at commit 8e9da12d43b630f277e66f222eed3131b9a7d9dd with
`go version go1.26.5 linux/amd64` (the module says `go 1.26`) on the Daisy swarm
worker's runner, a shared container (x86_64, Intel Xeon E5-2670 @ 2.60GHz,
16 CPUs), from one `make bench` run. These are that machine's numbers, not a
contract; runs on it vary by tens of percent, and the maintainer may replace
them.

A change that should not slow gjot down is compared with `benchstat`, not with
this table: `go test -run '^$' -bench . -benchmem -count 10 ./... > old.txt` on
`main`, the same into `new.txt` on the branch, then `benchstat old.txt new.txt`.

## Architecture

gjot's block parser is a container stack, in the shape of zjot v2.0.0's
`Parser.zig` with the Zig allocator plumbing and the streaming removed. Each
line is run against the open containers from the outermost in; while the
innermost container it continues holds blocks, the line may open new ones; what
remains is lazy text for an open paragraph, a blank line, or text for the
innermost container. A container closes when a line neither continues it nor is
lazy text for it. One pass through the input builds the whole `ast.Node` tree. Ids
are registered where they stand; references, footnotes and sections are resolved
after the pass.

The root holds one file, `gjot.go`, as gridapp's ADR 0018 rules; the tree is
the public package `ast`, and the implementation is `internal/djot`, which
imports `ast` and never the root. `ast` imports only the standard library.

```
gjot.go                     Public API: Parse, ParseErr, MaxNesting, ErrTooDeep, WithSourcePos, HTML, WithHook, Hook, ASTText, Djot
ast/node.go                 AST types: the Node interface, Header, one struct per kind, Tag, Attr, SourcePos, Range, HeaderOf
ast/walk.go                 Walk, WalkStatus, and each kind's children (Children)
ast/json.go                 The JSON form: per-kind MarshalJSON, UnmarshalNode
internal/djot/djot.go       The parser entry the root calls, and the nesting limit
internal/djot/parser.go     Block parsing: the container stack over lines
internal/djot/leaf.go       What one line says (fences, markers, table rows, ...)
internal/djot/document.go   Definitions, ids, references, sections, footnotes
internal/djot/inline.go     Inline parsing (emphasis, links, smart quotes, ...)
internal/djot/attributes.go Attribute parser ({#id .class key=value})
internal/djot/linemap.go    Source position mapping for joined inline text
internal/djot/html.go       HTML renderer and the class hook
internal/djot/asttext.go    AST text renderer
internal/djot/writer.go     Djot writer
spec_test.go                Corpus runner over testdata/spec/*.test
bench_test.go               Benchmarks over the corpus and testdata/bench/page.dj; the page generator
layout_test.go              The root's one file and the exported names of the root and ast
testdata/bench/page.dj      The benchmark page (generated, committed)
cmd/gjot                    CLI
cmd/driftcheck              Corpus drift check against zjot
```

## Changes

- 2026-11-09. Removed from the public API: the `Tag` constants
  `ReferenceDefinition`, `Escape`, `LeftSingleQuote`, `RightSingleQuote`,
  `LeftDoubleQuote` and `RightDoubleQuote`, which `Parse` never produced and no
  renderer needed; and the `Node.Classes` field (JSON `classes`), which `Parse`
  never set. A node's classes are its `class` entry in `Attrs`, read by
  `HasClass`. `json.Unmarshal` now ignores a `classes` key in a `Node`
  document; gjot never wrote one.
- 2026-11-10. The AST has a Go type per node kind, as `go/ast` does
  (swarm-gridapp#8). `Node` is now an interface, with an unexported method,
  implemented by one struct per kind `Parse` produces: `Section`, `Heading`,
  `Para`, `ThematicBreak`, `CodeBlock`, `RawBlock`, `BlockQuote`, `Div`,
  `BulletList`, `TaskList`, `OrderedList`, `DefinitionList`, `ListItem`,
  `TaskListItem`, `DefinitionListItem`, `Term`, `Definition`, `Table`,
  `Caption`, `Row`, `Cell`, `Footnote`, `Str`, `SoftBreak`, `HardBreak`,
  `NonBreakingSpace`, `Emph`, `Strong`, `Span`, `Mark`, `Superscript`,
  `Subscript`, `Insert`, `Delete`, `DoubleQuoted`, `SingleQuoted`,
  `Verbatim`, `InlineMath`, `DisplayMath`, `RawInline`, `URL`, `Email`,
  `FootnoteReference`, `Symb`, `Link`, `Image`, `Ellipsis`, `EmDash`,
  `EnDash`. Each embeds a `Header` (`ID`, `Attrs`, `Pos`) and carries only
  its own fields; `HasClass` and `Attr` are on `Header`, so on every node.
  - Removed: the `*Node` struct and its fields, and the 49 `Tag` constants,
    whose names the kinds now take. `n.Tag` (a field) is `n.Tag()` (a method
    returning the same djot.js name); the `Tag` type stays.
  - Moved: `Children` is on the kinds that have children, as `[]Node`;
    `Level` on `Heading` and `Section`; `AutoID` on `Heading` and `Section`
    only; `Text` on `Str`, `CodeBlock`, `RawBlock`, `Verbatim`,
    `InlineMath`, `DisplayMath`, `RawInline`, `URL` and `Email`; `Lang` on
    `CodeBlock`; `Style` and `Tight` on the lists; `Head` on `Row` and
    `Cell`, `Align` on `Cell`; `Destination` and `Reference` on `Link` and
    `Image`; `Source` on `Str`.
  - Renamed: an ordered list's first marker `Text` is `OrderedList.Marker`; a
    footnote's and a footnote reference's label `Text` is `Label`; a
    symbol's `Text` is `Symb.Alias`; a raw block's or raw inline's `Lang` is
    `Format`. `TaskListItem.Checked` is a `bool` (it was a `*bool` the
    parser always set). A link's `Text`, which `Parse` never set and only
    `HTML` read for a link without children, is gone.
  - Signatures: `Parse` and `ParseErr` return `Node` (a `*Section`);
    `HTML`, `ASTText` and `Djot` take `Node`; a `Hook` is
    `func(w io.Writer, n Node, entering bool) (WalkStatus, error)` and is
    called with a `*Div` or `*Span`.
  - Added: `Walk(n Node, visit func(Node) WalkStatus)`, and
    `UnmarshalNode(data []byte) (Node, error)` to decode a node of any kind
    (`json.Unmarshal` cannot fill an interface).
  - Unchanged: the JSON form, byte for byte, its field names and its
    `"tag"` discriminator included, though `json.Marshal(doc)` is slower (see
    Benchmarks); `json.Unmarshal` into a `*Section`
    decodes a document. A hook or a consumer that read `n.Tag` and fields
    now type-switches: `switch n := n.(type) { case *gjot.Heading: ... }`
    (see Library usage).
- 2026-11-10. Breaking change: the module and import path is now
  `github.com/devselfio/gjot`. Update existing import statements and install
  commands before upgrading.
- 2026-11-11. Breaking change: the node types moved to the package
  `github.com/devselfio/gjot/ast` (swarm-gridapp#17, gridapp ADR 0018), as
  `go/ast` sits beside `go/parser`. The root package keeps one file,
  `gjot.go`, with the entry points; the parser and renderers are in
  `internal/djot`, which no program can import. Behaviour, output and the
  JSON form are unchanged.
  - Moved to `ast`, under the same names: `Node`, `Header`, `Tag`, `Attr`,
    `SourcePos`, `Range`, `CellAlign` and its four `Align*` constants, the
    49 node kinds with their methods (`Tag`, `MarshalJSON`, `UnmarshalJSON`,
    and `HasClass` and `Attr` on `Header`), `Walk`, `WalkStatus`,
    `WalkContinue`, `WalkSkipChildren` and `UnmarshalNode`. Write
    `ast.Node`, `*ast.Heading`, `ast.Walk`, `ast.WalkSkipChildren`,
    `ast.UnmarshalNode` where you wrote `gjot.`; the root keeps no aliases
    for them, so that each name has one home.
  - Unchanged in `gjot`: `Parse`, `ParseErr`, `HTML`, `ASTText`, `Djot`,
    `ParseOption`, `WithSourcePos`, `HTMLOption`, `Hook`, `WithHook`,
    `MaxNesting` and `ErrTooDeep`; their signatures now name `ast.Node` and
    `ast.WalkStatus` (`Hook` is
    `func(w io.Writer, n ast.Node, entering bool) (ast.WalkStatus, error)`).
  - Added to `ast`, because the renderers left the package that declares the
    tree and still need what they read before through unexported names:
    `HeaderOf(n Node) *Header` (the node's own header), `Children(n Node)
    []Node` (its child slice, nil for a leaf), `(*Header).WrittenID()` (the
    written id and whether one was written, `id=""` included) and
    `(*Header).ClassList()` (its classes in written order). `Node` keeps its
    unexported method, so only `ast`'s kinds implement it.
  - `Str` is 8 bytes smaller: the parse-time source offset it carried in an
    unexported field is now kept by the inline parser while it runs.
