# Corpus origin

The `*.test` files in this directory are zjot's `test/*.test` at the tag
below: the 26 djot.js suite files zjot vendors plus its own `grid.test` and
`zjot.test`, 28 files, copied byte for byte from the repository below. They
are never edited here: a failing case is a parser bug, not a fixture bug.

Repository: https://github.com/devselfio/zjot
Ref: v2.0.0

`Ref:` is the pin, and it must be a tag of the form `vN.N.N`, never a branch
or a commit: the drift check refuses anything else. `SHA256SUMS` beside this
file records the 28 files at that tag: its first line, `# Ref: <tag>`, names
the tag it records, then one `sha256sum` line per file
(`cd testdata/spec && sha256sum -c SHA256SUMS` verifies it with coreutils).

The drift check checks the pin. Run it in CI and whenever either corpus
changes:

    make spec-drift
    # or, without make:
    go run ./cmd/driftcheck

Without a network it compares the files here with `SHA256SUMS` and exits 1
naming each file added, removed or changed against it. With the network it
also fetches zjot at the tag and exits 1 naming each file that differs
upstream. Either way, a `Ref:` that `SHA256SUMS` does not record is red:
bumping `Ref:` without re-recording the corpus fails offline and online.

To move the pin: copy zjot's `test/*.test` at the new tag over the files
here, set `Ref:` to the tag, and record the corpus, which works only online
and writes `SHA256SUMS` only if the files match zjot at that tag:

    go run ./cmd/driftcheck -record

To measure the gap to another ref without moving the pin, name the ref. Any
ref works, a branch included; this works only online, does not consult
`SHA256SUMS`, and writes nothing:

    go run ./cmd/driftcheck main

`filters.test` needs a Lua filter and its cases are skipped by the runner.
