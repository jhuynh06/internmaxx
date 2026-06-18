package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/discord"
	"github.com/jhuynh06/internmaxx/backend/internal/scraper"
)

func main() {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	company := "spacex"
	jobs, err := scraper.ScrapeGreenhouse(client, company)
	if err != nil {
		log.Fatal(err)
	}

	if err := discord.SendJobs(context.Background(), company, jobs); err != nil {
		log.Fatal(err)
	}
}
