package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type options struct {
	rungrid, config, stateDir, evidence string
	duration, interval                  time.Duration
	openWorkspace                       bool
}

func main() {
	var configuration options
	flag.StringVar(&configuration.rungrid, "rungrid", "", "Rungrid binary under validation")
	flag.StringVar(&configuration.config, "config", "", "workspace manifest")
	flag.StringVar(&configuration.stateDir, "state-dir", "", "optional Rungrid state root")
	flag.StringVar(&configuration.evidence, "evidence", "", "private evidence directory")
	flag.DurationVar(&configuration.duration, "duration", 24*time.Hour, "soak duration")
	flag.DurationVar(&configuration.interval, "interval", 10*time.Second, "collection interval")
	flag.BoolVar(&configuration.openWorkspace, "open-workspace", false, "open configured terminal tabs when starting an inactive runtime")
	flag.Parse()
	if configuration.rungrid == "" || configuration.config == "" || configuration.evidence == "" || configuration.duration <= 0 || configuration.interval < time.Second {
		fmt.Fprintln(os.Stderr, "rungrid, config, evidence, positive duration, and interval >=1s are required")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer cancel()
	result, err := runSoak(ctx, configuration)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
	}
	if writeErr := writeResult(configuration.evidence, result); writeErr != nil && err == nil {
		err = writeErr
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "resource guard soak failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("resource guard soak PASS: %s\n", filepath.Join(configuration.evidence, "result.json"))
}
