# Corpus origin

The `*.test` files in this directory are the djot.js test suite as vendored
by zjot (`~/workspaces/website.260223/zjot/test`, zjot commit `ae01cd4`,
2026-03-02), copied byte for byte. They are never edited here: a failing case
is a parser bug, not a fixture bug.

Parity check, rerun whenever either corpus changes (must report only this file):

    diff -r ~/workspaces/website.260223/zjot/test gjot/testdata/spec

`filters.test` needs a Lua filter and its cases are skipped by the runner.
