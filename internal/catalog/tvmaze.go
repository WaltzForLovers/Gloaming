package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type HTTPClient struct {
	baseURL string
	http    *http.Client
	cache   Cache
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{},
	}
}

func (c *HTTPClient) WithCache(cache Cache) *HTTPClient {
	c.cache = cache
	return c
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
		Name    string `json:"name"`
		Season  int    `json:"season"`
		Number  int    `json:"number"`
		Runtime *int   `json:"runtime"`
	}
	if err := c.get(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	episodes := make([]Episode, len(payload))
	for i, item := range payload {
		episodes[i].Name = item.Name
		episodes[i].Season = item.Season
		episodes[i].Number = item.Number
		if item.Runtime != nil {
			episodes[i].Runtime = *item.Runtime
		}
	}
	return episodes, nil
}

func (c *HTTPClient) get(ctx context.Context, endpoint string, dest any) error {
	body, status, err := loadBody(ctx, c.http, c.cache, "Gloaming/1.0", endpoint)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("catalog status %d", status)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("catalog json: %w", err)
	}
	return nil
}

type showJSON struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	Premiered      *string  `json:"premiered"`
	Runtime        *int     `json:"runtime"`
	AverageRuntime *int     `json:"averageRuntime"`
	Genres         []string `json:"genres"`
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
	show.Genres = s.Genres
	return show
}

func (c *HTTPClient) Genres(ctx context.Context, showID int) ([]string, error) {
	var payload struct {
		Genres []string `json:"genres"`
	}
	endpoint := fmt.Sprintf("%s/shows/%d", c.baseURL, showID)
	if err := c.get(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	return payload.Genres, nil
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
