package plantinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	Key     string
	BaseURL string
	HTTP    *http.Client
}

func NewClient(key, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://perenual.com"
	}
	return &Client{Key: key, BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 8 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("key", c.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, redactKey(err.Error(), c.Key))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: некорректный ответ", ErrUnavailable)
	}
	return nil
}

func redactKey(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "***")
}

type searchResp struct {
	Data []struct {
		ID         int      `json:"id"`
		CommonName string   `json:"common_name"`
		Scientific []string `json:"scientific_name"`
		OtherName  []string `json:"other_name"`
		DefaultImg *image   `json:"default_image"`
	} `json:"data"`
}

type image struct {
	Regular   string `json:"regular_url"`
	Medium    string `json:"medium_url"`
	Small     string `json:"small_url"`
	Thumbnail string `json:"thumbnail"`
}

func (i *image) url() string {
	if i == nil {
		return ""
	}
	for _, u := range []string{i.Medium, i.Small, i.Thumbnail, i.Regular} {

		if strings.HasPrefix(u, "http") && !strings.Contains(u, "upgrade_access") {
			return u
		}
	}
	return ""
}

type detailsResp struct {
	CommonName any      `json:"common_name"`
	Scientific []string `json:"scientific_name"`
	DefaultImg *image   `json:"default_image"`
	Watering   any      `json:"watering"`
	Benchmark  struct {
		Value any `json:"value"`
		Unit  any `json:"unit"`
	} `json:"watering_general_benchmark"`
	Sunlight     any `json:"sunlight"`
	PruningMonth any `json:"pruning_month"`
	CareLevel    any `json:"care_level"`
}

const maxCandidates = 12

func (c *Client) Search(ctx context.Context, query string) ([]Candidate, error) {
	var sr searchResp
	if err := c.get(ctx, "/api/v2/species-list", url.Values{"q": {query}}, &sr); err != nil {
		return nil, err
	}
	var with, without []Candidate
	for _, d := range sr.Data {
		cd := Candidate{ID: d.ID, CommonName: strings.TrimSpace(d.CommonName), OtherNames: d.OtherName,
			ImageURL: d.DefaultImg.url(), DetailsAvailable: d.ID >= 1 && d.ID <= 3000}
		if len(d.Scientific) > 0 {
			cd.Scientific = d.Scientific[0]
		}
		if cd.OtherNames == nil {
			cd.OtherNames = []string{}
		}
		if cd.DetailsAvailable {
			with = append(with, cd)
		} else {
			without = append(without, cd)
		}
	}
	out := append(with, without...)
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out, nil
}

func (c *Client) Details(ctx context.Context, id int) (*Facts, error) {
	var dr detailsResp
	if err := c.get(ctx, "/api/v2/species/details/"+strconv.Itoa(id), url.Values{}, &dr); err != nil {
		if err == ErrRateLimited {
			return nil, err
		}
		return &Facts{SpeciesID: id, Restricted: true}, nil
	}
	f := &Facts{SpeciesID: id, CommonName: cleanString(dr.CommonName), ImageURL: dr.DefaultImg.url()}
	if len(dr.Scientific) > 0 {
		f.Scientific = dr.Scientific[0]
	}
	f.Watering = normalizeWatering(dr.Watering)
	f.WateringMinDays, f.WateringMaxDays = parseBenchmark(dr.Benchmark.Value, dr.Benchmark.Unit)
	f.Sunlight = stringList(dr.Sunlight)
	f.PruningMonths = monthNumbers(stringList(dr.PruningMonth))
	f.CareLevel = cleanString(dr.CareLevel)
	return f, nil
}

func restricted(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "upgrade plans") || strings.Contains(l, "subscription-api-pricing")
}

func cleanString(v any) string {
	s, ok := v.(string)
	if !ok || restricted(s) {
		return ""
	}
	return strings.TrimSpace(s)
}

func normalizeWatering(v any) string {
	switch strings.ToLower(cleanString(v)) {
	case "frequent":
		return "Frequent"
	case "average":
		return "Average"
	case "minimum":
		return "Minimum"
	case "none":
		return "None"
	}
	return ""
}

func stringList(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, x := range arr {
		if s := cleanString(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}

var rangeRe = regexp.MustCompile(`(\d+)\D+(\d+)|(\d+)`)

func parseBenchmark(value, unit any) (minDays, maxDays int) {
	u := strings.ToLower(cleanString(unit))
	if u != "" && !strings.HasPrefix(u, "day") {
		return 0, 0
	}
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case float64:
		s = strconv.Itoa(int(v))
	default:
		return 0, 0
	}
	if restricted(s) {
		return 0, 0
	}
	m := rangeRe.FindStringSubmatch(s)
	switch {
	case m == nil:
		return 0, 0
	case m[1] != "":
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a > b {
			a, b = b, a
		}
		return a, b
	default:
		a, _ := strconv.Atoi(m[3])
		return a, a
	}
}

var monthIdx = map[string]int{"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
	"july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12}

func monthNumbers(names []string) []int {
	var out []int
	seen := map[int]bool{}
	for _, n := range names {
		if m := monthIdx[strings.ToLower(strings.TrimSpace(n))]; m > 0 && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}
