// Command ward runs applications with the credentials they need, without the
// person running them having to hold those credentials.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tobe0504/Warder/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.Execute(ctx, os.Args[1:]); err != nil {
		// Errors go to stderr so that a command's real output on stdout stays
		// clean and pipeable. Nothing here can contain a secret value: the
		// values never reach this layer as anything but a map handed to the
		// child process.
		fmt.Fprintf(os.Stderr, "ward: %v\n", err)
		os.Exit(1)
	}
}
