package main

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hazz-dev/servprobe/internal/storage"
)

type statusStore interface {
	AllLatest(ctx context.Context) ([]storage.Check, error)
}

func executeStatus(cmd *cobra.Command, db statusStore) error {
	out := cmd.OutOrStdout()
	checks, err := db.AllLatest(context.Background())
	if err != nil {
		return fmt.Errorf("querying status: %w", err)
	}

	if len(checks) == 0 {
		fmt.Fprintln(out, "No check history. Run 'servprobe serve' or 'servprobe check' first.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS\tRESPONSE\tLAST CHECKED\tERROR")
	for _, c := range checks {
		resp := "—"
		if c.ResponseMs > 0 {
			resp = time.Duration(c.ResponseMs * int64(time.Millisecond)).Round(time.Millisecond).String()
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.Service,
			c.Status,
			resp,
			c.CheckedAt.Local().Format("2006-01-02 15:04:05"),
			c.Error,
		)
	}
	w.Flush()
	return nil
}
