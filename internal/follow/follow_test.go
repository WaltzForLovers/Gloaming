package follow

import (
	"testing"

	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

func TestPicksRankSharedGenresAndSkipNamesakes(t *testing.T) {
	queued := []storage.Title{
		{ID: 1, Name: "Breaking Bad", Genres: []string{"Drama", "Crime", "Thriller"}},
		{ID: 2, Name: "Supernatural", Genres: []string{"Drama", "Action", "Supernatural"}},
	}
	pool := []storage.Title{
		{ID: 10, Name: "Breaking Bad: Original Minisodes", Genres: []string{"Drama", "Crime"}, Poster: "p"},
		{ID: 11, Name: "One Genre", Genres: []string{"Drama", "Comedy"}, Poster: "p"},
		{ID: 12, Name: "Better Call Saul", Genres: []string{"Drama", "Crime"}, Rating: 8.7},
		{ID: 13, Name: "Ozark", Genres: []string{"Drama", "Crime", "Thriller"}, Poster: "p", Rating: 8.0},
		{ID: 14, Name: "Narcos", Genres: []string{"Drama", "Crime"}, Poster: "p", Rating: 8.6},
		{ID: 15, Name: "narcos", Genres: []string{"Drama", "Crime"}, Poster: "p", Rating: 1},
		{ID: 16, Name: "Lucifer", Genres: []string{"Drama", "Supernatural"}, Poster: "p", Rating: 8.0},
	}
	picks := Picks(queued, pool, 10)
	var ids []int64
	for _, pick := range picks {
		ids = append(ids, pick.TitleID)
	}
	want := []int64{13, 14, 16, 12}
	if len(ids) != len(want) {
		t.Fatalf("%v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("%v", ids)
		}
	}
	if picks[0].Reason != `Like "Breaking Bad": Drama, Crime, Thriller` {
		t.Fatalf("%q", picks[0].Reason)
	}
	if picks[2].Reason != `Like "Supernatural": Drama, Supernatural` {
		t.Fatalf("%q", picks[2].Reason)
	}
}

func TestPicksAcceptOneGenreWhenTheQueueHasOne(t *testing.T) {
	queued := []storage.Title{{ID: 1, Name: "Queued", Genres: []string{"Drama"}}}
	pool := []storage.Title{{ID: 2, Name: "Neighbor", Genres: []string{"Drama", "Comedy"}}}
	picks := Picks(queued, pool, 6)
	if len(picks) != 1 || picks[0].TitleID != 2 || picks[0].Reason != `Like "Queued": Drama` {
		t.Fatalf("%+v", picks)
	}
	if got := Picks(nil, pool, 6); len(got) != 0 {
		t.Fatalf("empty queue %+v", got)
	}
}

func TestNamesakeNeedsWholeWords(t *testing.T) {
	queued := []string{normName("The Boys"), normName("It")}
	if !namesake(normName("The Boys Presents: Diabolical"), queued) {
		t.Fatal("spin-off kept")
	}
	if namesake(normName("The Boyscouts"), queued) {
		t.Fatal("prefix inside a word dropped")
	}
	if namesake(normName("It Crowd"), queued) {
		t.Fatal("short name matched inside a longer one")
	}
}
