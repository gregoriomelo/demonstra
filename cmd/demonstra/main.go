package main

import (
	"context"
	"os"

	"github.com/demonstra-me/demonstra/internal/cli"
)

func main() {
	os.Exit(cli.Execute(context.Background(), os.Args[1:], cli.Env{
		LookupEnv: os.Getenv,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}))
}
