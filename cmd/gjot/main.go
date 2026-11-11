// Command gjot converts Djot markup to HTML, AST text, JSON, or Djot.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/devselfio/gjot"
)

const usage = `Usage: gjot [OPTIONS] [FILE]

Parse Djot markup and produce output.

If FILE is omitted, reads from stdin.

Options:
  --ast         Output AST text instead of HTML
  --sourcepos   Include source positions in AST output
  --json        Output the AST as JSON
  --djot        Output the AST written back as Djot
  -h, --help    Show this help message
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	mode, sourcepos, file := "html", false, ""
	for _, arg := range args {
		switch arg {
		case "--ast", "--json", "--djot":
			mode = arg[2:]
		case "--sourcepos":
			sourcepos = true
		case "-h", "--help":
			io.WriteString(stdout, usage)
			return 0
		default:
			if len(arg) > 0 && arg[0] == '-' {
				fmt.Fprintf(stderr, "gjot: unknown option: %s\n%s", arg, usage)
				return 1
			}
			file = arg
		}
	}

	var input []byte
	var err error
	if file == "" {
		input, err = io.ReadAll(stdin)
	} else {
		input, err = os.ReadFile(file)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gjot: %v\n", err)
		return 1
	}

	var opts []gjot.ParseOption
	if sourcepos {
		opts = append(opts, gjot.WithSourcePos())
	}
	doc := gjot.Parse(input, opts...)

	var out []byte
	switch mode {
	case "ast":
		out = gjot.ASTText(doc)
	case "json":
		out, err = json.MarshalIndent(doc, "", "  ")
		out = append(out, '\n')
	case "djot":
		out = gjot.Djot(doc)
	default:
		out, err = gjot.HTML(doc)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gjot: %v\n", err)
		return 1
	}
	stdout.Write(out)
	return 0
}
