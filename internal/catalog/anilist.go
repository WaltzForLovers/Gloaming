package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const animeSearchLimit = 8

const animeQuery = `query ($search: String, $perPage: Int) {
  Page(perPage: $perPage) {
    media(search: $search, type: ANIME, isAdult: false, sort: SEARCH_MATCH) {
      id
      format
      episodes
      duration
      startDate { year }
      title { romaji english }
      coverImage { large }
      genres
      averageScore
      description(asHtml: false)
      nextAiringEpisode { episode }
    }
  }
}`

var animeGenres = map[string]string{
	"Sci-Fi": "Science-Fiction",
}

type AniList struct {
	endpoint string
	http     *http.Client
	cache    Cache
}

func NewAniList(endpoint string) *AniList {
	return &AniList{endpoint: endpoint, http: &http.Client{}}
}

func (c *AniList) WithCache(cache Cache) *AniList {
	c.cache = cache
	return c
}

func (c *AniList) SearchAnime(ctx context.Context, query string) ([]Show, error) {
	payload, err := json.Marshal(map[string]any{
		"query":     animeQuery,
		"variables": map[string]any{"search": query, "perPage": animeSearchLimit},
	})
	if err != nil {
		return nil, err
	}
	body, status, err := postBody(ctx, c.http, c.cache, "Gloaming/1.0 (local watch queue)", c.endpoint, payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, errStatus(status)
	}
	var reply struct {
		Data struct {
			Page struct {
				Media []animeJSON `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, err
	}
	shows := make([]Show, 0, len(reply.Data.Page.Media))
	for _, media := range reply.Data.Page.Media {
		if show, ok := media.show(); ok {
			shows = append(shows, show)
		}
	}
	return shows, nil
}

type animeJSON struct {
	ID        int    `json:"id"`
	Format    string `json:"format"`
	Episodes  *int   `json:"episodes"`
	Duration  *int   `json:"duration"`
	StartDate struct {
		Year *int `json:"year"`
	} `json:"startDate"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	CoverImage struct {
		Large string `json:"large"`
	} `json:"coverImage"`
	Genres       []string `json:"genres"`
	AverageScore *int     `json:"averageScore"`
	Description  string   `json:"description"`
	Next         *struct {
		Episode int `json:"episode"`
	} `json:"nextAiringEpisode"`
}

func (m animeJSON) show() (Show, bool) {
	name := strings.TrimSpace(m.Title.English)
	if name == "" {
		name = strings.TrimSpace(m.Title.Romaji)
	}
	episodes := 0
	if m.Episodes != nil {
		episodes = *m.Episodes
	} else if m.Next != nil {
		episodes = m.Next.Episode - 1
	}
	if m.Format == "MOVIE" && episodes <= 0 {
		episodes = 1
	}
	if m.ID <= 0 || name == "" || episodes <= 0 {
		return Show{}, false
	}
	show := Show{
		ID:       m.ID,
		Source:   sourceAniList,
		Name:     name,
		Alias:    strings.TrimSpace(m.Title.Romaji),
		Episodes: episodes,
		Poster:   m.CoverImage.Large,
		Summary:  m.Description,
		Genres:   append(mapGenres(m.Genres), "Anime"),
	}
	if m.StartDate.Year != nil {
		show.Year = *m.StartDate.Year
	}
	if m.Duration != nil {
		show.Runtime = *m.Duration
	}
	if m.AverageScore != nil {
		show.Rating = float64(*m.AverageScore) / 10
	}
	return show, true
}

func mapGenres(genres []string) []string {
	out := make([]string, 0, len(genres)+1)
	for _, genre := range genres {
		if mapped, ok := animeGenres[genre]; ok {
			genre = mapped
		}
		if genre != "" && genre != "Anime" {
			out = append(out, genre)
		}
	}
	return out
}

func animeEpisodes(show Show) []Episode {
	episodes := make([]Episode, show.Episodes)
	for i := range episodes {
		episodes[i] = Episode{Season: 1, Number: i + 1, Runtime: show.Runtime, SeasonPoster: show.Poster}
	}
	if len(episodes) == 1 {
		episodes[0].Name = show.Name
	}
	return episodes
}
