package main

import (
	"os"

	"github.com/B23admin/kubectl-config-cleanup/cleanup"
	"github.com/spf13/pflag"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-config-cleanup", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cleanup.NewCmdCleanup(os.Stdin, os.Stdout, os.Stderr)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
