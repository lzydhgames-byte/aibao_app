// cost-report is a command-line tool to summarize cost_events + outline_events
// for operations / pricing decisions. Plan 11B §7.1.
//
// Examples:
//
//	cost-report overall --since=last_7d
//	cost-report overall --since=2026-04-01 --until=2026-04-30
//	cost-report purpose --since=last_30d
//	cost-report users --since=last_month --limit=5
//
// Reads the same config the server uses (AIBAO_CONFIG env or --config flag).
// Read-only — never writes to PG.
package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/aibao/server/internal/pkg/config"
	"github.com/aibao/server/internal/repository"
	"github.com/aibao/server/internal/service/cost"
)

func main() {
	root := &cobra.Command{
		Use:   "cost-report",
		Short: "Plan 11B cost observability — summarize cost_events + outline_events",
	}
	root.AddCommand(overallCmd())
	root.AddCommand(purposeCmd())
	root.AddCommand(usersCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// commonFlags carries flag values shared across subcommands.
type commonFlags struct {
	since      string
	until      string
	configPath string
}

func (f *commonFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.since, "since", "last_7d", "start of range (YYYY-MM-DD | last_7d | last_30d | last_month)")
	cmd.Flags().StringVar(&f.until, "until", "now", "end of range (YYYY-MM-DD | now)")
	defaultCfg := os.Getenv("AIBAO_CONFIG")
	if defaultCfg == "" {
		defaultCfg = "config/config.dev.yaml"
	}
	cmd.Flags().StringVar(&f.configPath, "config", defaultCfg, "path to server config YAML (defaults to $AIBAO_CONFIG)")
}

func (f *commonFlags) parse() (time.Time, time.Time, error) {
	s, err := parseSince(f.since)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --since: %w", err)
	}
	u, err := parseUntil(f.until)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --until: %w", err)
	}
	return s, u, nil
}

func parseSince(s string) (time.Time, error) {
	now := time.Now()
	switch s {
	case "last_7d":
		return now.Add(-7 * 24 * time.Hour), nil
	case "last_30d":
		return now.Add(-30 * 24 * time.Hour), nil
	case "last_month":
		return time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location()), nil
	default:
		return time.Parse("2006-01-02", s)
	}
}

func parseUntil(s string) (time.Time, error) {
	if s == "now" {
		return time.Now(), nil
	}
	return time.Parse("2006-01-02", s)
}

// connectDB opens the same PG the server uses via repository.NewDB.
func connectDB(configPath string) (*cost.Aggregator, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", configPath, err)
	}
	db, err := repository.NewDB(cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("open pg: %w", err)
	}
	return cost.NewAggregator(db), nil
}

func overallCmd() *cobra.Command {
	flags := &commonFlags{}
	cmd := &cobra.Command{
		Use:   "overall",
		Short: "top-level cost summary + outline outcome counts + saving estimate",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, u, err := flags.parse()
			if err != nil {
				return err
			}
			agg, err := connectDB(flags.configPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			st, err := agg.Overall(ctx, s, u)
			if err != nil {
				return err
			}
			saved, err := agg.OutlineSaving(ctx, s, u)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer w.Flush()
			fmt.Fprintf(w, "Period:\t%s to %s\n\n", s.Format("2006-01-02"), u.Format("2006-01-02"))
			fmt.Fprintln(w, "=== Overall ===")
			fmt.Fprintf(w, "Total spent:\t¥%.2f\n", st.TotalYuan)
			fmt.Fprintf(w, "Stories accepted:\t%d\n", st.StoriesAccepted)
			fmt.Fprintf(w, "Outlines previewed:\t%d\n", st.OutlinesPreviewed)
			fmt.Fprintf(w, "  accepted:\t%d\n", st.OutlinesAccepted)
			fmt.Fprintf(w, "  refreshed:\t%d\n", st.OutlinesRefreshed)
			fmt.Fprintf(w, "  expired:\t%d\n", st.OutlinesExpired)
			fmt.Fprintf(w, "\nOutline saving (full-pipeline formula):\t¥%.2f\n", saved)
			return nil
		},
	}
	flags.bind(cmd)
	return cmd
}

func purposeCmd() *cobra.Command {
	flags := &commonFlags{}
	cmd := &cobra.Command{
		Use:   "purpose",
		Short: "cost broken down by purpose (story / tts / outline / ...)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, u, err := flags.parse()
			if err != nil {
				return err
			}
			agg, err := connectDB(flags.configPath)
			if err != nil {
				return err
			}
			rows, err := agg.ByPurpose(context.Background(), s, u)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer w.Flush()
			fmt.Fprintf(w, "Period:\t%s to %s\n\n", s.Format("2006-01-02"), u.Format("2006-01-02"))
			fmt.Fprintln(w, "=== By Purpose ===")
			fmt.Fprintln(w, "purpose\tcost_yuan")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t¥%.4f\n", r.Purpose, r.CostYuan)
			}
			return nil
		},
	}
	flags.bind(cmd)
	return cmd
}

func usersCmd() *cobra.Command {
	flags := &commonFlags{}
	var limit int
	cmd := &cobra.Command{
		Use:   "users",
		Short: "top spenders ranked by total cost (HMAC-hashed ids)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, u, err := flags.parse()
			if err != nil {
				return err
			}
			agg, err := connectDB(flags.configPath)
			if err != nil {
				return err
			}
			rows, err := agg.TopUsers(context.Background(), s, u, limit)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer w.Flush()
			fmt.Fprintf(w, "Period:\t%s to %s\n\n", s.Format("2006-01-02"), u.Format("2006-01-02"))
			fmt.Fprintln(w, "=== Top Users (HMAC-hashed) ===")
			fmt.Fprintln(w, "user_hash\tstories\toutlines\ttotal_yuan")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%d\t%d\t¥%.4f\n", r.UserIDHash, r.Stories, r.Outlines, r.TotalYuan)
			}
			return nil
		},
	}
	flags.bind(cmd)
	cmd.Flags().IntVar(&limit, "limit", 10, "max rows")
	return cmd
}
