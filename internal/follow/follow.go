package follow

import (
	"context"
	"strings"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/plan"
	"github.com/WaltzForLovers/Gloaming/internal/queue"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

const LetterThursday = "season_thursday"

func LetterBody(title string) string {
	return "«" + title + "» кончится в четверг."
}

// Apply stores the pace, a Thursday letter, and genre picks for one user.
// The watch mark itself is already saved; this only follows from it.
func Apply(ctx context.Context, db *storage.DB, userID int64, now time.Time) error {
	items, err := queue.New(db).List(ctx, userID)
	if err != nil {
		return err
	}
	daily, err := db.DailyMinutes(ctx, userID)
	if err != nil {
		return err
	}
	rhythm := plan.RhythmOf(daily, planItems(items))
	hasDate := rhythm.HasCurrent && daily > 0
	if err := db.SaveRhythmMark(ctx, userID, rhythm.CurrentTitle, rhythm.CurrentOffset, hasDate); err != nil {
		return err
	}
	if hasDate && now.AddDate(0, 0, rhythm.CurrentOffset).Weekday() == time.Thursday {
		if err := db.PutLetter(ctx, userID, LetterThursday, LetterBody(rhythm.CurrentTitle)); err != nil {
			return err
		}
	} else if err := db.ClearLetter(ctx, userID, LetterThursday); err != nil {
		return err
	}
	queued, err := db.QueuedTitles(ctx, userID)
	if err != nil {
		return err
	}
	found, err := db.Suggest(ctx, userID, joinedGenres(queued), 5)
	if err != nil {
		return err
	}
	ids := make([]int64, len(found))
	for i, title := range found {
		ids[i] = title.ID
	}
	return db.ReplaceSuggestions(ctx, userID, ids)
}

func planItems(items []queue.Item) []plan.Item {
	out := make([]plan.Item, len(items))
	for i, item := range items {
		remaining := item.Remaining
		if remaining == nil {
			remaining = []int{}
		}
		out[i] = plan.Item{Title: item.Name, Remaining: remaining}
	}
	return out
}

func joinedGenres(titles []storage.Title) string {
	seen := map[string]bool{}
	var genres []string
	for _, title := range titles {
		for _, genre := range title.Genres {
			if genre == "" || seen[genre] {
				continue
			}
			seen[genre] = true
			genres = append(genres, genre)
		}
	}
	return strings.Join(genres, ",")
}
