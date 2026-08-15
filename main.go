// Command tropmail is the terminal client for the TropMail API.
//
// Run it with no arguments for the interactive inbox, or use any subcommand
// with --json for scripting.
package main

import (
	"os"

	"github.com/tropmail/tropmail-cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
