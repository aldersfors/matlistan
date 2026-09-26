// Command matlistan plans the household's dinners for the coming week.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	_ "time/tzdata" // distroless has no zoneinfo; MATLISTAN_TIMEZONE needs it

	"github.com/jalet/matlistan/internal/release"
)

// env is what every subcommand receives.
type env struct {
	stdout, stderr io.Writer
	getenv         func(string) string
}

var _commands = map[string]func(ctx context.Context, e env) int{
	"version": func(_ context.Context, e env) int {
		_, _ = fmt.Fprintln(e.stdout, release.Version())
		return 0
	},
	"migrate": notYet("migrate"),
	"serve":   notYet("serve"),
}

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	names := make([]string, 0, len(_commands))
	for n := range _commands {
		names = append(names, n)
	}
	sort.Strings(names)
	usage := "usage: matlistan <" + strings.Join(names, "|") + ">"
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, usage)
		return 2
	}
	cmd, ok := _commands[args[1]]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n%s\n", args[1], usage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cmd(ctx, env{stdout: stdout, stderr: stderr, getenv: getenv})
}

func notYet(name string) func(context.Context, env) int {
	return func(_ context.Context, e env) int {
		_, _ = fmt.Fprintf(e.stderr, "%s: not implemented yet\n", name)
		return 1
	}
}
