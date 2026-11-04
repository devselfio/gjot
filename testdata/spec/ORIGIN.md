# Corpus origin

The `*.test` files in this directory are the djot.js test suite as vendored
by zjot, copied byte for byte from the `test/` directory of the repository
below at the ref below. They are never edited here: a failing case is a
parser bug, not a fixture bug.

Repository: https://github.com/quiteclose/zjot
Ref: ae01cd4

The drift check fetches that ref and exits non-zero when any `*.test` file
here was added, removed or changed upstream. Run it in CI and whenever
either corpus changes:

    go run ./cmd/driftcheck

To measure the gap to another ref without moving the pin, name the ref;
nothing is written:

    go run ./cmd/driftcheck v2.0.0

The pin is a commit, not a tag, only until the re-sync to zjot 2.0.0 moves it
to `v2.0.0` together with the files (constraints: corpora are pinned to tags
except briefly during cross-repo work).

`filters.test` needs a Lua filter and its cases are skipped by the runner.
