package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/plan"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

type Item struct {
	ID                int64
	Position          int
	TitleID           int64
	SeasonID          int64
	SeasonNumber      int
	Name              string
	Year              int
	EpisodeCount      int
	SeasonEpisodes    int
	RemainingEpisodes int
	RemainingMinutes  int
	Remaining         []int
	AverageMinutes    int
	Poster            string
}

type Service struct {
	db *storage.DB
}

func New(db *storage.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Add(ctx context.Context, userID, titleID int64) (Item, error) {
	row, err := s.db.AddQueueItem(ctx, userID, titleID)
	if errors.Is(err, storage.ErrNotFound) {
		return Item{}, apperr.NotFound()
	}
	if errors.Is(err, storage.ErrQueueDuplicate) {
		return Item{}, apperr.AlreadyInQueue()
	}
	if errors.Is(err, storage.ErrPickSeason) {
		return Item{}, apperr.Validation("У сериала несколько сезонов. Откройте его и выберите сезон.")
	}
	if err != nil {
		return Item{}, apperr.Internal(err)
	}
	return itemFrom(row), nil
}

func (s *Service) AddSeason(ctx context.Context, userID, seasonID int64) (Item, error) {
	row, err := s.db.AddSeason(ctx, userID, seasonID)
	if errors.Is(err, storage.ErrNotFound) {
		return Item{}, apperr.NotFound()
	}
	if errors.Is(err, storage.ErrQueueDuplicate) {
		return Item{}, apperr.AlreadyInQueue()
	}
	if err != nil {
		return Item{}, apperr.Internal(err)
	}
	return itemFrom(row), nil
}

func (s *Service) AddAll(ctx context.Context, userID, titleID int64) ([]Item, error) {
	rows, err := s.db.AddAllSeasons(ctx, userID, titleID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, apperr.NotFound()
	}
	if errors.Is(err, storage.ErrQueueDuplicate) {
		return nil, apperr.AlreadyInQueue()
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	items := make([]Item, len(rows))
	for i, row := range rows {
		items[i] = itemFrom(row)
	}
	return items, nil
}

func (s *Service) DailyMinutes(ctx context.Context, userID int64) (int, error) {
	minutes, err := s.db.DailyMinutes(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return 0, apperr.NotFound()
	}
	if err != nil {
		return 0, apperr.Internal(err)
	}
	return minutes, nil
}

func (s *Service) SetDailyMinutes(ctx context.Context, userID int64, minutes int) error {
	if _, err := plan.NormalizeDaily(minutes); err != nil {
		return apperr.Validation("Укажите от 0 до 1440 минут в день.")
	}
	err := s.db.SetDailyMinutes(ctx, userID, minutes)
	if errors.Is(err, storage.ErrNotFound) {
		return apperr.NotFound()
	}
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (s *Service) List(ctx context.Context, userID int64) ([]Item, error) {
	rows, err := s.db.ListQueue(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	items := make([]Item, len(rows))
	for i, row := range rows {
		items[i] = itemFrom(row)
	}
	return items, nil
}

func (s *Service) Move(ctx context.Context, userID, itemID int64, direction string) error {
	delta, err := deltaOf(direction)
	if err != nil {
		return err
	}
	err = s.db.MoveQueueItem(ctx, userID, itemID, delta)
	if errors.Is(err, storage.ErrNotFound) {
		return apperr.NotFound()
	}
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (s *Service) Remove(ctx context.Context, userID, itemID int64) error {
	err := s.db.DeleteQueueItem(ctx, userID, itemID)
	if errors.Is(err, storage.ErrNotFound) {
		return apperr.NotFound()
	}
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func deltaOf(direction string) (int, error) {
	switch direction {
	case "up":
		return -1, nil
	case "down":
		return 1, nil
	default:
		return 0, apperr.Validation("Укажите направление: up или down.")
	}
}

func itemFrom(row storage.QueueRow) Item {
	name := row.Title.Name
	if row.SeasonCount > 1 {
		name = fmt.Sprintf("%s · сезон %d", row.Title.Name, row.SeasonNumber)
	}
	remaining := row.Remaining
	if remaining == nil {
		remaining = []int{}
	}
	minutes := 0
	for _, runtime := range remaining {
		if runtime > 0 {
			minutes += runtime
		}
	}
	return Item{
		ID:                row.ID,
		Position:          row.Position,
		TitleID:           row.Title.ID,
		SeasonID:          row.SeasonID,
		SeasonNumber:      row.SeasonNumber,
		Name:              name,
		Year:              row.Title.Year,
		EpisodeCount:      row.Title.EpisodeCount,
		SeasonEpisodes:    row.TotalEpisodes,
		RemainingEpisodes: len(remaining),
		RemainingMinutes:  minutes,
		Remaining:         remaining,
		AverageMinutes:    row.Title.AverageMinutes,
		Poster:            row.Title.Poster,
	}
}
