# gjot — agent instructions

gjot is a Djot library for Go: a parser, an HTML renderer with a div/span
hook, a Djot writer and a JSON-serialisable AST. It is a port of zjot.
Module `github.com/devselfio/gjot`; consumers depend on tagged versions
(`vN.N.N`), never a branch. `README.md` documents the public API; keep it
true when the API changes, and record breaking changes under its Changes.

## The contract

- The specification is zjot v2.0.0's corpus, vendored byte for byte under
  `testdata/spec/` and pinned in `testdata/spec/ORIGIN.md`. Never edit a
  `.test` file: a failing case is a parser bug, not a fixture bug.
- Every case must pass, and pass again after a `Djot` write-back and
  re-parse. Only the 3 cases in `filters.test` skip (they need a Lua
  filter); add no other skips.
- HTML stays byte-identical to djot.js; `ASTText` matches djot.js's format.
  The JSON wire form (field names, `"tag"` discriminator) is pinned by
  `TestTreeJSONIdentical`; do not change it without a decision to.
- `Parse` never fails. `HTML`'s only error comes from a hook. Nesting stops
  at `MaxNesting`; `ParseErr` reports `ErrTooDeep`.
- Moving the pin: see `ORIGIN.md` (`go run ./cmd/driftcheck -record`).

## Commands

Use the toolchain `go.mod` declares (`go 1.26`). Tests are verified under
`GOTOOLCHAIN=go1.26.5`; under Go 1.27.1 `TestJSONStringEscaping` (in `ast`)
fails, so pin the toolchain if your default Go is newer.

- `make help` — list targets
- `make build` — `go build ./...` (writes no binary)
- `make test` — `go test ./...`: unit tests and the corpus
- `make check` — `gofmt -l`, `go vet`, `go mod tidy -diff`; rewrites nothing
- `make spec-count` — the corpus line, today
  `corpus: 1063 of 1066 cases pass, 3 skipped (lua filter)`
- `make spec-drift` — corpus against `SHA256SUMS`; also against zjot at the
  pinned tag when the network answers
- `make bench` — benchmarks; not a CI gate
- `make tidy` — `go mod tidy`; refuses to run on a dirty tree, by design
- One case: `go test -run '^TestSpec/attributes/L4$' -v .`
- CLI: `echo 'Hi *x*' | go run ./cmd/gjot` (`--ast`, `--sourcepos`,
  `--json`, `--djot`; `-h` for help)

CI (`.github/workflows/ci.yml`, Go from `go-version-file: go.mod`) runs
`make check`, `build`, `test`, `spec-count`, `spec-drift` on pushes to
`main` and pull requests. Run `make check && make test` before committing.

## Layout

The root package holds one non-test file, `gjot.go`: the entry points
(`Parse`, `ParseErr`, `HTML`, `WithHook`, `ASTText`, `Djot`, options).
Everything else lives in subpackages (gridapp's ADR 0018):

- `ast/` — public node types, one struct per kind behind `ast.Node`, plus
  `Walk` and the JSON form. Imports only the standard library.
- `internal/djot/` — parser and renderers; imports `ast`, never the root.
- `cmd/gjot` (CLI), `cmd/driftcheck` (corpus drift check).

`layout_test.go` enforces the one-file root and pins the exported names of
the root and `ast`; update it deliberately when the API changes.

## Code conventions

- Standard library only; `go.mod` has no requirements. A dependency needs a
  decision first, not a line in a diff.
- Tests sit beside the code they test; root tests go through the public
  API. Fixtures live in `testdata/`.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`); never swallow.
- Comments only for non-obvious constraints or trade-offs.

## Working mode

Smallest correct diff. Write a failing test first, then make it pass. No
dead code. Stop at the scope of the change.
