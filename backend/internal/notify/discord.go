package notify

import (
	"context"
	"fmt"
	"sort"
	"time"

	webhooks "github.com/typical-developers/discord-webhooks-go/v2"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

const (
	embedColor = 0x5865F2
	maxEmbeds  = 10 // Discord's per-message embed limit
)

// Discord sends job alerts to a webhook. Credentials come from config (env),
// never hardcoded.
type Discord struct {
	client *webhooks.WebhookClient
}

func NewDiscord(id, secret string) *Discord {
	return &Discord{client: webhooks.NewWebhookClient(id, secret)}
}

func jobToEmbed(job models.Job) webhooks.Embed {
	location := job.Region
	if job.Country != "" && job.Country != job.Region {
		if location != "" {
			location = fmt.Sprintf("%s, %s", job.Region, job.Country)
		} else {
			location = job.Country
		}
	}

	fields := []webhooks.EmbedField{}
	if job.Category != "" {
		fields = append(fields, webhooks.EmbedField{Name: "Category", Value: job.Category, Inline: true})
	}
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

// Notify groups jobs by company and sends batched embed messages.
func (d *Discord) Notify(ctx context.Context, jobs []models.Job) error {
	if len(jobs) == 0 {
		return nil
	}

	byCompany := map[string][]models.Job{}
	var order []string
	for _, j := range jobs {
		if _, ok := byCompany[j.Company]; !ok {
			order = append(order, j.Company)
		}
		byCompany[j.Company] = append(byCompany[j.Company], j)
	}
	sort.Strings(order)

	for _, company := range order {
		if err := d.sendCompany(ctx, company, byCompany[company]); err != nil {
			return err
		}
	}
	return nil
}

func (d *Discord) sendCompany(ctx context.Context, company string, jobs []models.Job) error {
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

		_, _, err := d.client.Execute(ctx, webhooks.MessagePayload{
			Content: content,
			Embeds:  embeds,
		}, nil)
		if err != nil {
			return fmt.Errorf("discord webhook: %w", err)
		}
		// Gentle spacing to stay under the ~30 msg/min webhook limit.
		if end < len(jobs) {
			select {
			case <-time.After(1200 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}
