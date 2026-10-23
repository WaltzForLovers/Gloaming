package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const filmSearchLimit = 15

var filmTypes = map[string]bool{
	"Q11424":  true, // film
	"Q202866": true, // animated film
	"Q506240": true, // television film
	"Q93204":  true, // documentary film
	"Q24862":  true, // short film
}

type Wikidata struct {
	wiki  string
	pages string
	http  *http.Client
	cache Cache
}

func (c *Wikidata) WithCache(cache Cache) *Wikidata {
	c.cache = cache
	return c
}

func NewWikidata(wikiBase, pagesBase string) *Wikidata {
	return &Wikidata{
		wiki:  strings.TrimRight(wikiBase, "/"),
		pages: strings.TrimRight(pagesBase, "/"),
		http:  &http.Client{},
	}
}

func (c *Wikidata) SearchFilms(ctx context.Context, query string) ([]Show, error) {
	ids, err := c.searchIDs(ctx, query)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	films, titles, err := c.films(ctx, ids)
	if err != nil || len(films) == 0 {
		return films, err
	}
	pctx, pcancel := context.WithTimeout(ctx, time.Second)
	defer pcancel()
	posters, err := c.posters(pctx, titles)
	if err != nil {
		return films, nil
	}
	for i, title := range titles {
		if title == "" || i >= len(films) {
			continue
		}
		films[i].Poster = posters[title]
	}
	return films, nil
}

func (c *Wikidata) searchIDs(ctx context.Context, query string) ([]string, error) {
	type batch struct {
		lang string
		ids  []string
		err  error
	}
	ch := make(chan batch, 2)
	for _, lang := range []string{"ru", "en"} {
		go func(lang string) {
			ids, err := c.searchLang(ctx, query, lang)
			ch <- batch{lang: lang, ids: ids, err: err}
		}(lang)
	}
	found := map[string]batch{}
	var last error
	ok := 0
	for range 2 {
		part := <-ch
		found[part.lang] = part
		if part.err != nil {
			last = part.err
			continue
		}
		ok++
	}
	if ok == 0 && last != nil {
		return nil, last
	}
	seen := map[string]bool{}
	ids := make([]string, 0, filmSearchLimit)
	for _, lang := range []string{"ru", "en"} {
		for _, id := range found[lang].ids {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 20 {
		ids = ids[:20]
	}
	return ids, nil
}

func (c *Wikidata) searchLang(ctx context.Context, query, lang string) ([]string, error) {
	endpoint := c.wiki + "/w/api.php?" + url.Values{
		"action":   {"wbsearchentities"},
		"search":   {query},
		"language": {lang},
		"format":   {"json"},
		"type":     {"item"},
		"limit":    {strconv.Itoa(filmSearchLimit)},
	}.Encode()
	var payload struct {
		Search []struct {
			ID string `json:"id"`
		} `json:"search"`
	}
	if err := c.get(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(payload.Search))
	for _, item := range payload.Search {
		if strings.HasPrefix(item.ID, "Q") {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}

func (c *Wikidata) films(ctx context.Context, ids []string) ([]Show, []string, error) {
	endpoint := c.wiki + "/w/api.php?" + url.Values{
		"action":     {"wbgetentities"},
		"ids":        {strings.Join(ids, "|")},
		"props":      {"labels|descriptions|claims|sitelinks"},
		"languages":  {"ru|en"},
		"sitefilter": {"ruwiki|enwiki"},
		"format":     {"json"},
	}.Encode()
	var payload struct {
		Entities map[string]wikiEntity `json:"entities"`
	}
	if err := c.get(ctx, endpoint, &payload); err != nil {
		return nil, nil, err
	}
	films := make([]Show, 0, len(ids))
	titles := make([]string, 0, len(ids))
	for _, id := range ids {
		entity, ok := payload.Entities[id]
		if !ok || !entity.film() {
			continue
		}
		show, title, ok := entity.show(id)
		if !ok {
			continue
		}
		films = append(films, show)
		titles = append(titles, title)
		if len(films) == searchLimit {
			break
		}
	}
	return films, titles, nil
}

func (c *Wikidata) posters(ctx context.Context, titles []string) (map[string]string, error) {
	found := make(map[string]string, len(titles))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, title := range titles {
		if title == "" {
			continue
		}
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			poster, err := c.poster(ctx, title)
			if err != nil || poster == "" {
				return
			}
			mu.Lock()
			found[title] = poster
			mu.Unlock()
		}(title)
	}
	wg.Wait()
	return found, nil
}

func (c *Wikidata) Poster(ctx context.Context, title string) (string, error) {
	if title == "" {
		return "", nil
	}
	return c.poster(ctx, title)
}

func (c *Wikidata) poster(ctx context.Context, title string) (string, error) {
	endpoint := c.pages + "/api/rest_v1/page/summary/" + url.PathEscape(title)
	var payload struct {
		Thumbnail struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
	}
	if err := c.get(ctx, endpoint, &payload); err != nil {
		return "", err
	}
	return payload.Thumbnail.Source, nil
}

func (c *Wikidata) get(ctx context.Context, endpoint string, dest any) error {
	body, status, err := loadBody(ctx, c.http, c.cache, "Gloaming/1.0 (local watch queue)", endpoint)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return err
	}
	return nil
}

type statusError int

func (e statusError) Error() string { return "catalog status " + strconv.Itoa(int(e)) }

func errStatus(code int) error { return statusError(code) }

type wikiEntity struct {
	Labels map[string]struct {
		Value string `json:"value"`
	} `json:"labels"`
	Descriptions map[string]struct {
		Value string `json:"value"`
	} `json:"descriptions"`
	Sitelinks map[string]struct {
		Title string `json:"title"`
	} `json:"sitelinks"`
	Claims map[string][]struct {
		MainSnak struct {
			DataValue struct {
				Value json.RawMessage `json:"value"`
			} `json:"datavalue"`
		} `json:"mainsnak"`
	} `json:"claims"`
}

func (e wikiEntity) film() bool {
	for _, claim := range e.Claims["P31"] {
		var value struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(claim.MainSnak.DataValue.Value, &value) == nil && filmTypes[value.ID] {
			return true
		}
	}
	return false
}

func (e wikiEntity) show(id string) (Show, string, bool) {
	number, err := strconv.Atoi(strings.TrimPrefix(id, "Q"))
	name := e.Labels["ru"].Value
	if name == "" {
		name = e.Labels["en"].Value
	}
	if err != nil || number <= 0 || name == "" {
		return Show{}, "", false
	}
	title := e.Sitelinks["ruwiki"].Title
	if title == "" {
		title = e.Sitelinks["enwiki"].Title
	}
	summary := e.Descriptions["ru"].Value
	if summary == "" {
		summary = e.Descriptions["en"].Value
	}
	return Show{
		ID:        number,
		Source:    sourceWikidata,
		Name:      name,
		Year:      e.year(),
		Runtime:   e.minutes(),
		WikiTitle: title,
		Summary:   summary,
	}, title, true
}

func (e wikiEntity) year() int {
	best := 0
	for _, claim := range e.Claims["P577"] {
		var value struct {
			Time string `json:"time"`
		}
		if json.Unmarshal(claim.MainSnak.DataValue.Value, &value) != nil || len(value.Time) < 5 {
			continue
		}
		year, err := strconv.Atoi(strings.TrimPrefix(value.Time[:5], "+"))
		if err != nil || year <= 0 {
			continue
		}
		if best == 0 || year < best {
			best = year
		}
	}
	return best
}

func (e wikiEntity) minutes() int {
	for _, claim := range e.Claims["P2047"] {
		var value struct {
			Amount string `json:"amount"`
			Unit   string `json:"unit"`
		}
		if json.Unmarshal(claim.MainSnak.DataValue.Value, &value) != nil {
			continue
		}
		amount, err := strconv.Atoi(strings.TrimPrefix(value.Amount, "+"))
		if err != nil || amount <= 0 {
			continue
		}
		switch {
		case strings.HasSuffix(value.Unit, "Q7727"):
			return amount
		case strings.HasSuffix(value.Unit, "Q11574"):
			if amount >= 60 {
				return amount / 60
			}
		case strings.HasSuffix(value.Unit, "Q25235"):
			return amount * 60
		}
	}
	return 0
}
