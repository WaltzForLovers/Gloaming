package plan

import "errors"

const MaxFreeMinutes = 7 * 24 * 60

var ErrInvalidFreeTime = errors.New("invalid free time")

type Outcome string

const (
	OutcomeFull    Outcome = "full"
	OutcomePartial Outcome = "partial"
	OutcomeSkipped Outcome = "skipped"
	OutcomeStopped Outcome = "stopped"
)

type Item struct {
	Title          string
	EpisodeCount   int
	AverageMinutes int
	// Remaining is the unwatched episode runtimes, in order.
	// A nil slice keeps the older count-and-average rule.
	// An empty slice means nothing is left to watch.
	Remaining []int
}

type Line struct {
	Title    string
	Outcome  Outcome
	Episodes int
	Minutes  int
}

type Result struct {
	Lines        []Line
	SpentMinutes int
	LeftMinutes  int
	QueueMinutes int
}

func FreeMinutes(hours, minutes int) (int, error) {
	if hours < 0 || minutes < 0 || minutes > MaxFreeMinutes || hours > MaxFreeMinutes/60 {
		return 0, ErrInvalidFreeTime
	}
	total := hours*60 + minutes
	if total <= 0 || total > MaxFreeMinutes {
		return 0, ErrInvalidFreeTime
	}
	return total, nil
}

func Build(items []Item, freeMinutes int) (Result, error) {
	if freeMinutes <= 0 || freeMinutes > MaxFreeMinutes {
		return Result{}, ErrInvalidFreeTime
	}

	lines := make([]Line, 0)
	left := freeMinutes
	queueMinutes := 0
	stopped := false
	for _, item := range items {
		runtimes, ok := episodeRuntimes(item)
		if ok {
			for _, runtime := range runtimes {
				if runtime > 0 {
					queueMinutes += runtime
				}
			}
		}
		if item.Remaining != nil && len(item.Remaining) == 0 {
			continue
		}
		if stopped {
			continue
		}
		line, nextLeft, stop := place(item, runtimes, ok, left)
		lines = append(lines, line)
		left = nextLeft
		stopped = stop
	}

	return Result{
		Lines:        lines,
		SpentMinutes: freeMinutes - left,
		LeftMinutes:  left,
		QueueMinutes: queueMinutes,
	}, nil
}

func episodeRuntimes(item Item) ([]int, bool) {
	if item.Remaining != nil {
		if len(item.Remaining) == 0 {
			return nil, false
		}
		return item.Remaining, true
	}
	if item.EpisodeCount <= 0 || item.AverageMinutes <= 0 {
		return nil, false
	}
	runtimes := make([]int, item.EpisodeCount)
	for i := range runtimes {
		runtimes[i] = item.AverageMinutes
	}
	return runtimes, true
}

// place walks the queue in order. A later shorter title is not pulled forward.
func place(item Item, runtimes []int, ok bool, left int) (Line, int, bool) {
	line := Line{Title: item.Title}
	if !ok {
		line.Outcome = OutcomeSkipped
		return line, left, false
	}
	taken := 0
	spent := 0
	for _, runtime := range runtimes {
		if runtime <= 0 {
			continue
		}
		if runtime > left {
			if taken == 0 {
				line.Outcome = OutcomeStopped
				return line, left, true
			}
			line.Outcome = OutcomePartial
			line.Episodes = taken
			line.Minutes = spent
			return line, left, true
		}
		taken++
		spent += runtime
		left -= runtime
	}
	if taken == 0 {
		line.Outcome = OutcomeSkipped
		return line, left, false
	}
	line.Outcome = OutcomeFull
	line.Episodes = taken
	line.Minutes = spent
	return line, left, false
}
