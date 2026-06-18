package discord

import (
	"context"
	"fmt"

	webhooks "github.com/typical-developers/discord-webhooks-go/v2"
	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

const (
	webhookID     = "1515597973306871860"
	webhookSecret = "UrdsnG1Q7S-UrULOWciXWl4XDD90hxc6PiA5BADXpeSasL4z3dvM_fPaJXxoCHoWkNAu"
	embedColor    = 0x5865F2
	maxEmbeds     = 10
)

func jobToEmbed(job models.Job) webhooks.Embed {
	location := job.Region
	if job.Country != "" && job.Country != job.Region {
		location = fmt.Sprintf("%s, %s", job.Region, job.Country)
	}

	fields := []webhooks.EmbedField{}
	if location != "" {
		fields = append(fields, webhooks.EmbedField{Name: "Location", Value: location, Inline: true})
	}
	if job.Modality != "" {
		fields = append(fields, webhooks.EmbedField{Name: "Modality", Value: job.Modality, Inline: true})
	}
	if !job.Date.IsZero() {
		fields = append(fields, webhooks.EmbedField{Name: "Posted", Value: job.Date.Format("Jan 2, 2006"), Inline: true})
	}

	return webhooks.Embed{
		Title:  job.Position,
		URL:    job.Link,
		Color:  embedColor,
		Fields: fields,
	}
}

func SendJobs(ctx context.Context, company string, jobs []models.Job) error {
	if len(jobs) == 0 {
		return nil
	}

	client := webhooks.NewWebhookClient(webhookID, webhookSecret)

	for i := 0; i < len(jobs); i += maxEmbeds {
		end := i + maxEmbeds
		if end > len(jobs) {
			end = len(jobs)
		}

		batch := jobs[i:end]
		embeds := make([]webhooks.Embed, len(batch))
		for j, job := range batch {
			embeds[j] = jobToEmbed(job)
		}

		content := ""
		if i == 0 {
			content = fmt.Sprintf("**%s** — %d new internship listing(s)", company, len(jobs))
		}

		_, _, err := client.Execute(ctx, webhooks.MessagePayload{
			Content: content,
			Embeds:  embeds,
		}, nil)
		if err != nil {
			return fmt.Errorf("discord webhook: %w", err)
		}
	}

	return nil
}
