package main

import (
	"os"

	"github.com/b23llc/kubectl-config-cleanup/pkg/cmd"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-config-cleanup", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewCmdCleanup(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
