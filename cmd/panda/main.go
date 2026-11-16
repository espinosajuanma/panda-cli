package main

import (
	"fmt"
	"io"
	"os"

	"github.com/espinosajuanma/panda-cli/internal/app"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if err := app.Run(args, in, out, errOut); err != nil {
		fmt.Fprintln(errOut, "panda:", err)
		return 1
	}
	return 0
}
