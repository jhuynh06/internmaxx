// Package notify delivers new-job alerts. Notifier is the seam; Discord is the
// first implementation and email can be added later without touching callers.
package notify

import (
	"context"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

type Notifier interface {
	Notify(ctx context.Context, jobs []models.Job) error
}

// Nop is a Notifier that does nothing — used when no webhook is configured so
// the daemon still runs (seeding, dedup) without sending.
type Nop struct{}

func (Nop) Notify(context.Context, []models.Job) error { return nil }
