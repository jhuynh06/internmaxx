// Package cli holds small helpers shared by the command binaries.
package cli

import (
	"flag"
	"os"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// ScopeFlags registers the shared run-scoping flags on fs and returns a
// resolver. Flags override environment variables (SCOPE_GROUPS, SCOPE_TIERS,
// SCOPE_ONLY, SCOPE_EXCLUDE); env provides defaults for the deployed daemon.
type ScopeFlags struct {
	groups  *string
	tiers   *string
	only    *string
	exclude *string
}

func RegisterScope(fs *flag.FlagSet) *ScopeFlags {
	return &ScopeFlags{
		groups:  fs.String("groups", envOr("SCOPE_GROUPS"), "comma-separated groups/aliases to include"),
		tiers:   fs.String("tiers", envOr("SCOPE_TIERS"), "comma-separated tiers to include (e.g. 1,2)"),
		only:    fs.String("only", envOr("SCOPE_ONLY"), "comma-separated slugs allowlist"),
		exclude: fs.String("exclude", envOr("SCOPE_EXCLUDE"), "comma-separated slugs to drop"),
	}
}

func (s *ScopeFlags) Scope() (registry.Scope, error) {
	return registry.ParseScope(*s.groups, *s.tiers, *s.only, *s.exclude)
}

func envOr(key string) string { return os.Getenv(key) }
