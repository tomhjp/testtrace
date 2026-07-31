package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/tomhjp/testtrace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func run() error {
	var (
		in  = flag.String("i", "", "path to a file with recorded go test -json output; use - for stdin")
		out = flag.String("o", "", "path to write the trace to; defaults to stdout")
	)
	flag.Parse()

	w := os.Stdout
	if *out != "" {
		wf, err := os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("error opening out file %q: %w", *out, err)
		}
		defer wf.Close()
		w = wf
	}
	tw, err := testtrace.NewTraceWriter(w)
	if err != nil {
		return err
	}

	info, err := os.Stdin.Stat()
	if err != nil {
		return err
	}

	var s *bufio.Scanner
	if (info.Mode()&os.ModeNamedPipe) != 0 || (info.Mode()&os.ModeType) == 0 {
		// Either a pipe or file redirection is attached to stdin, read that.
		s = bufio.NewScanner(os.Stdin)
	} else {
		switch *in {
		case "":
			// TODO: support running the tests ourselves
			return errors.New("must pipe go test -json output to stdin or pass -i")
		case "-":
			s = bufio.NewScanner(os.Stdin)
		default:
			rf, err := os.Open(*in)
			if err != nil {
				return fmt.Errorf("error opening %q: %w", *in, err)
			}
			defer rf.Close()
			s = bufio.NewScanner(rf)
		}
	}
	for s.Scan() {
		if err := tw.AddTest2JSONLine(s.Bytes()); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}

	return nil
}
