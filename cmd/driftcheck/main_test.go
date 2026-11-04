package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// buildTar makes a gzipped tar like GitHub's: a pax global header first,
// then entries in sorted order. Paths ending in "/" are directories.
func buildTar(t *testing.T, files map[string][]byte, order []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": "abc"}}); err != nil {
		t.Fatal(err)
	}
	for _, p := range order {
		if strings.HasSuffix(p, "/") {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: p, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		b := files[p]
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: p, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		tw.Write(b)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// corpusTar builds an archive with root/test/<name> for each corpus file.
func corpusTar(t *testing.T, root string, corpus map[string]string) []byte {
	files := map[string][]byte{}
	order := []string{root + "/", root + "/README.md", root + "/test/"}
	files[root+"/README.md"] = []byte("hi")
	for n, c := range corpus {
		files[root+"/test/"+n] = []byte(c)
		order = append(order, root+"/test/"+n)
	}
	return buildTar(t, files, order)
}

func serve(t *testing.T, refs map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/archive/", func(w http.ResponseWriter, r *http.Request) {
		ref := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/archive/"), ".tar.gz")
		b, ok := refs[ref]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func writeDir(t *testing.T, repo, ref string, corpus map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	origin := "# Corpus origin\n\nRepository: " + repo + "\nRef: " + ref + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ORIGIN.md"), []byte(origin), 0o644); err != nil {
		t.Fatal(err)
	}
	for n, c := range corpus {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			m[p] = string(b)
		}
		return nil
	})
	return m
}

func TestParseOriginRealFile(t *testing.T) {
	b, err := os.ReadFile("../../testdata/spec/ORIGIN.md")
	if err != nil {
		t.Fatal(err)
	}
	o, err := parseOrigin(b)
	if err != nil {
		t.Fatal(err)
	}
	if o.Repository != "https://github.com/quiteclose/zjot" || o.Ref != "ae01cd4" {
		t.Fatalf("got %+v", o)
	}
}

func TestParseOriginErrors(t *testing.T) {
	cases := []struct{ name, in, key string }{
		{"missing ref", "Repository: http://x\n", "Ref"},
		{"missing repo", "Ref: v1\n", "Repository"},
		{"duplicate", "Repository: http://x\nRef: a\nRef: b\n", "Ref"},
		{"empty value", "Repository: http://x\nRef: \n", "Ref"},
		{"space in value", "Repository: http://x\nRef: a b\n", "Ref"},
	}
	for _, c := range cases {
		_, err := parseOrigin([]byte(c.in))
		if err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s: err = %v, want mention of %s", c.name, err, c.key)
		}
	}
}

func TestReadCorpusTar(t *testing.T) {
	for _, root := range []string{"zjot-2.0.0", "zjot-ae01cd41bbd4a358991711d5ddeae5efb606c5f2"} {
		files := map[string][]byte{
			root + "/README.md":       []byte("r"),
			root + "/test/a.test":     []byte("A"),
			root + "/test/b.test":     []byte("B"),
			root + "/test/cli/x.test": []byte("X"),
			root + "/test/tools/y.dj": []byte("Y"),
			root + "/src/z.test":      []byte("Z"),
		}
		order := []string{root + "/", root + "/README.md", root + "/test/", root + "/test/a.test", root + "/test/b.test", root + "/test/cli/x.test", root + "/test/tools/y.dj", root + "/src/z.test"}
		got, err := readCorpusTar(bytes.NewReader(buildTar(t, files, order)))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][]byte{"a.test": []byte("A"), "b.test": []byte("B")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v", root, got)
		}
	}
}

func TestReadLocal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.test"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(dir, "ORIGIN.md"), []byte("o"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.test"), []byte("C"), 0o644)
	os.Mkdir(filepath.Join(dir, "d.test"), 0o755)
	got, err := readLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string][]byte{"a.test": []byte("A")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestCompare(t *testing.T) {
	local := map[string][]byte{"a": []byte("1"), "b": []byte("1"), "c": []byte("1")}
	up := map[string][]byte{"b": []byte("1"), "c": []byte("2"), "d": []byte("1")}
	want := []change{{"a", "removed"}, {"c", "changed"}, {"d", "added"}}
	if got := compare(local, up); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := compare(local, local); len(got) != 0 {
		t.Fatalf("identical: got %v", got)
	}
}

func TestRunMatch(t *testing.T) {
	c := map[string]string{"a.test": "A", "b.test": "B"}
	s := serve(t, map[string][]byte{"ae01cd4": corpusTar(t, "zjot-ae01cd4", c)})
	dir := writeDir(t, s.URL, "ae01cd4", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	for _, w := range []string{"matches", "ae01cd4", "(2 files)"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("stdout %q lacks %q", out.String(), w)
		}
	}
	if errb.Len() != 0 {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestRunDrift(t *testing.T) {
	local := map[string]string{"keep.test": "K", "gone.test": "G", "diff.test": "old"}
	up := map[string]string{"keep.test": "K", "new.test": "N", "diff.test": "new"}
	s := serve(t, map[string][]byte{"v1": corpusTar(t, "zjot-v1", up)})
	dir := writeDir(t, s.URL, "v1", local)
	before := snapshot(t, dir)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 1 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 || lines[0] != "changed\tdiff.test" || lines[1] != "removed\tgone.test" || lines[2] != "added\tnew.test" || !strings.HasPrefix(lines[3], "driftcheck: 3 files differ") {
		t.Fatalf("stdout %q", out.String())
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Error("directory changed")
	}
}

func TestRunOtherRef(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	c2 := map[string]string{"a.test": "A", "extra.test": "E"}
	s := serve(t, map[string][]byte{"pin": corpusTar(t, "r", c), "v2.0.0": corpusTar(t, "r", c2)})
	dir := writeDir(t, s.URL, "pin", c)
	before := snapshot(t, dir)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir, "v2.0.0"}, s.Client(), &out, &errb); code != 1 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "v2.0.0") || !strings.Contains(out.String(), "added\textra.test") {
		t.Errorf("stdout %q", out.String())
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Error("directory changed")
	}
}

func TestRunRefNotFound(t *testing.T) {
	c := map[string]string{"a.test": "A"}
	s := serve(t, nil)
	dir := writeDir(t, s.URL, "nope", c)
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 2 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "nope") || !strings.Contains(errb.String(), "not found") || out.Len() != 0 {
		t.Errorf("stderr %q stdout %q", errb.String(), out.String())
	}
}

func TestRunNoOrigin(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", t.TempDir()}, http.DefaultClient, &out, &errb); code != 2 || !strings.Contains(errb.String(), "ORIGIN.md") {
		t.Errorf("no file: code %d stderr %q", code, errb.String())
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ORIGIN.md"), []byte("Repository: http://x\n"), 0o644)
	errb.Reset()
	if code := run([]string{"-dir", dir}, http.DefaultClient, &out, &errb); code != 2 || !strings.Contains(errb.String(), "Ref") {
		t.Errorf("no Ref: code %d stderr %q", code, errb.String())
	}
}

func TestRunEmptyUpstream(t *testing.T) {
	s := serve(t, map[string][]byte{"r1": corpusTar(t, "root", nil)})
	dir := writeDir(t, s.URL, "r1", map[string]string{"a.test": "A"})
	var out, errb bytes.Buffer
	if code := run([]string{"-dir", dir}, s.Client(), &out, &errb); code != 2 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "no *.test files") {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestRunUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-h"}, http.DefaultClient, &out, &errb); code != 0 || !strings.Contains(out.String(), "Usage") {
		t.Errorf("-h: code %d stdout %q", code, out.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"a", "b"}, http.DefaultClient, &out, &errb); code != 2 || !strings.Contains(errb.String(), "Usage") {
		t.Errorf("two args: code %d stderr %q", code, errb.String())
	}
	if code := run([]string{"-bogus"}, http.DefaultClient, &out, &errb); code != 2 {
		t.Errorf("-bogus: code %d", code)
	}
}
