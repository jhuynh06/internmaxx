// Package registry loads the company list from companies.yaml and applies
// run-scoping filters (--groups / --tiers / --only / --exclude).
//
// The file may be either form:
//
//	# bare sequence (current)
//	- {name: OpenAI, ats: ashby, slug: openai, tier: 1, group: ai}
//
//	# mapping form (enables alias groups)
//	groups:
//	  dream: [openai, anthropic, janestreet]
//	companies:
//	  - {name: OpenAI, ats: ashby, slug: openai, tier: 1, group: ai}
package registry

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Workday holds the three coordinates a Workday tenant needs (phase-2 sources).
// Search overrides the default "internship" searchText for boards whose intern
// postings don't fuzzy-match it (e.g. Snap: "internship" 2 hits, "intern" 93).
type Workday struct {
	Tenant   string `yaml:"tenant"`
	Instance string `yaml:"instance"`
	Site     string `yaml:"site"`
	Search   string `yaml:"search,omitempty"`
}

// ORC holds the coordinates for an Oracle Recruiting Cloud tenant (banks etc).
type ORC struct {
	Tenant string `yaml:"tenant"` // e.g. "jpmc"
	Site   string `yaml:"site"`   // siteNumber, e.g. "CX_1001"
}

// Eightfold holds the coordinates for an Eightfold-backed careers site
// (Millennium etc). Query narrows at the source; empty fetches the whole board
// (Eightfold's fuzzy match can miss intern titles, so small boards omit it).
type Eightfold struct {
	Host   string `yaml:"host"`   // e.g. "mlp.eightfold.ai" or a vanity domain
	Domain string `yaml:"domain"` // Eightfold tenant domain, e.g. "mlp.com"
	Query  string `yaml:"query,omitempty"`
}

type Company struct {
	Name    string   `yaml:"name"`
	ATS     string   `yaml:"ats"`
	Slug    string   `yaml:"slug"`
	Tier    int      `yaml:"tier"`
	Group   string   `yaml:"group"`
	Workday   *Workday   `yaml:"workday,omitempty"`
	ORC       *ORC       `yaml:"orc,omitempty"`
	Eightfold *Eightfold `yaml:"eightfold,omitempty"`
}

type Registry struct {
	Companies []Company
	Aliases   map[string][]string // alias name -> slug list
}

// Scope is the intersection of run-scoping filters. Empty fields mean "no
// constraint on this axis". Exclude is always applied last.
type Scope struct {
	Groups  []string
	Tiers   []int
	Only    []string
	Exclude []string
}

// Load reads and parses companies.yaml (either supported form).
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("registry: read %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("registry: parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return &Registry{}, nil
	}
	root := doc.Content[0]

	reg := &Registry{Aliases: map[string][]string{}}
	switch root.Kind {
	case yaml.SequenceNode:
		if err := root.Decode(&reg.Companies); err != nil {
			return nil, fmt.Errorf("registry: decode companies: %w", err)
		}
	case yaml.MappingNode:
		var m struct {
			Groups    map[string][]string `yaml:"groups"`
			Companies []Company           `yaml:"companies"`
		}
		if err := root.Decode(&m); err != nil {
			return nil, fmt.Errorf("registry: decode document: %w", err)
		}
		reg.Companies = m.Companies
		if m.Groups != nil {
			reg.Aliases = m.Groups
		}
	default:
		return nil, fmt.Errorf("registry: unexpected top-level YAML kind in %s", path)
	}
	return reg, nil
}

// Filter returns the companies matching the scope. All axes intersect;
// Exclude removes by slug last.
func (r *Registry) Filter(s Scope) []Company {
	// Expand requested groups into (categories) + (alias slug allowlist).
	categoryWanted := map[string]bool{}
	aliasSlugs := map[string]bool{}
	for _, g := range s.Groups {
		if slugs, ok := r.Aliases[g]; ok {
			for _, sl := range slugs {
				aliasSlugs[sl] = true
			}
		} else {
			categoryWanted[g] = true
		}
	}

	onlySet := strSet(s.Only)
	excludeSet := strSet(s.Exclude)
	tierSet := map[int]bool{}
	for _, t := range s.Tiers {
		tierSet[t] = true
	}

	var out []Company
	for _, c := range r.Companies {
		if len(s.Groups) > 0 && !categoryWanted[c.Group] && !aliasSlugs[c.Slug] {
			continue
		}
		if len(s.Tiers) > 0 && !tierSet[c.Tier] {
			continue
		}
		if len(s.Only) > 0 && !onlySet[c.Slug] {
			continue
		}
		if excludeSet[c.Slug] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// ParseScope builds a Scope from comma-separated flag/env strings.
func ParseScope(groups, tiers, only, exclude string) (Scope, error) {
	s := Scope{
		Groups:  splitCSV(groups),
		Only:    splitCSV(only),
		Exclude: splitCSV(exclude),
	}
	for _, t := range splitCSV(tiers) {
		n, err := strconv.Atoi(t)
		if err != nil {
			return Scope{}, fmt.Errorf("registry: bad tier %q: %w", t, err)
		}
		s.Tiers = append(s.Tiers, n)
	}
	return s, nil
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NormalizeName reduces a company name to lowercase alphanumerics (dropping any
// parenthetical qualifier), so registry names and aggregator company_name values
// compare loosely — "Figure (robotics)" and "Figure" both become "figure".
func NormalizeName(s string) string {
	// Cut at the first "(" or "/" so "Point72 / Cubist" and "Figure (robotics)"
	// reduce to their leading name.
	if i := strings.IndexAny(s, "(/"); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// KnownNames returns the normalized set of every registry company name (used by
// the aggregator discovery log to spot untracked companies).
func (r *Registry) KnownNames() map[string]bool {
	m := make(map[string]bool, len(r.Companies))
	for _, c := range r.Companies {
		if n := NormalizeName(c.Name); n != "" {
			m[n] = true
		}
	}
	return m
}

func strSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
