// Command ass is the command-line interface for Analyst's Statistical Suite,
// a SAS-compatible data processing engine.
package main

import (
	"fmt"
	"os"

	"github.com/solifugus/ass/lexer"
	"github.com/solifugus/ass/parser"
)

const usage = `ass - Analyst's Statistical Suite

Usage:
  ass <file.sas>       Run a SAS program (currently: dump tokens)
  ass parse <file.sas> Parse a SAS program and print its AST
  ass test <dir>       Run the compatibility corpus in <dir>
  ass --help           Show this help

ass is an independent open-source project, not affiliated with SAS Institute Inc.`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ass: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Println(usage)
		return nil
	}

	switch args[0] {
	case "--help", "-h", "help":
		fmt.Println(usage)
		return nil
	case "test":
		return runTest(args[1:])
	case "parse":
		if len(args) < 2 {
			return fmt.Errorf("parse: missing <file.sas>")
		}
		return runParse(args[1])
	default:
		return runFile(args[0])
	}
}

// runParse parses a SAS source file and prints the resulting AST, plus any
// parse errors (to stderr).
func runParse(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p := parser.New(string(src))
	prog := p.ParseProgram()
	fmt.Println(prog.String())
	if errs := p.Errors(); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d parse error(s):\n", len(errs))
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "  - "+e)
		}
		return fmt.Errorf("parse completed with %d error(s)", len(errs))
	}
	return nil
}

// runFile reads a SAS source file and, until the parser/runtime exist, prints
// its token stream (debug mode).
func runFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	l := lexer.New(string(src))
	for {
		tok := l.NextToken()
		if tok.Type == lexer.EOF {
			break
		}
		fmt.Printf("%4d:%-3d %-14s %q\n", tok.Line, tok.Col, tok.Type, tok.Literal)
	}
	fmt.Fprintln(os.Stderr, "note: execution pipeline not yet implemented (token dump only)")
	return nil
}

// runTest will run the compatibility corpus. Stubbed until Phase 11.
func runTest(args []string) error {
	dir := "corpus"
	if len(args) > 0 {
		dir = args[0]
	}
	fmt.Printf("test harness not implemented (target dir: %s)\n", dir)
	return nil
}
