package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hazz-dev/servprobe/internal/checker"
	"github.com/hazz-dev/servprobe/internal/config"
)

func executeCheck(cmd *cobra.Command, cfg *config.Config) error {
	return runChecks(cmd.OutOrStdout(), cfg)
}

func runChecks(out io.Writer, cfg *config.Config) error {
	type result struct {
		svc    config.Service
		result checker.CheckResult
	}

	results := make([]result, len(cfg.Services))
	var wg sync.WaitGroup

	for i, svc := range cfg.Services {
		wg.Add(1)
		go func(i int, svc config.Service) {
			defer wg.Done()
			c, err := checker.New(svc)
			if err != nil {
				results[i] = result{
					svc: svc,
					result: checker.CheckResult{
						ServiceName: svc.Name,
						Status:      checker.StatusDown,
						Error:       fmt.Sprintf("creating checker: %v", err),
						CheckedAt:   time.Now(),
					},
				}
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), svc.Timeout.Duration)
			defer cancel()
			results[i] = result{svc: svc, result: c.Check(ctx)}
		}(i, svc)
	}
	wg.Wait()

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tTYPE\tSTATUS\tRESPONSE\tERROR")
	allUp := true
	for _, r := range results {
		resp := "—"
		if r.result.ResponseTime > 0 {
			resp = r.result.ResponseTime.Round(time.Millisecond).String()
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.svc.Name,
			r.svc.Type,
			r.result.Status,
			resp,
			r.result.Error,
		)
		if r.result.Status != checker.StatusUp {
			allUp = false
		}
	}
	w.Flush()

	if !allUp {
		return fmt.Errorf("one or more services are down")
	}
	return nil
}
