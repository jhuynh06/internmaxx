// Command scrape-once runs a single pass over the scoped companies (and
// optionally the Simplify aggregator), printing matched internships. It does
// not persist or notify — it's for testing a company/slug before adding it.
//
//	scrape-once --only openai
//	scrape-once --groups quant --tiers 1
//	scrape-once --simplify
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/cli"
	"github.com/jhuynh06/internmaxx/backend/internal/config"
	"github.com/jhuynh06/internmaxx/backend/internal/filter"
	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
	"github.com/jhuynh06/internmaxx/backend/internal/scraper"
)

func main() {
	fs := flag.NewFlagSet("scrape-once", flag.ExitOnError)
	companiesPath := fs.String("companies", "companies.yaml", "path to companies.yaml")
	showRaw := fs.Bool("raw", false, "print all postings, not just matched internships")
	doSimplify := fs.Bool("simplify", false, "also run the Simplify aggregator")
	scopeFlags := cli.RegisterScope(fs)
	_ = fs.Parse(os.Args[1:])

	cfg := config.Load()
	filterCfg := filter.Config{USOnly: cfg.USOnly, AllowPhD: cfg.AllowPhD}
	client := scraper.NewClient(cfg.HostMinGap)
	scrapers := scraper.All()

	scope, err := scopeFlags.Scope()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad scope:", err)
		os.Exit(1)
	}
	reg, err := registry.Load(*companiesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load registry:", err)
		os.Exit(1)
	}
	companies := reg.Filter(scope)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	totalMatched := 0
	for _, c := range companies {
		sc, ok := scrapers[c.ATS]
		if !ok {
			fmt.Printf("%-24s SKIP  (no scraper for ats %q)\n", c.Name, c.ATS)
			continue
		}
		raw, err := sc.Fetch(ctx, client, c)
		if err != nil {
			fmt.Printf("%-24s ERR   %v\n", c.Name, err)
			continue
		}
		matched := filter.Apply(raw, filterCfg)
		totalMatched += len(matched)
		fmt.Printf("%-24s %-14s raw=%-4d matched=%d\n", c.Name, "("+c.ATS+")", len(raw), len(matched))
		printJobs(matched)
		if *showRaw {
			fmt.Println("  --- all postings ---")
			printJobs(raw)
		}
	}

	if *doSimplify {
		fmt.Println("\n=== Simplify aggregator ===")
		agg := scraper.NewSimplify()
		jobs, _, err := agg.Fetch(ctx, client)
		if err != nil {
			fmt.Fprintln(os.Stderr, "simplify:", err)
		} else {
			matched := filter.Apply(jobs, filterCfg)
			fmt.Printf("active=%d matched=%d\n", len(jobs), len(matched))
			totalMatched += len(matched)
			// Print a sample so we don't flood the terminal.
			printJobs(matched[:min(len(matched), 40)])
		}
	}

	fmt.Printf("\nTotal matched internships: %d\n", totalMatched)
}

func printJobs(jobs []models.Job) {
	for _, j := range jobs {
		cat := j.Category
		if cat == "" {
			cat = "-"
		}
		loc := j.Region
		if loc == "" {
			loc = j.Modality
		}
		fmt.Printf("    • [%-16s] %s  (%s)  %s\n", cat, j.Position, loc, j.Link)
	}
}
