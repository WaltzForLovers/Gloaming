package catalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/mail"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

const (
	searchLimit    = 10
	sourceTVMaze   = "tvmaze"
	sourceWikidata = "wikidata"
	sourceManual   = "manual"
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
	Assembling     bool
}

type Show struct {
	ID             int
	Source         string
	Name           string
	Year           int
	AverageRuntime int
	Runtime        int
	Poster         string
	WikiTitle      string
	Genres         []string
}

type FilmClient interface {
	SearchFilms(ctx context.Context, query string) ([]Show, error)
}

type Episode struct {
	Season  int
	Number  int
	Name    string
	Runtime int
}

type EpisodeCard struct {
	ID      int64
	Number  int
	Name    string
	Runtime int
	Watched bool
}

type SeasonCard struct {
	ID       int64
	Number   int
	Episodes []EpisodeCard
}

type Client interface {
	SearchShows(ctx context.Context, query string) ([]Show, error)
	Episodes(ctx context.Context, showID int) ([]Episode, error)
}

type Service struct {
	db            *storage.DB
	client        Client
	films         FilmClient
	timeout       time.Duration
	grace         time.Duration
	episodeBudget time.Duration
	jobs          chan catalogJob
	pub           Publisher
	posters       posterStore
	mail          mail.Sender
}

type Publisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

type posterStore interface {
	SaveRemote(ctx context.Context, titleID int64, rawURL string) (string, error)
}

func New(db *storage.DB, client Client) *Service {
	return &Service{
		db: db, client: client,
		timeout: 8 * time.Second, grace: 1500 * time.Millisecond, episodeBudget: 1500 * time.Millisecond,
	}
}

func (s *Service) UsePublisher(pub Publisher) *Service {
	s.pub = pub
	return s
}

func (s *Service) UsePosters(store posterStore) *Service {
	s.posters = store
	return s
}

func (s *Service) UseMail(sender mail.Sender) *Service {
	s.mail = sender
	return s
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
	catalogCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	type found struct {
		shows []Show
		err   error
	}
	filmCh := make(chan found, 1)
	showCh := make(chan found, 1)
	go func() {
		if s.films == nil {
			filmCh <- found{}
			return
		}
		shows, err := s.films.SearchFilms(catalogCtx, query)
		filmCh <- found{shows, err}
	}()
	go func() {
		shows, err := s.client.SearchShows(catalogCtx, query)
		showCh <- found{shows, err}
	}()

	var showsFound, filmsFound found
	var showGot, filmGot bool
	var grace <-chan time.Time
	armGrace := func() {
		if grace != nil || s.grace <= 0 || showGot == filmGot {
			return
		}
		useful := false
		if showGot {
			useful = showsFound.err == nil && len(showsFound.shows) > 0
		} else {
			useful = filmsFound.err == nil && len(filmsFound.shows) > 0
		}
		if useful {
			grace = time.After(s.grace)
		}
	}
	for !(showGot && filmGot) {
		select {
		case filmsFound = <-filmCh:
			filmGot = true
			armGrace()
		case showsFound = <-showCh:
			showGot = true
			armGrace()
		case <-grace:
			cancel()
			goto answered
		case <-catalogCtx.Done():
			goto answered
		}
	}
answered:
	if !showGot {
		select {
		case showsFound = <-showCh:
			showGot = true
		default:
			showsFound.err = context.DeadlineExceeded
		}
	}
	if !filmGot {
		select {
		case filmsFound = <-filmCh:
		default:
			filmsFound.err = context.DeadlineExceeded
		}
	}
	shows, showErr := showsFound.shows, showsFound.err
	if showErr != nil && (filmsFound.err != nil || len(filmsFound.shows) == 0) {
		return nil, apperr.CatalogUnavailable().WithCause(showErr)
	}
	var episodes [][]Episode
	var epErrs []error
	if showErr != nil {
		shows = nil
	} else if len(shows) > 0 {
		budget := s.episodeBudget
		if budget <= 0 {
			budget = s.timeout
		}
		epCtx, epCancel := context.WithTimeout(ctx, budget)
		episodes, epErrs = s.fetchEpisodes(epCtx, shows)
		epCancel()
	}
	if filmsFound.err != nil {
		if len(shows) == 0 {
			return nil, apperr.CatalogUnavailable().WithCause(filmsFound.err)
		}
		filmsFound.shows = nil
	}
	merged := mergeResults(shows, episodes, epErrs, filmsFound.shows)
	if len(merged) == 0 {
		return nil, apperr.TitleNotFound()
	}

	cards := make([]Card, 0, len(merged))
	for _, item := range merged {
		episodes := item.episodes
		if item.show.Source == sourceWikidata {
			episodes = []Episode{{Runtime: item.show.Runtime}}
		}
		card := BuildCard(item.show, episodes)
		source := item.show.Source
		if source == "" {
			source = sourceTVMaze
		}
		var seeds []storage.EpisodeSeed
		if !item.slow {
			seeds = seedsOf(source, item.show, episodes)
		}
		saved, err := s.db.SaveCatalog(ctx, storage.Title{
			Source:         source,
			ExternalID:     card.ExternalID,
			Name:           card.Name,
			Year:           card.Year,
			EpisodeCount:   card.EpisodeCount,
			AverageMinutes: card.AverageMinutes,
			Poster:         card.Poster,
			Genres:         item.show.Genres,
		}, seeds, item.slow)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		card.ID = saved.ID
		card.EpisodeCount = saved.EpisodeCount
		card.AverageMinutes = saved.AverageMinutes
		card.Kind = KindOf(saved.EpisodeCount)
		card.Poster = saved.Poster
		card.Assembling = saved.Assembling
		if saved.Assembling || (item.show.WikiTitle != "" && saved.Poster == "") {
			s.enqueue(saved.ID, item.show.WikiTitle)
		}
		cards = append(cards, card)
	}
	return cards, nil
}

type mergedShow struct {
	show     Show
	episodes []Episode
	slow     bool
}

func (s *Service) fetchEpisodes(ctx context.Context, shows []Show) ([][]Episode, []error) {
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
	return episodes, errs
}

func mergeResults(shows []Show, episodes [][]Episode, errs []error, films []Show) []mergedShow {
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
		slow := i < len(errs) && errs[i] != nil
		if !slow && i < len(episodes) {
			eps = episodes[i]
		}
		out = append(out, mergedShow{show: show, episodes: eps, slow: slow})
	}
	for _, film := range films {
		film.Source = sourceWikidata
		out = append(out, mergedShow{show: film})
	}
	return out
}

func seedsOf(source string, show Show, episodes []Episode) []storage.EpisodeSeed {
	if source == sourceWikidata {
		runtime := show.Runtime
		if runtime <= 0 {
			runtime = show.AverageRuntime
		}
		return []storage.EpisodeSeed{{SeasonNumber: 1, Number: 1, Name: show.Name, Runtime: runtime}}
	}
	if len(episodes) == 0 {
		return nil
	}
	seeds := make([]storage.EpisodeSeed, len(episodes))
	for i, episode := range episodes {
		season := episode.Season
		number := episode.Number
		if season < 1 {
			season = 1
		}
		if number < 1 {
			number = i + 1
		}
		seeds[i] = storage.EpisodeSeed{SeasonNumber: season, Number: number, Name: episode.Name, Runtime: episode.Runtime}
	}
	return seeds
}

func (s *Service) Detail(ctx context.Context, userID, id int64) (Card, []SeasonCard, error) {
	card, err := s.Get(ctx, id)
	if err != nil {
		return Card{}, nil, err
	}
	title, err := s.db.TitleByID(ctx, id)
	if err != nil {
		return Card{}, nil, apperr.Internal(err)
	}
	if title.OwnerID != 0 && title.OwnerID != userID {
		return Card{}, nil, apperr.TitleNotFound()
	}
	seasons, err := s.db.TitleSeasons(ctx, id, userID)
	if err != nil {
		return Card{}, nil, apperr.Internal(err)
	}
	if title.Source == sourceTVMaze && len(seasons) == 0 {
		card.Assembling = true
		s.enqueue(id, "")
	}
	out := make([]SeasonCard, len(seasons))
	for i, season := range seasons {
		out[i] = SeasonCard{ID: season.ID, Number: season.Number, Episodes: make([]EpisodeCard, len(season.Episodes))}
		for j, episode := range season.Episodes {
			out[i].Episodes[j] = EpisodeCard{
				ID: episode.ID, Number: episode.Number, Name: episode.Name, Runtime: episode.Runtime, Watched: episode.Watched,
			}
		}
	}
	return card, out, nil
}

func (s *Service) SetWatched(ctx context.Context, userID, episodeID int64, on bool) error {
	err := s.db.SetWatched(ctx, userID, episodeID, on)
	if errors.Is(err, storage.ErrNotFound) {
		return apperr.NotFound()
	}
	if err != nil {
		return apperr.Internal(err)
	}
	s.afterWatch(ctx, userID, episodeID, on)
	return nil
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
		Assembling:     title.Assembling,
	}, nil
}

func (s *Service) AddManual(ctx context.Context, userID int64, name string, year, episodes, average int) (Card, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 2 || n > 120 {
		return Card{}, apperr.Validation("Название: от 2 до 120 символов.")
	}
	if year < 1888 || year > 2100 {
		return Card{}, apperr.Validation("Год: от 1888 до 2100.")
	}
	if episodes < 1 || episodes > 500 {
		return Card{}, apperr.Validation("Число серий: от 1 до 500.")
	}
	if average < 1 || average > 1440 {
		return Card{}, apperr.Validation("Длительность серии: от 1 до 1440 минут.")
	}
	saved, err := s.db.CreateManual(ctx, userID, storage.Title{
		Name: name, Year: year, EpisodeCount: episodes, AverageMinutes: average, Source: sourceManual,
	})
	if err != nil {
		return Card{}, apperr.Internal(err)
	}
	return cardFrom(saved), nil
}

func (s *Service) FindManual(ctx context.Context, userID int64, query string) ([]Card, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < 2 {
		return nil, nil
	}
	titles, err := s.db.ManualTitles(ctx, userID, query)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	cards := make([]Card, len(titles))
	for i, title := range titles {
		cards[i] = cardFrom(title)
	}
	return cards, nil
}

func MergeCards(owned, found []Card) []Card {
	out := make([]Card, 0, len(owned)+len(found))
	seen := map[int64]bool{}
	for _, list := range [][]Card{owned, found} {
		for _, card := range list {
			if card.ID == 0 || seen[card.ID] {
				continue
			}
			seen[card.ID] = true
			out = append(out, card)
			if len(out) == searchLimit {
				return out
			}
		}
	}
	return out
}

func cardFrom(title storage.Title) Card {
	return Card{
		ID:             title.ID,
		ExternalID:     title.ExternalID,
		Name:           title.Name,
		Year:           title.Year,
		EpisodeCount:   title.EpisodeCount,
		AverageMinutes: title.AverageMinutes,
		Kind:           KindOf(title.EpisodeCount),
		Poster:         title.Poster,
		Assembling:     title.Assembling,
	}
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
