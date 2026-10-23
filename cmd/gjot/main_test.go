package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runWith(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunDefaultHTML(t *testing.T) {
	code, out, _ := runWith(t, "Hello *world*\n")
	if code != 0 || out != "<p>Hello <strong>world</strong></p>\n" {
		t.Errorf("code %d, out %q", code, out)
	}
}

func TestRunAST(t *testing.T) {
	code, out, _ := runWith(t, "# H\n", "--ast")
	if code != 0 || out != "doc\n  section id=\"H\"\n    heading level=1\n      str text=\"H\"\n" {
		t.Errorf("code %d, out %q", code, out)
	}
	_, out, _ = runWith(t, "a\n", "--ast", "--sourcepos")
	if !strings.Contains(out, "para (1:1:0-2:0:1)") {
		t.Errorf("no positions in %q", out)
	}
}

func TestRunJSON(t *testing.T) {
	code, out, _ := runWith(t, "a\n", "--json")
	var doc map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &doc) != nil || doc["tag"] != "section" {
		t.Errorf("code %d, out %q", code, out)
	}
}

func TestRunDjot(t *testing.T) {
	code, out, _ := runWith(t, "# H\n\n- a\n- b\n", "--djot")
	if code != 0 || out != "# H\n\n- a\n- b\n" {
		t.Errorf("code %d, out %q", code, out)
	}
}

func TestRunFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in.dj")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := runWith(t, "", path); code != 0 || out != "<p>x</p>\n" {
		t.Errorf("code %d, out %q", code, out)
	}
	code, _, errOut := runWith(t, "", filepath.Join(t.TempDir(), "missing.dj"))
	if code != 1 || !strings.Contains(errOut, "missing.dj") {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

func TestRunUsage(t *testing.T) {
	code, _, errOut := runWith(t, "", "--bogus")
	if code != 1 || !strings.Contains(errOut, "--bogus") || !strings.Contains(errOut, "Usage") {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
	code, out, _ := runWith(t, "", "-h")
	if code != 0 || !strings.Contains(out, "Usage") {
		t.Errorf("code %d, stdout %q", code, out)
	}
}
