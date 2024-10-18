package main

import (
	"fmt"
	"os"
)

func main() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error while executing eotsd CLI: %s", err.Error())
		os.Exit(1)
	}
}
