package main

import (
	"fmt"
	"os"

	"github.com/B23admin/kubectl-config-cleanup/cleanup"
)

func main() {
	root := cleanup.NewCmdCleanup(os.Stdin, os.Stdout, os.Stderr)
	if err := root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// func main() {
// 	cleanup.Execute()
// }
