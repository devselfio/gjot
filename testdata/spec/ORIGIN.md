# Corpus origin

The `*.test` files in this directory are zjot's `test/*.test` at the tag
below: the 26 djot.js suite files zjot vendors plus its own `grid.test` and
`zjot.test`, 28 files, copied byte for byte from the repository below. They
are never edited here: a failing case is a parser bug, not a fixture bug.

Repository: https://github.com/quiteclose/zjot
Ref: v2.0.0

The drift check fetches that ref and exits non-zero when any `*.test` file
here was added, removed or changed upstream. Run it in CI and whenever
either corpus changes:

    go run ./cmd/driftcheck

To measure the gap to another ref without moving the pin, name the ref;
nothing is written:

    go run ./cmd/driftcheck main

`known-failures.txt` beside the corpus lists the cases gjot does not yet
pass and is read by `TestSpec`; see the README.

`filters.test` needs a Lua filter and its cases are skipped by the runner.
