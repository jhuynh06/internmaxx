// Package filter turns a raw stream of postings into the SWE/engineering
// internships worth alerting on. It is pure functions over models.Job so the
// logic is unit-testable against real title fixtures (filter_test.go).
//
// A job is kept when it is (1) an internship, (2) an engineering role, (3) not
// a non-eng role that merely contains an eng word ("Sales Engineer"), and (4)
// in an allowed location. Keeping means Category is set to a human label.
package filter

import (
	"regexp"
	"strings"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

type Config struct {
	USOnly   bool // default true: US + Remote-US only
	AllowPhD bool // default false: drop PhD-only postings
}

var (
	// "summer analyst"/"summer associate" are the finance-industry terms for an
	// internship (banks). Role classification still gates on an eng term, so a
	// non-eng "Markets Summer Analyst" is dropped anyway.
	internRe = regexp.MustCompile(`(?i)\b(intern|interns|internship|co-?op|apprentice|apprenticeship|trainee|university\s+grad|new\s+grad|early\s+career|student|summer\s+analyst|summer\s+associate)\b`)

	// Negative role terms — if present the job is not a SWE/eng internship even
	// when it also contains an engineering word (e.g. "Sales Engineer").
	negativeRe = regexp.MustCompile(`(?i)\b(sales|account\s+executive|marketing|recruit(er|ing)|talent|people\s+ops|human\s+resources|\bhr\b|legal|counsel|paralegal|accounting|payroll|customer\s+success|customer\s+support|community|content|copywrit|social\s+media|brand|communications|public\s+relations|\bpr\b|partnerships|business\s+development|\bbdr\b|\bsdr\b|procurement|facilities|administrative|executive\s+assistant)\b`)

	phdRe = regexp.MustCompile(`(?i)\b(ph\.?d|doctoral|postdoc)\b`)

	// Category classifiers, checked in priority order; first match wins.
	categoryRules = []struct {
		name string
		re   *regexp.Regexp
	}{
		{"Quant", regexp.MustCompile(`(?i)\b(quant(itative)?|trading|trader)\b`)},
		{"ML/AI", regexp.MustCompile(`(?i)\b(machine\s*learning|\bml\b|\bai\b|artificial\s+intelligence|deep\s+learning|\bnlp\b|computer\s+vision|research\s+scientist|research\s+engineer|applied\s+scientist|\bllm\b|generative)\b`)},
		// No trailing \b: "scien"/"analy" are stems of science/analytics, so a
		// word boundary right after them would never match.
		{"Data", regexp.MustCompile(`(?i)\bdata\s+(engineer|scien|analy)|\banalytics\b`)},
		{"Security", regexp.MustCompile(`(?i)\b(cyber\s*security|cyber|security|appsec|infosec|cryptograph|vulnerability)\b`)},
		{"Infra/Platform", regexp.MustCompile(`(?i)\b(infrastructure|platform|devops|\bsre\b|site\s+reliability|cloud|systems|distributed|networking)\b`)},
		{"Embedded/Hardware", regexp.MustCompile(`(?i)\b(embedded|firmware|hardware|\bfpga\b|\basic\b|silicon|electrical|robotics|controls|mechatronics)\b`)},
		{"Mobile", regexp.MustCompile(`(?i)\b(mobile|\bios\b|android)\b`)},
		{"Software", regexp.MustCompile(`(?i)\b(software|swe\b|\bsde\b|back\s*end|front\s*end|full[\s-]*stack|web|developer|programmer|engineer(ing)?|compiler|graphics|game\s+program)\b`)},
	}
)

// IsInternship reports whether the posting is an internship/early-career role,
// using the title plus any ATS-native employment type ("Intern").
func IsInternship(j models.Job) bool {
	if internRe.MatchString(j.Position) {
		return true
	}
	if et := strings.ToLower(j.EmploymentType); strings.Contains(et, "intern") {
		return true
	}
	return false
}

// Classify returns a category label for engineering roles, or "" if the title
// is not an engineering role at all.
func Classify(j models.Job) string {
	title := j.Position
	for _, rule := range categoryRules {
		if rule.re.MatchString(title) {
			return rule.name
		}
	}
	return ""
}

func isNegativeRole(j models.Job) bool {
	return negativeRe.MatchString(j.Position)
}

func requiresPhD(j models.Job) bool {
	// Only judge from the title to avoid description false-positives ("no PhD
	// required"). Description-level parsing can come later.
	return phdRe.MatchString(j.Position)
}

func locationOK(j models.Job, cfg Config) bool {
	if !cfg.USOnly {
		return true
	}
	// Assemble the location text we have; empty means unknown → keep (many
	// startups omit structured location on the board).
	loc := strings.ToLower(strings.Join([]string{j.Region, j.Country, j.Modality}, " "))
	if strings.TrimSpace(loc) == "" {
		return true
	}
	// US signals win first, so a US city that collides with a foreign one
	// ("Paris, TX", "Remote - US") is kept before the non-US blocklist runs.
	if hasUSSignal(loc, j.Region) {
		return true
	}
	// Explicit non-US location (country/city in the text, common with Workday
	// which puts "China, Shanghai" in the location string) → reject.
	if nonUSRe.MatchString(loc) {
		return false
	}
	// Remote with no country and no non-US signal → assume US-eligible, keep.
	if strings.Contains(loc, "remote") {
		return true
	}
	// Country explicitly set and not US → reject.
	if c := strings.ToLower(strings.TrimSpace(j.Country)); c != "" {
		switch c {
		case "united states", "usa", "us", "u.s.", "u.s.a.", "america":
			return true
		default:
			return false
		}
	}
	// Unknown-but-nonempty and no non-US signal: keep rather than miss a role.
	return true
}

func hasUSSignal(loc, region string) bool {
	for _, s := range []string{"united states", "usa", " us ", ", us", "us,", "u.s", "america"} {
		if strings.Contains(loc, s) {
			return true
		}
	}
	return usStateRe.MatchString(region) || usCityRe.MatchString(region)
}

var (
	usStateRe = regexp.MustCompile(`(?i),\s*(AL|AK|AZ|AR|CA|CO|CT|DE|FL|GA|HI|ID|IL|IN|IA|KS|KY|LA|ME|MD|MA|MI|MN|MS|MO|MT|NE|NV|NH|NJ|NM|NY|NC|ND|OH|OK|OR|PA|RI|SC|SD|TN|TX|UT|VT|VA|WA|WV|WI|WY|DC)\b`)
	usCityRe  = regexp.MustCompile(`(?i)\b(new\s+york|san\s+francisco|seattle|austin|boston|chicago|los\s+angeles|mountain\s+view|palo\s+alto|sunnyvale|menlo\s+park|bellevue|redmond|cambridge|atlanta|denver|dallas|houston|san\s+jose|san\s+diego|washington)\b`)
	// Non-US countries/cities commonly seen in intern location strings.
	nonUSRe = regexp.MustCompile(`(?i)\b(china|shanghai|beijing|shenzhen|india|bangalore|bengaluru|hyderabad|pune|gurgaon|gurugram|noida|chennai|mumbai|new\s+delhi|canada|toronto|vancouver|montreal|ottawa|waterloo|ontario|united\s+kingdom|england|london|manchester|ireland|dublin|germany|berlin|munich|france|paris\s*,?\s*(fr|france)|netherlands|amsterdam|singapore|japan|tokyo|korea|seoul|israel|tel\s+aviv|australia|sydney|melbourne|brazil|mexico|poland|warsaw|krakow|romania|bucharest|spain|madrid|barcelona|portugal|lisbon|sweden|stockholm|switzerland|zurich|taiwan|taipei|hong\s+kong|philippines|manila|vietnam|malaysia|indonesia|thailand|bangkok|dubai|abu\s+dhabi|egypt|cairo|new\s+zealand|italy|milan|belgium|brussels|austria|vienna|denmark|copenhagen|norway|oslo|finland|helsinki|czech|prague|hungary|budapest|greece|athens|turkey|istanbul|scotland|edinburgh)\b`)
)

// Keep reports whether a single job should alert, and returns its category.
func Keep(j models.Job, cfg Config) (category string, keep bool) {
	if !IsInternship(j) {
		return "", false
	}
	cat := Classify(j)
	if cat == "" {
		return "", false
	}
	if isNegativeRole(j) {
		return "", false
	}
	if !cfg.AllowPhD && requiresPhD(j) {
		return "", false
	}
	if !locationOK(j, cfg) {
		return "", false
	}
	return cat, true
}

// Apply filters a slice and stamps Category on the kept jobs.
func Apply(jobs []models.Job, cfg Config) []models.Job {
	out := make([]models.Job, 0, len(jobs))
	for _, j := range jobs {
		if cat, ok := Keep(j, cfg); ok {
			j.Category = cat
			out = append(out, j)
		}
	}
	return out
}
