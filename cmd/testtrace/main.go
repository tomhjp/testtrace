package main

import (
	"bufio"
	"log"
	"os"

	"github.com/tomhjp/testtrace"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tw, err := testtrace.NewTraceWriter(os.Stdout)
	if err != nil {
		return err
	}
	s := bufio.NewScanner(os.Stdin)
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
