package plan

import (
	"errors"
	"testing"
)

func TestFreeMinutes(t *testing.T) {
	tests := []struct {
		name    string
		hours   int
		minutes int
		want    int
		wantErr bool
	}{
		{name: "hour and half", hours: 1, minutes: 30, want: 90},
		{name: "minutes only", hours: 0, minutes: 90, want: 90},
		{name: "seven days", hours: 168, minutes: 0, want: MaxFreeMinutes},
		{name: "zero", hours: 0, minutes: 0, wantErr: true},
		{name: "negative hours", hours: -1, minutes: 30, wantErr: true},
		{name: "negative minutes", hours: 1, minutes: -1, wantErr: true},
		{name: "over seven days", hours: 168, minutes: 1, wantErr: true},
		{name: "too many minutes", hours: 0, minutes: MaxFreeMinutes + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FreeMinutes(tt.hours, tt.minutes)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidFreeTime) {
					t.Fatalf("FreeMinutes(%d, %d) err = %v, want ErrInvalidFreeTime", tt.hours, tt.minutes, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FreeMinutes(%d, %d) unexpected err: %v", tt.hours, tt.minutes, err)
			}
			if got != tt.want {
				t.Fatalf("FreeMinutes(%d, %d) = %d, want %d", tt.hours, tt.minutes, got, tt.want)
			}
		})
	}
}

func TestBuildRejectsInvalidFreeTime(t *testing.T) {
	for _, minutes := range []int{0, -5, MaxFreeMinutes + 1} {
		_, err := Build(nil, minutes)
		if !errors.Is(err, ErrInvalidFreeTime) {
			t.Fatalf("Build(nil, %d) err = %v, want ErrInvalidFreeTime", minutes, err)
		}
	}
}

func TestBuildEmptyQueue(t *testing.T) {
	got, err := Build(nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 0 || got.SpentMinutes != 0 || got.LeftMinutes != 90 || got.QueueMinutes != 0 {
		t.Fatalf("empty queue: %+v", got)
	}
}

func TestBuildFilmFits(t *testing.T) {
	got, err := Build([]Item{{Title: "Film", EpisodeCount: 1, AverageMinutes: 80}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines:        []Line{{Title: "Film", Outcome: OutcomeFull, Episodes: 1, Minutes: 80}},
		SpentMinutes: 80,
		LeftMinutes:  10,
		QueueMinutes: 80,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildFilmExact(t *testing.T) {
	got, err := Build([]Item{{Title: "Film", EpisodeCount: 1, AverageMinutes: 90}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines:        []Line{{Title: "Film", Outcome: OutcomeFull, Episodes: 1, Minutes: 90}},
		SpentMinutes: 90,
		LeftMinutes:  0,
		QueueMinutes: 90,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildFilmTooLongStopsAndSkipsLater(t *testing.T) {
	items := []Item{
		{Title: "Long", EpisodeCount: 1, AverageMinutes: 100},
		{Title: "Short", EpisodeCount: 1, AverageMinutes: 20},
	}
	got, err := Build(items, 40)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines:        []Line{{Title: "Long", Outcome: OutcomeStopped}},
		SpentMinutes: 0,
		LeftMinutes:  40,
		QueueMinutes: 120,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildSeriesFits(t *testing.T) {
	got, err := Build([]Item{{Title: "Show", EpisodeCount: 3, AverageMinutes: 20}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines:        []Line{{Title: "Show", Outcome: OutcomeFull, Episodes: 3, Minutes: 60}},
		SpentMinutes: 60,
		LeftMinutes:  30,
		QueueMinutes: 60,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildSeriesPartialStops(t *testing.T) {
	items := []Item{
		{Title: "Show", EpisodeCount: 10, AverageMinutes: 40},
		{Title: "Later", EpisodeCount: 1, AverageMinutes: 10},
	}
	got, err := Build(items, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines:        []Line{{Title: "Show", Outcome: OutcomePartial, Episodes: 2, Minutes: 80}},
		SpentMinutes: 80,
		LeftMinutes:  20,
		QueueMinutes: 410,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildSeriesDoesNotFitOneEpisode(t *testing.T) {
	got, err := Build([]Item{{Title: "Show", EpisodeCount: 8, AverageMinutes: 50}}, 40)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines:        []Line{{Title: "Show", Outcome: OutcomeStopped}},
		SpentMinutes: 0,
		LeftMinutes:  40,
		QueueMinutes: 400,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildUnknownDurationIsSkipped(t *testing.T) {
	items := []Item{
		{Title: "First", EpisodeCount: 1, AverageMinutes: 30},
		{Title: "Mystery", EpisodeCount: 0, AverageMinutes: 40},
		{Title: "NoRuntime", EpisodeCount: 6, AverageMinutes: 0},
		{Title: "Second", EpisodeCount: 1, AverageMinutes: 30},
	}
	got, err := Build(items, 90)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines: []Line{
			{Title: "First", Outcome: OutcomeFull, Episodes: 1, Minutes: 30},
			{Title: "Mystery", Outcome: OutcomeSkipped},
			{Title: "NoRuntime", Outcome: OutcomeSkipped},
			{Title: "Second", Outcome: OutcomeFull, Episodes: 1, Minutes: 30},
		},
		SpentMinutes: 60,
		LeftMinutes:  30,
		QueueMinutes: 60,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildPlansOnlyRemainingEpisodes(t *testing.T) {
	got, err := Build([]Item{{Title: "Season", Remaining: []int{40, 40, 40}}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines:        []Line{{Title: "Season", Outcome: OutcomePartial, Episodes: 2, Minutes: 80}},
		SpentMinutes: 80,
		LeftMinutes:  10,
		QueueMinutes: 120,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	done, err := Build([]Item{{Title: "Done", Remaining: []int{}}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if len(done.Lines) != 0 || done.SpentMinutes != 0 || done.LeftMinutes != 90 || done.QueueMinutes != 0 {
		t.Fatalf("finished season should leave the window: %+v", done)
	}

	skipped, err := Build([]Item{{Title: "Gap", Remaining: []int{0, 30}}}, 40)
	if err != nil {
		t.Fatal(err)
	}
	wantSkip := Result{
		Lines:        []Line{{Title: "Gap", Outcome: OutcomeFull, Episodes: 1, Minutes: 30}},
		SpentMinutes: 30,
		LeftMinutes:  10,
		QueueMinutes: 30,
	}
	if !sameResult(skipped, wantSkip) {
		t.Fatalf("unknown episode: %+v", skipped)
	}
}

func TestBuildContinuesAfterAFullItem(t *testing.T) {
	items := []Item{
		{Title: "Show", EpisodeCount: 2, AverageMinutes: 30},
		{Title: "Film", EpisodeCount: 1, AverageMinutes: 20},
	}
	got, err := Build(items, 90)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		Lines: []Line{
			{Title: "Show", Outcome: OutcomeFull, Episodes: 2, Minutes: 60},
			{Title: "Film", Outcome: OutcomeFull, Episodes: 1, Minutes: 20},
		},
		SpentMinutes: 80,
		LeftMinutes:  10,
		QueueMinutes: 80,
	}
	if !sameResult(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRhythmOf(t *testing.T) {
	items := []Item{
		{Title: "Done", Remaining: []int{}},
		{Title: "Current", Remaining: []int{60, 0, 60, 60}},
		{Title: "Later", Remaining: []int{30}},
	}
	got := RhythmOf(40, items)
	if !got.HasCurrent || got.CurrentTitle != "Current" || got.CurrentMinutes != 180 || got.CurrentOffset != 4 {
		t.Fatalf("current %+v", got)
	}
	if !got.HasQueue || got.QueueMinutes != 210 || got.QueueOffset != 5 || got.DailyMinutes != 40 {
		t.Fatalf("queue %+v", got)
	}

	today, ok := FinishOffset(40, 40)
	tomorrow, okTomorrow := FinishOffset(41, 40)
	if !ok || today != 0 || !okTomorrow || tomorrow != 1 {
		t.Fatalf("today %d %v tomorrow %d %v", today, ok, tomorrow, okTomorrow)
	}
	if _, ok := FinishOffset(30, 0); ok {
		t.Fatal("pace 0 should not place a date")
	}

	unset := RhythmOf(0, items)
	if !unset.HasCurrent || unset.CurrentOffset != 0 || unset.QueueOffset != 0 {
		t.Fatalf("unset %+v", unset)
	}
	if _, err := NormalizeDaily(1441); !errors.Is(err, ErrInvalidDaily) {
		t.Fatal(err)
	}
	if minutes, err := NormalizeDaily(0); err != nil || minutes != 0 {
		t.Fatalf("%d %v", minutes, err)
	}
}

func sameResult(got, want Result) bool {
	if got.SpentMinutes != want.SpentMinutes || got.LeftMinutes != want.LeftMinutes || got.QueueMinutes != want.QueueMinutes {
		return false
	}
	if len(got.Lines) != len(want.Lines) {
		return false
	}
	for i := range want.Lines {
		if got.Lines[i] != want.Lines[i] {
			return false
		}
	}
	return true
}
