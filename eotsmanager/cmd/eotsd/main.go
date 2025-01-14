package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"

	dcli "github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
)

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "[eotsd] %v\n", err)
	os.Exit(1)
}

func main() {
	app := cli.NewApp()
	app.Name = "eotsd"
	app.Usage = "Extractable One Time Signature Daemon (eotsd)."
	app.Commands = append(
		app.Commands, dcli.StartCommand, dcli.InitCommand, dcli.SignSchnorrSig, dcli.VerifySchnorrSig,
		dcli.PoPCommands,
	)
	app.Commands = append(app.Commands, dcli.KeysCommands...)

	if err := app.Run(os.Args); err != nil {
		fatal(err)
	}
}
