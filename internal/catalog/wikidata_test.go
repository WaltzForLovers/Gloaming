package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWikidataSearchKeepsFilms(t *testing.T) {
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/w/api.php" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("action") {
		case "wbsearchentities":
			if r.URL.Query().Get("search") != "dragon & co" {
				t.Errorf("search %q", r.URL.Query().Get("search"))
			}
			w.Write([]byte(`{"search":[{"id":"Q373096","label":"How to Train Your Dragon"},{"id":"Q858935","label":"How to Train Your Dragon"}]}`))
		case "wbgetentities":
			if r.URL.Query().Get("ids") != "Q373096|Q858935" {
				t.Errorf("ids %q", r.URL.Query().Get("ids"))
			}
			w.Write([]byte(`{"entities":{
				"Q373096":{
					"labels":{"en":{"value":"How to Train Your Dragon"}},
					"sitelinks":{"enwiki":{"title":"How to Train Your Dragon (2010 film)"}},
					"claims":{
						"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q202866"}}}}],
						"P577":[{"mainsnak":{"datavalue":{"value":{"time":"+2010-03-26T00:00:00Z"}}}},{"mainsnak":{"datavalue":{"value":{"time":"+2010-03-18T00:00:00Z"}}}}],
						"P2047":[{"mainsnak":{"datavalue":{"value":{"amount":"+98","unit":"http://www.wikidata.org/entity/Q7727"}}}}]
					}
				},
				"Q858935":{
					"labels":{"en":{"value":"How to Train Your Dragon"}},
					"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q7889"}}}}]}
				}
			}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wiki.Close()

	pages := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/rest_v1/page/summary/"))
		if err != nil || got != "How to Train Your Dragon (2010 film)" {
			t.Errorf("path %q", r.URL.Path)
		}
		w.Write([]byte(`{"thumbnail":{"source":"https://upload.wikimedia.org/httyd.jpg"}}`))
	}))
	defer pages.Close()

	films, err := NewWikidata(wiki.URL, pages.URL).SearchFilms(context.Background(), "dragon & co")
	if err != nil {
		t.Fatal(err)
	}
	if len(films) != 1 || films[0].ID != 373096 || films[0].Name != "How to Train Your Dragon" || films[0].Year != 2010 || films[0].Runtime != 98 || films[0].Poster != "https://upload.wikimedia.org/httyd.jpg" {
		t.Fatalf("%+v", films)
	}
}
