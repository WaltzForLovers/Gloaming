package catalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

const (
	searchLimit    = 10
	sourceTVMaze   = "tvmaze"
	sourceWikidata = "wikidata"
)

type Card struct {
	ID             int64
	ExternalID     int
	Name           string
	Year           int
	EpisodeCount   int
	AverageMinutes int
	Kind           string
	Poster         string
}

type Show struct {
	ID             int
	Source         string
	Name           string
	Year           int
	AverageRuntime int
	Runtime        int
	Poster         string
}

type FilmClient interface {
	SearchFilms(ctx context.Context, query string) ([]Show, error)
}

type Episode struct {
	Runtime int
}

type Client interface {
	SearchShows(ctx context.Context, query string) ([]Show, error)
	Episodes(ctx context.Context, showID int) ([]Episode, error)
}

type Service struct {
	db      *storage.DB
	client  Client
	films   FilmClient
	timeout time.Duration
}

func New(db *storage.DB, client Client) *Service {
	return &Service{db: db, client: client, timeout: 8 * time.Second}
}

func (s *Service) WithFilms(films FilmClient) *Service {
	s.films = films
	return s
}

func KindOf(episodeCount int) string {
	switch {
	case episodeCount == 1:
		return "film"
	case episodeCount > 1:
		return "series"
	default:
		return ""
	}
}

func BuildCard(show Show, episodes []Episode) Card {
	average := show.AverageRuntime
	if average <= 0 {
		average = show.Runtime
	}
	if average <= 0 {
		average = meanRuntime(episodes)
	}
	return Card{
		ExternalID:     show.ID,
		Name:           show.Name,
		Year:           show.Year,
		EpisodeCount:   len(episodes),
		AverageMinutes: average,
		Kind:           KindOf(len(episodes)),
		Poster:         show.Poster,
	}
}

func (s *Service) Search(ctx context.Context, query string) ([]Card, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < 2 {
		return nil, apperr.Validation("Введите не меньше двух символов.")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	type found struct {
		shows []Show
		err   error
	}
	filmCh := make(chan found, 1)
	go func() {
		if s.films == nil {
			filmCh <- found{}
			return
		}
		shows, err := s.films.SearchFilms(ctx, query)
		filmCh <- found{shows, err}
	}()

	shows, showErr := s.client.SearchShows(ctx, query)
	films := <-filmCh
	if showErr != nil && (films.err != nil || len(films.shows) == 0) {
		return nil, apperr.CatalogUnavailable().WithCause(showErr)
	}
	var episodes [][]Episode
	if showErr != nil {
		shows = nil
	} else {
		var err error
		episodes, err = s.fetchEpisodes(ctx, shows)
		if err != nil && len(films.shows) == 0 {
			return nil, apperr.CatalogUnavailable().WithCause(err)
		}
		if err != nil {
			shows = nil
			episodes = nil
		}
	}
	if films.err != nil {
		if len(shows) == 0 {
			return nil, apperr.CatalogUnavailable().WithCause(films.err)
		}
		films.shows = nil
	}
	merged := mergeResults(shows, episodes, films.shows)
	if len(merged) == 0 {
		return nil, apperr.TitleNotFound()
	}

	cards := make([]Card, 0, len(merged))
	for _, item := range merged {
		var episodes []Episode
		if item.show.Source == sourceWikidata {
			episodes = []Episode{{Runtime: item.show.Runtime}}
		} else {
			episodes = item.episodes
		}
		card := BuildCard(item.show, episodes)
		source := item.show.Source
		if source == "" {
			source = sourceTVMaze
		}
		saved, err := s.db.UpsertTitle(ctx, storage.Title{
			Source:         source,
			ExternalID:     card.ExternalID,
			Name:           card.Name,
			Year:           card.Year,
			EpisodeCount:   card.EpisodeCount,
			AverageMinutes: card.AverageMinutes,
			Poster:         card.Poster,
		})
		if err != nil {
			return nil, apperr.Internal(err)
		}
		card.ID = saved.ID
		cards = append(cards, card)
	}
	return cards, nil
}

type mergedShow struct {
	show     Show
	episodes []Episode
}

func (s *Service) fetchEpisodes(ctx context.Context, shows []Show) ([][]Episode, error) {
	episodes := make([][]Episode, len(shows))
	errs := make([]error, len(shows))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, show := range shows {
		wg.Add(1)
		go func(i, showID int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			episodes[i], errs[i] = s.client.Episodes(ctx, showID)
		}(i, show.ID)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return episodes, nil
}

func mergeResults(shows []Show, episodes [][]Episode, films []Show) []mergedShow {
	if len(shows) > 0 && len(films) > 5 {
		films = films[:5]
	}
	if len(films) > searchLimit {
		films = films[:searchLimit]
	}
	room := searchLimit - len(films)
	if len(shows) > room {
		shows = shows[:room]
	}
	out := make([]mergedShow, 0, len(shows)+len(films))
	for i, show := range shows {
		if show.Source == "" {
			show.Source = sourceTVMaze
		}
		var eps []Episode
		if i < len(episodes) {
			eps = episodes[i]
		}
		out = append(out, mergedShow{show: show, episodes: eps})
	}
	for _, film := range films {
		film.Source = sourceWikidata
		out = append(out, mergedShow{show: film})
	}
	return out
}

func (s *Service) Get(ctx context.Context, id int64) (Card, error) {
	title, err := s.db.TitleByID(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return Card{}, apperr.TitleNotFound()
	}
	if err != nil {
		return Card{}, apperr.Internal(err)
	}
	return Card{
		ID:             title.ID,
		ExternalID:     title.ExternalID,
		Name:           title.Name,
		Year:           title.Year,
		EpisodeCount:   title.EpisodeCount,
		AverageMinutes: title.AverageMinutes,
		Kind:           KindOf(title.EpisodeCount),
		Poster:         title.Poster,
	}, nil
}

func meanRuntime(episodes []Episode) int {
	sum, n := 0, 0
	for _, episode := range episodes {
		if episode.Runtime <= 0 {
			continue
		}
		sum += episode.Runtime
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}
