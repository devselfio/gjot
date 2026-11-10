package gjot

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unsafe"
)

var writeBenchPage = flag.Bool("write-bench-page", false, "regenerate testdata/bench/page.dj from the generator and exit")

// benchPagePath is the large real document BenchmarkPage reads. It is made by
// generateBenchPage and committed; the benchmark never regenerates it.
const benchPagePath = "testdata/bench/page.dj"

// corpusInputs returns the input of every case TestSpec runs, the 3 Lua
// filter cases included: their inputs are ordinary Djot.
func corpusInputs(tb testing.TB) [][]byte {
	tb.Helper()
	files, err := filepath.Glob("testdata/spec/*.test")
	if err != nil {
		tb.Fatal(err)
	}
	if len(files) == 0 {
		tb.Fatal("no testdata/spec/*.test files")
	}
	sort.Strings(files)
	var inputs [][]byte
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			tb.Fatal(err)
		}
		for _, c := range parseSpecFile(string(content)) {
			inputs = append(inputs, []byte(c.input))
		}
	}
	return inputs
}

func readBenchPage(tb testing.TB) []byte {
	tb.Helper()
	page, err := os.ReadFile(benchPagePath)
	if err != nil {
		tb.Fatal(err)
	}
	return page
}

// countNodes is the size of the tree under n, n included.
func countNodes(n *Node) int {
	count := 1
	for _, c := range n.Children {
		count += countNodes(c)
	}
	return count
}

// parseAll parses every input and returns the trees and their node count.
func parseAll(inputs [][]byte, opts ...ParseOption) ([]*Node, int) {
	trees := make([]*Node, len(inputs))
	nodes := 0
	for i, in := range inputs {
		trees[i] = Parse(in, opts...)
		nodes += countNodes(trees[i])
	}
	return trees, nodes
}

// measure runs op under b.Loop and reports, beside the standard columns,
// ns per case and heap allocations and bytes per node. The allocation figures
// are a runtime.ReadMemStats delta around the loop, so they count every heap
// allocation in the process meanwhile; nothing else runs during a benchmark
// here, but a test that leaves goroutines allocating would skew them.
func measure(b *testing.B, cases, nodes int, op func()) {
	b.ReportAllocs()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for b.Loop() {
		op()
	}
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(b.Elapsed())/float64(b.N*cases), "ns/case")
	b.ReportMetric(float64(after.Mallocs-before.Mallocs)/float64(b.N*nodes), "allocs/node")
	b.ReportMetric(float64(after.TotalAlloc-before.TotalAlloc)/float64(b.N*nodes), "B/node")
	b.ReportMetric(float64(nodes), "nodes")
}

// benchPaths pushes inputs through the five paths. Parse times parsing; the
// other four time only their step, over trees parsed before the timer.
func benchPaths(b *testing.B, inputs [][]byte) {
	b.Run("Parse", func(b *testing.B) {
		_, nodes := parseAll(inputs)
		measure(b, len(inputs), nodes, func() {
			for _, in := range inputs {
				Parse(in)
			}
		})
	})
	b.Run("HTML", func(b *testing.B) {
		trees, nodes := parseAll(inputs)
		measure(b, len(inputs), nodes, func() {
			for _, t := range trees {
				if _, err := HTML(t); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
	b.Run("ASTTextPos", func(b *testing.B) {
		trees, nodes := parseAll(inputs, WithSourcePos())
		measure(b, len(inputs), nodes, func() {
			for _, t := range trees {
				ASTText(t)
			}
		})
	})
	b.Run("Djot", func(b *testing.B) {
		trees, nodes := parseAll(inputs)
		measure(b, len(inputs), nodes, func() {
			for _, t := range trees {
				Djot(t)
			}
		})
	})
	b.Run("JSON", func(b *testing.B) {
		trees, nodes := parseAll(inputs)
		measure(b, len(inputs), nodes, func() {
			for _, t := range trees {
				data, err := json.Marshal(t)
				if err != nil {
					b.Fatal(err)
				}
				if err := json.Unmarshal(data, &Node{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

// BenchmarkCorpus pushes TestSpec's 1066 cases through the five paths; an op
// is the whole corpus.
func BenchmarkCorpus(b *testing.B) {
	benchPaths(b, corpusInputs(b))
}

// BenchmarkPage pushes testdata/bench/page.dj through the five paths; an op
// is the one page.
func BenchmarkPage(b *testing.B) {
	benchPaths(b, [][]byte{readBenchPage(b)})
}

var benchWords = []string{
	"grid", "cell", "lattice", "harbour", "ledger", "copper", "meadow", "signal",
	"orbit", "window", "thread", "basalt", "river", "lantern", "margin", "quartz",
	"archive", "compass", "fern", "tide", "marble", "kernel", "willow", "beacon",
	"canvas", "ember", "falcon", "garnet", "hollow", "island", "juniper", "kettle",
}

// generateBenchPage is the bench page: numbered sections, each mixing the
// block and inline kinds a real page has, appended until the page passes
// 50 KB. It uses no randomness and no clock, so its output is the same on
// every run and platform.
func generateBenchPage() []byte {
	var b bytes.Buffer
	w := 0
	word := func() string {
		s := benchWords[w%len(benchWords)]
		w++
		return s
	}
	words := func(n int) string {
		var s bytes.Buffer
		for j := 0; j < n; j++ {
			if j > 0 {
				s.WriteByte(' ')
			}
			s.WriteString(word())
		}
		return s.String()
	}

	b.WriteString("{% Generated by `go test -run TestBenchPageCurrent -write-bench-page .`;\n")
	b.WriteString("   BenchmarkPage reads this file and never regenerates it. %}\n")
	b.WriteString("# Benchmark page\n\n")
	for i := 1; b.Len() < 50*1024; i++ {
		fmt.Fprintf(&b, "{#sec-%d .topic}\n## Section %d: %s\n\n", i, i, words(3))
		fmt.Fprintf(&b, "The %s is _%s_ and *%s*, with `%s %d` inline,\n", word(), words(2), words(2), word(), i)
		fmt.Fprintf(&b, "a link to [%s](https://example.org/%d), a reference to [%s][ref-%d],\n", words(2), i, word(), i)
		fmt.Fprintf(&b, "a [%s]{.kw} span, a footnote[^n-%d] and the %s's \"%s\" %s.\n\n", word(), i, word(), words(2), words(4))
		fmt.Fprintf(&b, "- %s\n\n  - %s\n  - %s _%s_\n- %s\n- %s `%s`\n\n", words(3), words(2), words(2), word(), words(4), words(2), word())
		fmt.Fprintf(&b, "1. %s\n2. %s *%s*\n3. %s\n\n", words(3), words(2), word(), words(5))
		fmt.Fprintf(&b, "> %s _%s_ %s\n> %s.\n\n", words(4), word(), words(3), words(5))
		fmt.Fprintf(&b, "| %s | %s | %s |\n|:--|:-:|--:|\n", word(), word(), word())
		for r := 0; r < 3; r++ {
			fmt.Fprintf(&b, "| %s | *%s* | %d |\n", word(), word(), i*10+r)
		}
		fmt.Fprintf(&b, "\n^ Table %d: %s\n\n", i, words(3))
		fmt.Fprintf(&b, "``` go\nfunc %s%d() string {\n\treturn %q\n}\n```\n\n", word(), i, words(3))
		fmt.Fprintf(&b, "[^n-%d]: %s _%s_ %s.\n\n", i, words(5), word(), words(3))
		fmt.Fprintf(&b, "[ref-%d]: https://example.org/ref/%d\n\n", i, i)
	}
	return b.Bytes()
}

// The committed page is the generator's output byte for byte, so it is
// reproducible and never hand-edited. With -write-bench-page it is rewritten.
func TestBenchPageCurrent(t *testing.T) {
	want := generateBenchPage()
	if *writeBenchPage {
		if err := os.MkdirAll(filepath.Dir(benchPagePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(benchPagePath, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got := readBenchPage(t); !bytes.Equal(got, want) {
		t.Fatalf("%s is not the generator's output; run go test -run TestBenchPageCurrent -write-bench-page .", benchPagePath)
	}
}

// The page is about 50 KB and holds every kind it is meant to exercise.
func TestBenchPageMix(t *testing.T) {
	page := readBenchPage(t)
	if n := len(page); n < 45*1024 || n > 60*1024 {
		t.Fatalf("page is %d bytes, want 45 to 60 KB", n)
	}
	doc, err := ParseErr(page)
	if err != nil {
		t.Fatalf("ParseErr: %v", err)
	}
	seen := map[Tag]bool{}
	attrs := false
	var walk func(n *Node)
	walk = func(n *Node) {
		seen[n.Tag] = true
		if len(n.Attrs) > 0 {
			attrs = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	for _, tag := range []Tag{Heading, Para, BulletList, OrderedList, BlockQuote, Table, CodeBlock, Footnote, FootnoteReference, Link, Emph, Strong, Verbatim, Span} {
		if !seen[tag] {
			t.Errorf("page has no %s node", tag)
		}
	}
	if !attrs {
		t.Error("page has no node with attributes")
	}
}

// TestNodeSize shows the old node's size and each kind's of the new tree,
// and on a 64-bit platform fails if a common kind is 100 bytes or more.
func TestNodeSize(t *testing.T) {
	t.Logf("Node is %d bytes (unsafe.Sizeof)", unsafe.Sizeof(Node{}))
	sizes := []struct {
		kind treeNode
		size uintptr
	}{
		{&treeSection{}, unsafe.Sizeof(treeSection{})},
		{&treeHeading{}, unsafe.Sizeof(treeHeading{})},
		{&treePara{}, unsafe.Sizeof(treePara{})},
		{&treeThematicBreak{}, unsafe.Sizeof(treeThematicBreak{})},
		{&treeCodeBlock{}, unsafe.Sizeof(treeCodeBlock{})},
		{&treeRawBlock{}, unsafe.Sizeof(treeRawBlock{})},
		{&treeBlockQuote{}, unsafe.Sizeof(treeBlockQuote{})},
		{&treeDiv{}, unsafe.Sizeof(treeDiv{})},
		{&treeBulletList{}, unsafe.Sizeof(treeBulletList{})},
		{&treeTaskList{}, unsafe.Sizeof(treeTaskList{})},
		{&treeOrderedList{}, unsafe.Sizeof(treeOrderedList{})},
		{&treeDefinitionList{}, unsafe.Sizeof(treeDefinitionList{})},
		{&treeListItem{}, unsafe.Sizeof(treeListItem{})},
		{&treeTaskListItem{}, unsafe.Sizeof(treeTaskListItem{})},
		{&treeDefinitionListItem{}, unsafe.Sizeof(treeDefinitionListItem{})},
		{&treeTerm{}, unsafe.Sizeof(treeTerm{})},
		{&treeDefinition{}, unsafe.Sizeof(treeDefinition{})},
		{&treeTable{}, unsafe.Sizeof(treeTable{})},
		{&treeCaption{}, unsafe.Sizeof(treeCaption{})},
		{&treeRow{}, unsafe.Sizeof(treeRow{})},
		{&treeCell{}, unsafe.Sizeof(treeCell{})},
		{&treeFootnote{}, unsafe.Sizeof(treeFootnote{})},
		{&treeStr{}, unsafe.Sizeof(treeStr{})},
		{&treeSoftBreak{}, unsafe.Sizeof(treeSoftBreak{})},
		{&treeHardBreak{}, unsafe.Sizeof(treeHardBreak{})},
		{&treeNonBreakingSpace{}, unsafe.Sizeof(treeNonBreakingSpace{})},
		{&treeEmph{}, unsafe.Sizeof(treeEmph{})},
		{&treeStrong{}, unsafe.Sizeof(treeStrong{})},
		{&treeSpan{}, unsafe.Sizeof(treeSpan{})},
		{&treeMark{}, unsafe.Sizeof(treeMark{})},
		{&treeSuperscript{}, unsafe.Sizeof(treeSuperscript{})},
		{&treeSubscript{}, unsafe.Sizeof(treeSubscript{})},
		{&treeInsert{}, unsafe.Sizeof(treeInsert{})},
		{&treeDelete{}, unsafe.Sizeof(treeDelete{})},
		{&treeDoubleQuoted{}, unsafe.Sizeof(treeDoubleQuoted{})},
		{&treeSingleQuoted{}, unsafe.Sizeof(treeSingleQuoted{})},
		{&treeVerbatim{}, unsafe.Sizeof(treeVerbatim{})},
		{&treeInlineMath{}, unsafe.Sizeof(treeInlineMath{})},
		{&treeDisplayMath{}, unsafe.Sizeof(treeDisplayMath{})},
		{&treeRawInline{}, unsafe.Sizeof(treeRawInline{})},
		{&treeURL{}, unsafe.Sizeof(treeURL{})},
		{&treeEmail{}, unsafe.Sizeof(treeEmail{})},
		{&treeFootnoteReference{}, unsafe.Sizeof(treeFootnoteReference{})},
		{&treeSymb{}, unsafe.Sizeof(treeSymb{})},
		{&treeLink{}, unsafe.Sizeof(treeLink{})},
		{&treeImage{}, unsafe.Sizeof(treeImage{})},
		{&treeEllipsis{}, unsafe.Sizeof(treeEllipsis{})},
		{&treeEmDash{}, unsafe.Sizeof(treeEmDash{})},
		{&treeEnDash{}, unsafe.Sizeof(treeEnDash{})},
	}
	if len(sizes) != len(nodeKinds) {
		t.Errorf("the table lists %d kinds, nodeKinds %d", len(sizes), len(nodeKinds))
	}
	var table strings.Builder
	for _, s := range sizes {
		fmt.Fprintf(&table, "\n  %-22s %3d", s.kind.Tag(), s.size)
	}
	t.Logf("kind sizes in bytes (unsafe.Sizeof):%s", table.String())
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the 100-byte bound is for 64-bit platforms")
	}
	common := map[Tag]bool{"str": true, "para": true, "emph": true, "strong": true, "link": true, "image": true,
		"heading": true, "section": true, "list_item": true, "bullet_list": true, "span": true, "verbatim": true}
	for _, s := range sizes {
		if common[s.kind.Tag()] {
			delete(common, s.kind.Tag())
			if s.size >= 100 {
				t.Errorf("%s is %d bytes, want under 100", s.kind.Tag(), s.size)
			}
		}
	}
	for tag := range common {
		t.Errorf("common kind %s is not in the table", tag)
	}
}
