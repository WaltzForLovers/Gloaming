package follow

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/WaltzForLovers/Gloaming/internal/plan"
	"github.com/WaltzForLovers/Gloaming/internal/queue"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

const LetterThursday = "season_thursday"

func LetterBody(title string) string {
	return `"` + title + `" ends on Thursday.`
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
	return Refresh(ctx, db, userID)
}

// Refresh recomputes the genre picks shown under the queue.
func Refresh(ctx context.Context, db *storage.DB, userID int64) error {
	queued, err := db.QueuedTitles(ctx, userID)
	if err != nil {
		return err
	}
	pool, err := db.SuggestPool(ctx, userID, genreSet(queued))
	if err != nil {
		return err
	}
	return db.ReplaceSuggestions(ctx, userID, Picks(queued, pool, PickLimit))
}

const PickLimit = 24

type scored struct {
	title  storage.Title
	shared []string
	weight int
	like   string
}

// Picks ranks pool titles by how many genres they share with the queue,
// skipping spin-offs and namesakes of queued titles.
func Picks(queued, pool []storage.Title, limit int) []storage.Pick {
	weight := map[string]int{}
	for _, title := range queued {
		for _, genre := range uniqueGenres(title.Genres) {
			weight[genre]++
		}
	}
	if len(weight) == 0 || limit <= 0 {
		return nil
	}
	var names []string
	for _, title := range queued {
		if name := normName(title.Name); name != "" {
			names = append(names, name)
		}
	}
	var ranked []scored
	for _, title := range pool {
		name := normName(title.Name)
		if name == "" || namesake(name, names) {
			continue
		}
		var shared []string
		total := 0
		for _, genre := range uniqueGenres(title.Genres) {
			if weight[genre] > 0 {
				shared = append(shared, genre)
				total += weight[genre]
			}
		}
		need := 2
		if len(weight) < need || len(uniqueGenres(title.Genres)) < need {
			need = 1
		}
		if len(shared) < need {
			continue
		}
		ranked = append(ranked, scored{title: title, shared: shared, weight: total, like: closest(title, queued)})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if len(a.shared) != len(b.shared) {
			return len(a.shared) > len(b.shared)
		}
		if a.weight != b.weight {
			return a.weight > b.weight
		}
		if (a.title.Poster != "") != (b.title.Poster != "") {
			return a.title.Poster != ""
		}
		if a.title.Rating != b.title.Rating {
			return a.title.Rating > b.title.Rating
		}
		if a.title.Year != b.title.Year {
			return a.title.Year > b.title.Year
		}
		return a.title.ID < b.title.ID
	})
	seen := map[string]bool{}
	var picks []storage.Pick
	for _, item := range ranked {
		name := normName(item.title.Name)
		if seen[name] {
			continue
		}
		seen[name] = true
		picks = append(picks, storage.Pick{TitleID: item.title.ID, Reason: reason(item)})
		if len(picks) == limit {
			break
		}
	}
	return picks
}

func reason(item scored) string {
	genres := strings.Join(item.shared, ", ")
	if item.like == "" {
		return genres
	}
	return `Like "` + item.like + `": ` + genres
}

func closest(title storage.Title, queued []storage.Title) string {
	mine := map[string]bool{}
	for _, genre := range title.Genres {
		mine[genre] = true
	}
	best, bestCount := "", 0
	for _, other := range queued {
		count := 0
		for _, genre := range uniqueGenres(other.Genres) {
			if mine[genre] {
				count++
			}
		}
		if count > bestCount {
			best, bestCount = other.Name, count
		}
	}
	return best
}

func namesake(name string, queued []string) bool {
	for _, other := range queued {
		short, long := name, other
		if len(short) > len(long) {
			short, long = long, short
		}
		if len(short) < 4 {
			if short == long {
				return true
			}
			continue
		}
		if strings.Contains(" "+long+" ", " "+short+" ") {
			return true
		}
	}
	return false
}

func normName(name string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
			continue
		}
		space = true
	}
	return b.String()
}

func uniqueGenres(genres []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, genre := range genres {
		if genre == "" || seen[genre] {
			continue
		}
		seen[genre] = true
		out = append(out, genre)
	}
	return out
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

func genreSet(titles []storage.Title) []string {
	var all []string
	for _, title := range titles {
		all = append(all, title.Genres...)
	}
	return uniqueGenres(all)
}
