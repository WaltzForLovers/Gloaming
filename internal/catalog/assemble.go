package catalog

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/events"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

type catalogJob struct {
	id        int64
	wikiTitle string
}

type posterLookup interface {
	Poster(ctx context.Context, title string) (string, error)
}

func (s *Service) StartAssembler() {
	if s.jobs != nil {
		return
	}
	s.jobs = make(chan catalogJob, 32)
	go func() {
		for job := range s.jobs {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			_ = s.finishCard(ctx, job.id, job.wikiTitle)
			cancel()
		}
	}()
}

func (s *Service) enqueue(id int64, wikiTitle string) {
	if id == 0 {
		return
	}
	if s.pub != nil {
		body, err := events.MarshalCard(id, wikiTitle)
		if err == nil {
			if err := s.pub.Publish(context.Background(), events.TopicCard, []byte(strconv.FormatInt(id, 10)), body); err == nil {
				return
			}
		}
	}
	if s.jobs == nil {
		return
	}
	select {
	case s.jobs <- catalogJob{id: id, wikiTitle: wikiTitle}:
	default:
	}
}

func (s *Service) finishCard(ctx context.Context, id int64, wikiTitle string) error {
	if err := s.Assemble(ctx, id, wikiTitle); err != nil {
		return err
	}
	return s.keepPoster(ctx, id)
}

func (s *Service) keepPoster(ctx context.Context, id int64) error {
	if s.posters == nil {
		return nil
	}
	title, err := s.db.TitleByID(ctx, id)
	if err != nil || !strings.HasPrefix(title.Poster, "http") {
		return err
	}
	local, err := s.posters.SaveRemote(ctx, id, title.Poster)
	if err != nil || local == "" || local == title.Poster {
		return err
	}
	return s.db.KeepPoster(ctx, id, local)
}

func (s *Service) Assemble(ctx context.Context, id int64, wikiTitle string) error {
	title, err := s.db.TitleByID(ctx, id)
	if err != nil {
		return err
	}
	if title.Source == sourceTVMaze && s.client != nil {
		seasons, err := s.db.TitleSeasons(ctx, id, 0)
		if err != nil {
			return err
		}
		if len(seasons) == 0 {
			episodes, err := s.client.Episodes(ctx, title.ExternalID)
			if err == nil && len(episodes) > 0 {
				show := Show{
					ID: title.ExternalID, Source: title.Source, Name: title.Name,
					Year: title.Year, AverageRuntime: title.AverageMinutes, Poster: title.Poster,
				}
				card := BuildCard(show, episodes)
				if _, err := s.db.SaveCatalog(ctx, storage.Title{
					Source: title.Source, ExternalID: title.ExternalID, Name: card.Name, Year: card.Year,
					EpisodeCount: card.EpisodeCount, AverageMinutes: card.AverageMinutes, Poster: card.Poster,
				}, seedsOf(sourceTVMaze, show, episodes), false); err != nil {
					return err
				}
				title.Poster = card.Poster
			}
		}
	}
	if wikiTitle != "" && title.Poster == "" {
		posters, ok := s.films.(posterLookup)
		if !ok {
			return nil
		}
		poster, err := posters.Poster(ctx, wikiTitle)
		if err != nil || poster == "" {
			return err
		}
		return s.db.SetPoster(ctx, id, poster)
	}
	return nil
}
