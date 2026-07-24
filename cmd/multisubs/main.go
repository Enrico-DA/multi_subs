package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Enrico-DA/multi_subs/internal/multisubs"
)

func main() {
	if err := multisubs.RunCLI(os.Args[1:]); err != nil {
		var exitErr *multisubs.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Message != "" {
				fmt.Fprintln(os.Stderr, exitErr.Message)
			}
			os.Exit(exitErr.Code)
		}
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
