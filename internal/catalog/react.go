package catalog

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/events"
	"github.com/WaltzForLovers/Gloaming/internal/follow"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

type genreSource interface {
	Genres(ctx context.Context, showID int) ([]string, error)
}

func (s *Service) afterWatch(ctx context.Context, userID, episodeID int64, on bool) {
	if s.pub != nil {
		body, err := events.MarshalWatched(userID, episodeID, on)
		if err == nil {
			if err := s.pub.Publish(ctx, events.TopicWatched, []byte(strconv.FormatInt(userID, 10)), body); err == nil {
				return
			}
		}
	}
	_ = s.ApplyWatched(ctx, userID, time.Now())
}

func (s *Service) ApplyWatched(ctx context.Context, userID int64, now time.Time) error {
	s.fillGenres(ctx, userID)
	if err := follow.Apply(ctx, s.db, userID, now); err != nil {
		return err
	}
	s.copyPosters(ctx, userID)
	s.sendLetter(ctx, userID)
	return nil
}

func (s *Service) sendLetter(ctx context.Context, userID int64) {
	if s.mail == nil {
		return
	}
	email, err := s.db.UserEmail(ctx, userID)
	if err != nil || email == "" {
		return
	}
	body, ok, err := s.db.UnsentLetter(ctx, userID, follow.LetterThursday)
	if err != nil || !ok {
		return
	}
	if err := s.mail.Send(ctx, email, "Сезон кончается в четверг", body+"\n"); err != nil {
		return
	}
	_ = s.db.MarkLetterSent(ctx, userID, follow.LetterThursday)
}

func (s *Service) copyPosters(ctx context.Context, userID int64) {
	if s.posters == nil {
		return
	}
	queued, err := s.db.QueuedTitles(ctx, userID)
	if err != nil {
		return
	}
	for _, title := range queued {
		_ = s.keepPoster(ctx, title.ID)
	}
	picks, err := s.db.Suggestions(ctx, userID)
	if err != nil {
		return
	}
	for _, title := range picks {
		_ = s.keepPoster(ctx, title.ID)
	}
}

func (s *Service) HandleWatched(ctx context.Context, value []byte) error {
	var event events.Watched
	if err := json.Unmarshal(value, &event); err != nil {
		return err
	}
	if event.UserID == 0 {
		return nil
	}
	return s.ApplyWatched(ctx, event.UserID, time.Now())
}

func (s *Service) HandleCard(ctx context.Context, value []byte) error {
	var event events.Card
	if err := json.Unmarshal(value, &event); err != nil {
		return err
	}
	return s.finishCard(ctx, event.TitleID, event.WikiTitle)
}

func (s *Service) Suggestions(ctx context.Context, userID int64) ([]Card, error) {
	titles, err := s.db.Suggestions(ctx, userID)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, len(titles))
	for i, title := range titles {
		cards[i] = Card{
			ID: title.ID, ExternalID: title.ExternalID, Name: title.Name, Year: title.Year,
			EpisodeCount: title.EpisodeCount, AverageMinutes: title.AverageMinutes,
			Kind: KindOf(title.EpisodeCount), Poster: title.Poster,
		}
	}
	return cards, nil
}

func (s *Service) Letter(ctx context.Context, userID int64) (string, error) {
	return s.db.Letter(ctx, userID, follow.LetterThursday)
}

func (s *Service) fillGenres(ctx context.Context, userID int64) {
	source, ok := s.client.(genreSource)
	if !ok || source == nil {
		return
	}
	queued, err := s.db.QueuedTitles(ctx, userID)
	if err != nil {
		return
	}
	var pending []storage.Title
	for _, title := range queued {
		if title.Source == sourceTVMaze && len(title.Genres) == 0 {
			pending = append(pending, title)
		}
	}
	if len(pending) < 8 {
		extra, err := s.db.CatalogWithoutGenres(ctx, 8)
		if err == nil {
			seen := map[int64]bool{}
			for _, title := range pending {
				seen[title.ID] = true
			}
			for _, title := range extra {
				if seen[title.ID] || len(pending) >= 8 {
					continue
				}
				pending = append(pending, title)
			}
		}
	}
	for _, title := range pending {
		gctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		genres, err := source.Genres(gctx, title.ExternalID)
		cancel()
		if err != nil || len(genres) == 0 {
			continue
		}
		_ = s.db.SetGenres(ctx, title.ID, genres)
	}
}
