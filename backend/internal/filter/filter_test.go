package filter

import (
	"testing"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

func TestKeep(t *testing.T) {
	cfg := Config{USOnly: true, AllowPhD: false}

	cases := []struct {
		name     string
		job      models.Job
		wantKeep bool
		wantCat  string
	}{
		{
			name:     "software eng intern",
			job:      models.Job{Position: "Software Engineer Intern", Region: "San Francisco, CA"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "swe co-op",
			job:      models.Job{Position: "SWE Co-op", Region: "Remote"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "ml intern beats software",
			job:      models.Job{Position: "Machine Learning Intern", Region: "New York"},
			wantKeep: true, wantCat: "ML/AI",
		},
		{
			name:     "quant trading intern",
			job:      models.Job{Position: "Quantitative Trading Intern", Region: "Chicago, IL"},
			wantKeep: true, wantCat: "Quant",
		},
		{
			name:     "data science intern",
			job:      models.Job{Position: "Data Science Intern", Region: "Austin, TX"},
			wantKeep: true, wantCat: "Data",
		},
		{
			name:     "ashby employmentType intern, title lacks keyword",
			job:      models.Job{Position: "Software Engineer, University Program", EmploymentType: "Intern", Region: "Seattle, WA"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "sales engineer intern dropped (negative wins)",
			job:      models.Job{Position: "Sales Engineer Intern", Region: "New York"},
			wantKeep: false,
		},
		{
			name:     "marketing intern dropped (not eng)",
			job:      models.Job{Position: "Marketing Intern", Region: "New York"},
			wantKeep: false,
		},
		{
			name:     "recruiting intern dropped",
			job:      models.Job{Position: "Technical Recruiting Intern", Region: "SF"},
			wantKeep: false,
		},
		{
			name:     "full time SWE dropped (not intern)",
			job:      models.Job{Position: "Senior Software Engineer", Region: "SF"},
			wantKeep: false,
		},
		{
			name:     "phd-only dropped by default",
			job:      models.Job{Position: "PhD Research Intern, Machine Learning", Region: "Seattle, WA"},
			wantKeep: false,
		},
		{
			name:     "non-US dropped",
			job:      models.Job{Position: "Software Engineer Intern", Region: "London", Country: "United Kingdom"},
			wantKeep: false,
		},
		{
			name:     "workday china in region text dropped",
			job:      models.Job{Position: "Software Engineering Intern, Neural Reconstruction", Region: "China, Shanghai"},
			wantKeep: false,
		},
		{
			name:     "US city colliding with foreign name kept (Paris, TX)",
			job:      models.Job{Position: "Software Engineer Intern", Region: "Paris, TX"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "workday US format kept (US, CA, Santa Clara)",
			job:      models.Job{Position: "Java Engineering Intern", Region: "US, CA, Santa Clara"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "unknown location kept",
			job:      models.Job{Position: "Backend Engineer Intern"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "remote non-structured kept",
			job:      models.Job{Position: "Frontend Engineering Intern", Modality: "Remote"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "security intern",
			job:      models.Job{Position: "Security Engineering Intern", Region: "Remote"},
			wantKeep: true, wantCat: "Security",
		},
		{
			name:     "embedded intern",
			job:      models.Job{Position: "Embedded Software Intern", Region: "Hawthorne, CA"},
			wantKeep: true, wantCat: "Embedded/Hardware",
		},
		{
			name:     "cybersecurity new grad (one word) classifies as Security",
			job:      models.Job{Position: "Cybersecurity Analyst: New Grad", Region: "New York, New York, United States"},
			wantKeep: true, wantCat: "Security",
		},
		{
			name:     "bank SWE summer analyst kept",
			job:      models.Job{Position: "Software Engineer Program - Summer Analyst", Region: "New York, NY, United States"},
			wantKeep: true, wantCat: "Software",
		},
		{
			name:     "markets summer analyst dropped (not eng)",
			job:      models.Job{Position: "2027 Markets Summer Analyst Program", Region: "New York, NY, United States"},
			wantKeep: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, keep := Keep(tc.job, cfg)
			if keep != tc.wantKeep {
				t.Fatalf("keep = %v, want %v (cat=%q)", keep, tc.wantKeep, cat)
			}
			if keep && cat != tc.wantCat {
				t.Fatalf("category = %q, want %q", cat, tc.wantCat)
			}
		})
	}
}

func TestPhDAllowed(t *testing.T) {
	cfg := Config{USOnly: true, AllowPhD: true}
	job := models.Job{Position: "PhD Research Intern, Machine Learning", Region: "Seattle, WA"}
	if cat, keep := Keep(job, cfg); !keep || cat != "ML/AI" {
		t.Fatalf("with AllowPhD: keep=%v cat=%q, want true ML/AI", keep, cat)
	}
}
