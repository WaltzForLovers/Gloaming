package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientParsesShowAndEpisodes(t *testing.T) {
	var gotShowQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/shows":
			gotShowQuery = r.URL.Query().Get("q")
			w.Write([]byte(`[
				{"show":{"id":139,"name":"Girls","premiered":"2012-04-15","averageRuntime":30,"runtime":28,"image":{"medium":"https://img.example/g.jpg"}}},
				{"show":{"id":0,"name":"skip"}},
				{"show":{"id":2,"name":"No date","premiered":null,"averageRuntime":null,"runtime":44}}
			]`))
		case "/shows/139/episodes":
			w.Write([]byte(`[{"runtime":30},{"runtime":30}]`))
		case "/shows/2/episodes":
			w.Write([]byte(`[{"runtime":null}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL)
	shows, err := client.SearchShows(context.Background(), "girls & co")
	if err != nil {
		t.Fatal(err)
	}
	if gotShowQuery != "girls & co" {
		t.Fatalf("query %q", gotShowQuery)
	}
	if len(shows) != 2 || shows[0].ID != 139 || shows[0].Year != 2012 || shows[0].AverageRuntime != 30 || shows[0].Runtime != 28 || shows[0].Poster != "https://img.example/g.jpg" {
		t.Fatalf("shows %+v", shows)
	}
	if shows[1].Poster != "" {
		t.Fatalf("poster %q", shows[1].Poster)
	}
	if shows[1].Year != 0 || shows[1].AverageRuntime != 0 || shows[1].Runtime != 44 {
		t.Fatalf("second %+v", shows[1])
	}
	episodes, err := client.Episodes(context.Background(), 139)
	if err != nil || len(episodes) != 2 || episodes[0].Runtime != 30 {
		t.Fatalf("%+v %v", episodes, err)
	}
}

func TestHTTPClientBadStatusAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/shows" {
			http.Error(w, "nope", http.StatusBadGateway)
			return
		}
		w.Write([]byte(`{`))
	}))
	defer srv.Close()
	client := NewHTTPClient(srv.URL)
	if _, err := client.SearchShows(context.Background(), "girls"); err == nil {
		t.Fatal("expected status error")
	}
	if _, err := client.Episodes(context.Background(), 1); err == nil {
		t.Fatal("expected json error")
	}
}
