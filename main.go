package main

import (
	"os"

	"github.com/barloc/dbt-gate/app"
)

func main() {
	if err := app.NewCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
