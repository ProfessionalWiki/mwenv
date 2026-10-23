package main

import (
	"os"

	"github.com/ProfessionalWiki/mwenv/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
