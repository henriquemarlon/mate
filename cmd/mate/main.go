package main

import (
	"os"

	"github.com/henriquemarlon/mate/cmd/mate/root"
)

func main() {
	err := root.Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
