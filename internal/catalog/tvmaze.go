package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type HTTPClient struct {
	baseURL string
	http    *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{},
	}
}

func (c *HTTPClient) SearchShows(ctx context.Context, query string) ([]Show, error) {
	endpoint := c.baseURL + "/search/shows?q=" + url.QueryEscape(query)
	var payload []struct {
		Show showJSON `json:"show"`
	}
	if err := c.get(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	shows := make([]Show, 0, len(payload))
	for _, item := range payload {
		show := item.Show.toShow()
		if show.ID <= 0 {
			continue
		}
		shows = append(shows, show)
	}
	return shows, nil
}

func (c *HTTPClient) Episodes(ctx context.Context, showID int) ([]Episode, error) {
	endpoint := fmt.Sprintf("%s/shows/%d/episodes", c.baseURL, showID)
	var payload []struct {
		Runtime *int `json:"runtime"`
	}
	if err := c.get(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	episodes := make([]Episode, len(payload))
	for i, item := range payload {
		if item.Runtime != nil {
			episodes[i].Runtime = *item.Runtime
		}
	}
	return episodes, nil
}

func (c *HTTPClient) get(ctx context.Context, endpoint string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Gloaming/1.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("catalog status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("catalog json: %w", err)
	}
	return nil
}

type showJSON struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	Premiered      *string `json:"premiered"`
	Runtime        *int    `json:"runtime"`
	AverageRuntime *int    `json:"averageRuntime"`
	Image          *struct {
		Medium string `json:"medium"`
	} `json:"image"`
}

func (s showJSON) toShow() Show {
	show := Show{ID: s.ID, Name: s.Name}
	if s.Premiered != nil {
		show.Year = yearOf(*s.Premiered)
	}
	if s.Runtime != nil {
		show.Runtime = *s.Runtime
	}
	if s.AverageRuntime != nil {
		show.AverageRuntime = *s.AverageRuntime
	}
	if s.Image != nil {
		show.Poster = s.Image.Medium
	}
	return show
}

func yearOf(premiered string) int {
	if len(premiered) < 4 {
		return 0
	}
	year, err := strconv.Atoi(premiered[:4])
	if err != nil {
		return 0
	}
	return year
}
