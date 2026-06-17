// Command ass is the command-line interface for Analyst's Statistical Suite,
// a SAS-compatible data processing engine.
package main

import (
	"fmt"
	"os"

	"github.com/solifugus/ass/lexer"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/runtime"
	"github.com/solifugus/ass/table"
)

const usage = `ass - Analyst's Statistical Suite

Usage:
  ass <file.sas>        Run a SAS program
  ass parse <file.sas>  Parse a SAS program and print its AST
  ass tokens <file.sas> Dump the token stream (lexer debug)
  ass test <dir>        Run the compatibility corpus in <dir>
  ass --help            Show this help

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
	case "tokens":
		if len(args) < 2 {
			return fmt.Errorf("tokens: missing <file.sas>")
		}
		return dumpTokens(args[1])
	default:
		return runProgram(args[0])
	}
}

// runProgram reads, parses, and executes a SAS program. The SAS-style log is
// written to stderr; PROC output goes to stdout. Parse errors abort before
// execution.
func runProgram(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p := parser.New(string(src))
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "%d parse error(s):\n", len(errs))
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "  - "+e)
		}
		return fmt.Errorf("aborted: %d parse error(s)", len(errs))
	}
	logger := log.New(os.Stderr)
	lib := table.NewLibrary()
	return runtime.RunProgram(prog, lib, logger)
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

// dumpTokens reads a SAS source file and prints its token stream (lexer debug).
func dumpTokens(path string) error {
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
