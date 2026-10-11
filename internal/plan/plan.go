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
		if known(item) {
			queueMinutes += item.EpisodeCount * item.AverageMinutes
		}
		if stopped {
			continue
		}
		line, nextLeft, stop := place(item, left)
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

func known(item Item) bool {
	return item.EpisodeCount > 0 && item.AverageMinutes > 0
}

// place walks the queue in order. A later shorter title is not pulled forward.
func place(item Item, left int) (Line, int, bool) {
	line := Line{Title: item.Title}
	if !known(item) {
		line.Outcome = OutcomeSkipped
		return line, left, false
	}
	if item.EpisodeCount == 1 {
		if item.AverageMinutes <= left {
			line.Outcome = OutcomeFull
			line.Episodes = 1
			line.Minutes = item.AverageMinutes
			return line, left - item.AverageMinutes, false
		}
		line.Outcome = OutcomeStopped
		return line, left, true
	}

	total := item.EpisodeCount * item.AverageMinutes
	if total <= left {
		line.Outcome = OutcomeFull
		line.Episodes = item.EpisodeCount
		line.Minutes = total
		return line, left - total, false
	}
	episodes := left / item.AverageMinutes
	if episodes < 1 {
		line.Outcome = OutcomeStopped
		return line, left, true
	}
	spent := episodes * item.AverageMinutes
	line.Outcome = OutcomePartial
	line.Episodes = episodes
	line.Minutes = spent
	return line, left - spent, true
}
