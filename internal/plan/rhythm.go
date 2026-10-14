package plan

import "errors"

const MaxDailyMinutes = 24 * 60

var ErrInvalidDaily = errors.New("invalid daily minutes")

// Rhythm is a minutes-per-day pace across the queue.
// Offsets count days after today; today is 0.
// An offset is meaningful only when the matching minutes and the pace are both positive.
type Rhythm struct {
	DailyMinutes   int
	CurrentTitle   string
	CurrentMinutes int
	CurrentOffset  int
	HasCurrent     bool
	QueueMinutes   int
	QueueOffset    int
	HasQueue       bool
}

func NormalizeDaily(minutes int) (int, error) {
	if minutes < 0 || minutes > MaxDailyMinutes {
		return 0, ErrInvalidDaily
	}
	return minutes, nil
}

// FinishOffset reports when a pace of daily minutes uses up remaining minutes.
func FinishOffset(remaining, daily int) (int, bool) {
	if remaining <= 0 || daily <= 0 {
		return 0, false
	}
	span := (remaining + daily - 1) / daily
	return span - 1, true
}

// RhythmOf takes the queue in watch order. The current title is the first
// one that still has a known runtime. Finished and unknown titles are skipped.
func RhythmOf(daily int, items []Item) Rhythm {
	out := Rhythm{DailyMinutes: daily}
	for _, item := range items {
		minutes := knownMinutes(item)
		if minutes <= 0 {
			continue
		}
		out.QueueMinutes += minutes
		if !out.HasCurrent {
			out.HasCurrent = true
			out.CurrentTitle = item.Title
			out.CurrentMinutes = minutes
		}
	}
	out.HasQueue = out.QueueMinutes > 0
	if offset, ok := FinishOffset(out.CurrentMinutes, daily); ok {
		out.CurrentOffset = offset
	}
	if offset, ok := FinishOffset(out.QueueMinutes, daily); ok {
		out.QueueOffset = offset
	}
	return out
}

func knownMinutes(item Item) int {
	runtimes, ok := episodeRuntimes(item)
	if !ok {
		return 0
	}
	minutes := 0
	for _, runtime := range runtimes {
		if runtime > 0 {
			minutes += runtime
		}
	}
	return minutes
}
