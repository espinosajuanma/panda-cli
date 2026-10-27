package main

import (
	"fmt"
	"os"

	"panda-dashboard/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "panda:", err)
		os.Exit(1)
	}
}
