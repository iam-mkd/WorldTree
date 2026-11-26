// Command worldtree gives each parallel coding agent its own isolated local runtime.
package main

import (
	"os"

	"github.com/iam-mkd/WorldTree/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
