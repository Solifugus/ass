// Command ass is the command-line interface for Analyst's Statistical Suite,
// a SAS-compatible data processing engine.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/solifugus/ass/corpus"
	"github.com/solifugus/ass/kernel"
	"github.com/solifugus/ass/lexer"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/macro"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/session"
)

const usage = `ass - Analyst's Statistical Suite

Usage:
  ass <file.sas>        Run a SAS program
  ass run <file.sas>    Run a SAS program (explicit form)
  ass repl              Start an interactive session (REPL)
  ass kernel --install  Register the ASS Jupyter kernel for the current user
  ass kernel <conn>     Run as a Jupyter kernel (invoked by Jupyter)
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
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("run: missing <file.sas>")
		}
		return runProgram(args[1])
	case "repl":
		return runREPL()
	case "kernel":
		return runKernel(args[1:])
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
	logger := log.New(os.Stderr)
	// A whole file is one submission to a fresh session — the resident model
	// generalizes the batch runner: a single Submit is the degenerate case.
	if err := session.New().Submit(string(src), logger); err != nil {
		var pe *session.ParseError
		if errors.As(err, &pe) {
			fmt.Fprintf(os.Stderr, "%d parse error(s):\n", len(pe.Errors))
			for _, e := range pe.Errors {
				fmt.Fprintln(os.Stderr, "  - "+e)
			}
			return fmt.Errorf("aborted: %d parse error(s)", len(pe.Errors))
		}
		return err
	}
	// A run that logged errors without aborting (e.g. a failing PROC PROOF
	// assertion) still exits non-zero, so CI / data-quality gates can detect it.
	if n := logger.ErrorCount(); n > 0 {
		return fmt.Errorf("completed with %d error(s)", n)
	}
	return nil
}

// runREPL starts an interactive session: it reads SAS source from stdin and
// submits it to a single resident session, so datasets, librefs, and macro
// state accumulate across entries. A fragment is submitted when the user enters
// a line that is just `run;` or `quit;`, or a blank line (which flushes any
// buffered global statements such as %let or libname). The SAS log and PROC
// output interleave on stdout. Ctrl-D (EOF) or `endsas;` exits.
func runREPL() error {
	fmt.Println("ass interactive session. Enter SAS code; `run;`/`quit;` or a blank line submits.")
	fmt.Println("Type `endsas;` or press Ctrl-D to exit.")
	s := session.New()
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var buf []string
	prompt := func() {
		if len(buf) == 0 {
			fmt.Print("ass> ")
		} else {
			fmt.Print(".... ")
		}
	}
	flush := func() {
		if len(buf) == 0 {
			return
		}
		src := strings.Join(buf, "\n")
		buf = buf[:0]
		if err := s.Submit(src, log.New(os.Stdout)); err != nil {
			var pe *session.ParseError
			if errors.As(err, &pe) {
				for _, e := range pe.Errors {
					fmt.Fprintln(os.Stderr, "parse error: "+e)
				}
			} else {
				fmt.Fprintln(os.Stderr, "error: "+err.Error())
			}
		}
	}
	prompt()
	for in.Scan() {
		line := in.Text()
		trimmed := strings.TrimSpace(line)
		low := strings.ToLower(trimmed)
		if low == "endsas;" || low == "endsas" {
			flush()
			break
		}
		if trimmed == "" {
			flush()
			prompt()
			continue
		}
		buf = append(buf, line)
		// A step terminator submits immediately, so `data ...; run;` runs as soon
		// as `run;` is typed without needing a trailing blank line.
		if low == "run;" || low == "quit;" {
			flush()
		}
		prompt()
	}
	fmt.Println()
	return in.Err()
}

// runKernel implements the `kernel` subcommand: `--install` registers the
// kernelspec with Jupyter, while a connection-file path runs the kernel itself
// (this is the form Jupyter invokes via the installed kernelspec).
func runKernel(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("kernel: need --install or a connection file path")
	}
	if args[0] == "--install" {
		dir, err := kernel.InstallSpec()
		if err != nil {
			return err
		}
		fmt.Println("Installed ASS Jupyter kernel at " + dir)
		fmt.Println("Start Jupyter and choose the \"ASS (SAS)\" kernel.")
		return nil
	}
	return kernel.Run(args[0])
}

// runParse parses a SAS source file and prints the resulting AST, plus any
// parse errors (to stderr).
func runParse(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p := parser.New(macro.Process(string(src)))
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

// runTest runs the compatibility corpus and prints a report. Flags:
//
//	--parse-only        only check parsing
//	--feature <tag>     only run items tagged <tag>
//	--json              emit a machine-readable JSON report
//	--coverage          print the per-feature value-verification backlog
//	-v / --verbose      show failure detail
func runTest(args []string) error {
	var opts corpus.Options
	verbose := false
	jsonOut := false
	coverage := false
	dir := "corpus"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--parse-only":
			opts.ParseOnly = true
		case "-v", "--verbose":
			verbose = true
		case "--json":
			jsonOut = true
		case "--coverage":
			coverage = true
		case "--feature":
			if i+1 >= len(args) {
				return fmt.Errorf("test: --feature requires a tag")
			}
			i++
			opts.Feature = args[i]
		default:
			dir = args[i]
		}
	}

	items, err := corpus.Load(dir)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no corpus items found in %s", dir)
	}

	rep := corpus.Run(items, opts)
	switch {
	case jsonOut:
		if err := rep.WriteJSON(os.Stdout); err != nil {
			return err
		}
	case coverage:
		rep.WriteCoverage(os.Stdout)
	default:
		rep.WriteReport(os.Stdout, verbose)
	}

	// CI-friendly: non-zero exit when any item fails.
	for _, r := range rep.Results {
		if !r.Pass() {
			return fmt.Errorf("%d of %d corpus items failed", countFailures(rep), len(rep.Results))
		}
	}
	return nil
}

func countFailures(rep corpus.Report) int {
	n := 0
	for _, r := range rep.Results {
		if !r.Pass() {
			n++
		}
	}
	return n
}
