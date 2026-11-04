// Command driftcheck compares the vendored corpus under testdata/spec with
// zjot's test/ directory at a ref and exits non-zero when they differ.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const usage = `Usage: driftcheck [-dir DIR] [REF]

Compare DIR/*.test (default testdata/spec) with zjot's test/*.test at REF.
REF defaults to the "Ref:" line of DIR/ORIGIN.md; the repository comes from
its "Repository:" line. Exit 0 when the files match, 1 when they differ,
2 when the check cannot run.
`

// origin is the pin recorded in ORIGIN.md.
type origin struct {
	Repository string
	Ref        string
}

// parseOrigin reads the "Repository: <url>" and "Ref: <ref>" lines of
// ORIGIN.md. Each key must appear exactly once at the start of a line,
// followed by ": " and a non-empty value with no spaces.
func parseOrigin(content []byte) (origin, error) {
	vals := map[string]string{}
	count := map[string]int{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimRight(line, "\r")
		for _, key := range []string{"Repository", "Ref"} {
			if !strings.HasPrefix(line, key+":") {
				continue
			}
			count[key]++
			v := strings.TrimPrefix(line, key+": ")
			if !strings.HasPrefix(line, key+": ") || v == "" || strings.ContainsAny(v, " \t") {
				return origin{}, fmt.Errorf("ORIGIN.md: %q line must be %q followed by a value without spaces", key, key+": ")
			}
			vals[key] = v
		}
	}
	for _, key := range []string{"Repository", "Ref"} {
		switch count[key] {
		case 0:
			return origin{}, fmt.Errorf("ORIGIN.md: no %q line", key)
		case 1:
		default:
			return origin{}, fmt.Errorf("ORIGIN.md: %q line appears %d times", key, count[key])
		}
	}
	return origin{Repository: vals["Repository"], Ref: vals["Ref"]}, nil
}

// readLocal returns the regular files named *.test directly in dir.
func readLocal(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".test") || !e.Type().IsRegular() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out[e.Name()] = b
	}
	return out, nil
}

func archiveURL(repository, ref string) string {
	return repository + "/archive/" + ref + ".tar.gz"
}

// fetchUpstream downloads the archive of ref and returns its corpus.
func fetchUpstream(client *http.Client, repository, ref string) (map[string][]byte, error) {
	url := archiveURL(repository, ref)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("ref %s not found at %s", ref, url)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("fetching %s: %s", url, resp.Status)
	}
	return readCorpusTar(resp.Body)
}

// readCorpusTar reads a gzipped tar with one root directory and returns the
// regular files whose path is exactly <root>/test/<name>.test.
func readCorpusTar(r io.Reader) (map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("bad archive: %w", err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bad archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		parts := strings.Split(h.Name, "/")
		if len(parts) != 3 || parts[1] != "test" || !strings.HasSuffix(parts[2], ".test") {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("bad archive: %w", err)
		}
		out[parts[2]] = b
	}
	return out, nil
}

// change is one difference, seen from the vendored copy.
type change struct {
	Name string
	Kind string // "added" (upstream only), "removed" (local only), "changed"
}

// compare returns the differences sorted by Name.
func compare(local, upstream map[string][]byte) []change {
	var out []change
	for n, lb := range local {
		ub, ok := upstream[n]
		switch {
		case !ok:
			out = append(out, change{n, "removed"})
		case !bytes.Equal(lb, ub):
			out = append(out, change{n, "changed"})
		}
	}
	for n := range upstream {
		if _, ok := local[n]; !ok {
			out = append(out, change{n, "added"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func run(args []string, client *http.Client, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("driftcheck", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "testdata/spec", "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprintf(stderr, "driftcheck: %v\n%s", err, usage)
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "driftcheck: at most one REF\n%s", usage)
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "driftcheck: %v\n", err)
		return 2
	}
	content, err := os.ReadFile(filepath.Join(*dir, "ORIGIN.md"))
	if err != nil {
		return fail(err)
	}
	o, err := parseOrigin(content)
	if err != nil {
		return fail(err)
	}
	if fs.NArg() == 1 {
		o.Ref = fs.Arg(0)
	}
	local, err := readLocal(*dir)
	if err != nil {
		return fail(err)
	}
	if len(local) == 0 {
		return fail(fmt.Errorf("no *.test files in %s", *dir))
	}
	upstream, err := fetchUpstream(client, o.Repository, o.Ref)
	if err != nil {
		return fail(err)
	}
	if len(upstream) == 0 {
		return fail(fmt.Errorf("no *.test files found in test/ at %s of %s", o.Ref, o.Repository))
	}
	changes := compare(local, upstream)
	if len(changes) == 0 {
		fmt.Fprintf(stdout, "driftcheck: %s matches zjot at %s (%d files)\n", *dir, o.Ref, len(local))
		return 0
	}
	for _, c := range changes {
		fmt.Fprintf(stdout, "%s\t%s\n", c.Kind, c.Name)
	}
	fmt.Fprintf(stdout, "driftcheck: %d files differ from zjot at %s (upstream view: added is upstream-only, removed is local-only)\n", len(changes), o.Ref)
	return 1
}

func main() {
	os.Exit(run(os.Args[1:], &http.Client{Timeout: 2 * time.Minute}, os.Stdout, os.Stderr))
}
