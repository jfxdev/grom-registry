package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jfxdev/grom/backend/internal/platform/registrymaintenance"
)

func main() {
	flags := flag.NewFlagSet("grom-registry-maintenance", flag.ExitOnError)
	socket := flags.String("socket", "/run/grom-registry-maintenance/agent.sock", "Unix control socket")
	config := flags.String("config", "/etc/distribution/config.yml", "Distribution configuration")
	data := flags.String("data", "/var/lib/registry", "Distribution storage")
	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := registrymaintenance.Serve(ctx, registrymaintenance.AgentOptions{SocketPath: *socket, ConfigPath: *config, DataPath: *data}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
