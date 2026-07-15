package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Oracle scrapes an Oracle Recruiting Cloud (ORC) tenant — the platform behind
// JPMorgan and other banks. One implementation serves every ORC company via the
// registry's per-company orc:{tenant, site} block. Uses keyword=internship
// (like Workday, "intern" is far noisier).
type Oracle struct{}

func (Oracle) Source() string { return "oracle" }

const (
	orcPageSize = 200
	orcMaxPages = 30
	orcKeyword  = "internship"
)

type orcResponse struct {
	Items []struct {
		TotalJobsCount  int             `json:"TotalJobsCount"`
		RequisitionList []models.ORCJob `json:"requisitionList"`
	} `json:"items"`
}

func (Oracle) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	if c.ORC == nil || c.ORC.Tenant == "" {
		return nil, fmt.Errorf("oracle: %s missing orc {tenant,site} config", c.Name)
	}
	o := c.ORC
	var jobs []models.Job
	for page := 0; page < orcMaxPages; page++ {
		// expand=requisitionList is required — without it ORC returns only search
		// metadata and an empty requisitionList.
		url := fmt.Sprintf(
			"https://%s.fa.oraclecloud.com/hcmRestApi/resources/latest/recruitingCEJobRequisitions"+
				"?onlyData=true&expand=requisitionList.secondaryLocations"+
				"&finder=findReqs;siteNumber=%s,keyword=%s,limit=%d,offset=%d",
			o.Tenant, o.Site, orcKeyword, orcPageSize, page*orcPageSize)
		var resp orcResponse
		if err := getJSON(ctx, client, url, &resp); err != nil {
			return nil, err
		}
		if len(resp.Items) == 0 {
			break
		}
		item := resp.Items[0]
		for _, j := range item.RequisitionList {
			job := j.ToJob(o.Tenant, o.Site)
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(item.RequisitionList) < orcPageSize || len(jobs) >= item.TotalJobsCount {
			break
		}
	}
	return jobs, nil
}
