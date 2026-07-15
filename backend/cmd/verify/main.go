// Command verify checks that every scoped registry entry resolves to a live
// board with postings, and (for Greenhouse) that the board's declared name
// matches the company — catching slug collisions like greenhouse "figure"
// (Figure Lending) vs "figureai" (the robotics company).
//
//	verify                 # check everything
//	verify --groups quant  # check a subset
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/cli"
	"github.com/jhuynh06/internmaxx/backend/internal/config"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
	"github.com/jhuynh06/internmaxx/backend/internal/scraper"
)

func main() {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	companiesPath := fs.String("companies", "companies.yaml", "path to companies.yaml")
	scopeFlags := cli.RegisterScope(fs)
	_ = fs.Parse(os.Args[1:])

	cfg := config.Load()
	client := scraper.NewClient(cfg.HostMinGap)
	scrapers := scraper.All()
	gh := scraper.Greenhouse{}

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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var okCount, emptyCount, errCount, warnCount int
	for _, c := range companies {
		sc, ok := scrapers[c.ATS]
		if !ok {
			fmt.Printf("%-26s %-16s SKIP   no scraper\n", c.Name, c.ATS)
			continue
		}
		raw, err := sc.Fetch(ctx, client, c)
		if err != nil {
			fmt.Printf("%-26s %-16s ERR    %v\n", c.Name, c.ATS+"/"+c.Slug, err)
			errCount++
			continue
		}

		status := "OK"
		note := fmt.Sprintf("jobs=%d", len(raw))
		if len(raw) == 0 {
			status = "EMPTY"
			emptyCount++
		} else {
			okCount++
		}

		if c.ATS == "greenhouse" {
			if name, err := gh.BoardName(ctx, client, c.Slug); err == nil && !nameMatches(c.Name, name) {
				status = "WARN"
				note += fmt.Sprintf("  board=%q != company", name)
				warnCount++
			}
		}

		fmt.Printf("%-26s %-16s %-6s %s\n", c.Name, c.ATS+"/"+c.Slug, status, note)
	}

	fmt.Printf("\nchecked=%d  ok=%d  empty=%d  warn=%d  err=%d\n",
		len(companies), okCount, emptyCount, warnCount, errCount)
	if errCount > 0 {
		os.Exit(1)
	}
}

func nameMatches(company, board string) bool {
	c := normalize(company)
	b := normalize(board)
	if c == "" || b == "" {
		return true
	}
	return strings.Contains(b, c) || strings.Contains(c, b)
}

func normalize(s string) string {
	// Drop parenthetical qualifiers like "Figure (robotics)".
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
