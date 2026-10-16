package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/auth"
	"github.com/WaltzForLovers/Gloaming/internal/catalog"
	"github.com/WaltzForLovers/Gloaming/internal/limit"
	"github.com/WaltzForLovers/Gloaming/internal/queue"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
	"github.com/WaltzForLovers/Gloaming/web"
)

func TestAPI(t *testing.T) {
	tv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/shows":
			if r.URL.Query().Get("q") == "missing-title" {
				w.Write([]byte(`[]`))
				return
			}
			w.Write([]byte(`[
				{"show":{"id":169,"name":"Test Show","premiered":"2008-01-20","averageRuntime":45,"image":{"medium":"https://img.example/show.jpg"}}},
				{"show":{"id":2,"name":"Test Film","premiered":"1999-05-01","averageRuntime":80}}
			]`))
		case "/shows/169/episodes":
			w.Write([]byte(`[{"runtime":45},{"runtime":45},{"runtime":45},{"runtime":45}]`))
		case "/shows/2/episodes":
			w.Write([]byte(`[{"runtime":80}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer tv.Close()

	db := storage.OpenTest(t)
	handler := New(auth.New(db), catalog.New(db, catalog.NewHTTPClient(tv.URL)), queue.New(db), io.Discard, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	status, body := post(t, client, srv.URL+"/api/login", `{"username":"ann","password":"password"}`)
	if status != http.StatusUnauthorized || !strings.Contains(body, apperr.InvalidCredentials().Message) {
		t.Fatalf("login missing: %d %s", status, body)
	}

	status, body = post(t, client, srv.URL+"/api/register", `{"username":"an","password":"password"}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "Имя:") {
		t.Fatalf("register: %d %s", status, body)
	}
	status, body = post(t, client, srv.URL+"/api/register", `{"username":"ann","password":"password","email":"ann@example.com"}`)
	if status != http.StatusCreated || !strings.Contains(body, `"username":"ann"`) {
		t.Fatalf("register ok: %d %s", status, body)
	}
	status, _ = post(t, client, srv.URL+"/api/register", `{"username":"ann","password":"password","email":"other@example.com"}`)
	if status != http.StatusConflict {
		t.Fatalf("duplicate %d", status)
	}

	status, body = post(t, client, srv.URL+"/api/login", `{"username":"ann","password":"password"}`)
	if status != http.StatusOK || !strings.Contains(body, `"username":"ann"`) {
		t.Fatalf("login: %d %s", status, body)
	}
	status, body = get(t, client, srv.URL+"/api/me")
	if status != http.StatusOK || !strings.Contains(body, `"username":"ann"`) {
		t.Fatalf("me: %d %s", status, body)
	}

	status, body = get(t, client, srv.URL+"/api/queue")
	if status != http.StatusOK || !strings.Contains(body, `"items":[]`) {
		t.Fatalf("empty queue: %d %s", status, body)
	}

	noCookie := &http.Client{}
	status, body = get(t, noCookie, srv.URL+"/api/queue")
	if status != http.StatusUnauthorized || !strings.Contains(body, apperr.Unauthorized().Message) || strings.Contains(body, "sql") {
		t.Fatalf("no cookie: %d %s", status, body)
	}

	status, body = get(t, client, srv.URL+"/api/titles?q=te")
	if status != http.StatusOK || !strings.Contains(body, "https://img.example/show.jpg") {
		t.Fatalf("search: %d %s", status, body)
	}
	var search struct {
		Titles []struct {
			ID             int64  `json:"id"`
			Name           string `json:"name"`
			Year           int    `json:"year"`
			EpisodeCount   int    `json:"episodeCount"`
			AverageMinutes int    `json:"averageMinutes"`
			Kind           string `json:"kind"`
		} `json:"titles"`
	}
	if err := json.Unmarshal([]byte(body), &search); err != nil {
		t.Fatal(err)
	}
	if len(search.Titles) != 2 || search.Titles[0].Name != "Test Show" || search.Titles[0].Year != 2008 || search.Titles[0].Kind != "series" || search.Titles[1].Kind != "film" {
		t.Fatalf("%+v", search.Titles)
	}

	status, body = get(t, client, srv.URL+"/api/titles?q=missing-title")
	if status != http.StatusNotFound || !strings.Contains(body, apperr.TitleNotFound().Message) {
		t.Fatalf("missing: %d %s", status, body)
	}

	showID := search.Titles[0].ID
	filmID := search.Titles[1].ID
	status, body = get(t, client, srv.URL+"/api/titles/"+itoa(showID))
	if status != http.StatusOK || !strings.Contains(body, `"averageMinutes":45`) {
		t.Fatalf("card: %d %s", status, body)
	}

	tv.Close()
	status, body = get(t, client, srv.URL+"/api/titles/"+itoa(showID))
	if status != http.StatusOK || !strings.Contains(body, "Test Show") || !strings.Contains(body, "https://img.example/show.jpg") {
		t.Fatalf("cached card: %d %s", status, body)
	}
	status, body = get(t, client, srv.URL+"/api/titles?q=other-show")
	if status != http.StatusServiceUnavailable || !strings.Contains(body, apperr.CatalogUnavailable().Message) || strings.Contains(body, "connection") {
		t.Fatalf("offline search: %d %s", status, body)
	}

	status, body = post(t, client, srv.URL+"/api/queue", `{"titleId":`+itoa(showID)+`}`)
	if status != http.StatusCreated {
		t.Fatalf("add show: %d %s", status, body)
	}
	status, _ = post(t, client, srv.URL+"/api/queue", `{"titleId":`+itoa(showID)+`}`)
	if status != http.StatusConflict {
		t.Fatalf("dup queue %d", status)
	}
	status, body = post(t, client, srv.URL+"/api/queue", `{"titleId":`+itoa(filmID)+`}`)
	if status != http.StatusCreated {
		t.Fatalf("add film: %d %s", status, body)
	}
	var added struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &added); err != nil {
		t.Fatal(err)
	}
	status, body = patch(t, client, srv.URL+"/api/queue/"+itoa(added.ID), `{"direction":"up"}`)
	if status != http.StatusOK {
		t.Fatalf("move: %d %s", status, body)
	}
	status, body = get(t, client, srv.URL+"/api/queue")
	if !strings.Contains(body, `"position":1`) || !strings.HasPrefix(strings.TrimSpace(body), "{") {
		t.Fatal(body)
	}
	var listed struct {
		Items []struct {
			Name     string `json:"name"`
			Position int    `json:"position"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 2 || listed.Items[0].Name != "Test Film" || listed.Items[1].Name != "Test Show" {
		t.Fatalf("%+v", listed.Items)
	}

	status, body = post(t, client, srv.URL+"/api/plan", `{"hours":1,"minutes":30}`)
	if status != http.StatusOK {
		t.Fatalf("plan: %d %s", status, body)
	}
	var planBody struct {
		Lines []struct {
			Title    string `json:"title"`
			Outcome  string `json:"outcome"`
			Episodes int    `json:"episodes"`
			Minutes  int    `json:"minutes"`
		} `json:"lines"`
		SpentMinutes int `json:"spentMinutes"`
		LeftMinutes  int `json:"leftMinutes"`
		QueueMinutes int `json:"queueMinutes"`
	}
	if err := json.Unmarshal([]byte(body), &planBody); err != nil {
		t.Fatal(err)
	}
	if len(planBody.Lines) != 2 || planBody.Lines[0].Title != "Test Film" || planBody.Lines[0].Outcome != "full" || planBody.Lines[1].Title != "Test Show" || planBody.Lines[1].Outcome != "stopped" || planBody.SpentMinutes != 80 || planBody.LeftMinutes != 10 || planBody.QueueMinutes != 260 {
		t.Fatalf("%+v", planBody)
	}

	status, body = post(t, client, srv.URL+"/api/plan", `{"hours":3,"minutes":0}`)
	if status != http.StatusOK {
		t.Fatalf("longer plan: %d %s", status, body)
	}
	if err := json.Unmarshal([]byte(body), &planBody); err != nil {
		t.Fatal(err)
	}
	if len(planBody.Lines) != 2 || planBody.Lines[1].Outcome != "partial" || planBody.Lines[1].Episodes != 2 || planBody.Lines[1].Minutes != 90 || planBody.SpentMinutes != 170 || planBody.LeftMinutes != 10 {
		t.Fatalf("partial %+v", planBody)
	}

	status, body = patch(t, client, srv.URL+"/api/rhythm", `{"dailyMinutes":40}`)
	if status != http.StatusOK {
		t.Fatalf("rhythm: %d %s", status, body)
	}
	var rhythm struct {
		DailyMinutes   int    `json:"dailyMinutes"`
		CurrentTitle   string `json:"currentTitle"`
		CurrentMinutes int    `json:"currentMinutes"`
		CurrentOffset  int    `json:"currentOffset"`
		HasCurrent     bool   `json:"hasCurrent"`
		QueueMinutes   int    `json:"queueMinutes"`
		QueueOffset    int    `json:"queueOffset"`
		HasQueue       bool   `json:"hasQueue"`
	}
	if err := json.Unmarshal([]byte(body), &rhythm); err != nil {
		t.Fatal(err)
	}
	if !rhythm.HasCurrent || rhythm.CurrentTitle != "Test Film" || rhythm.CurrentMinutes != 80 || rhythm.CurrentOffset != 1 || !rhythm.HasQueue || rhythm.QueueMinutes != 260 || rhythm.QueueOffset != 6 || rhythm.DailyMinutes != 40 {
		t.Fatalf("%+v", rhythm)
	}
	status, body = get(t, client, srv.URL+"/api/queue")
	if !strings.Contains(body, `"currentTitle":"Test Film"`) || !strings.Contains(body, `"dailyMinutes":40`) {
		t.Fatalf("queue rhythm: %s", body)
	}
	status, body = patch(t, client, srv.URL+"/api/rhythm", `{"dailyMinutes":1441}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "1440") {
		t.Fatalf("bad pace: %d %s", status, body)
	}

	status, body = post(t, client, srv.URL+"/api/plan", `{"hours":0,"minutes":0}`)
	if status != http.StatusBadRequest || !strings.Contains(body, apperr.InvalidFreeTime().Message) {
		t.Fatalf("bad time: %d %s", status, body)
	}

	status, _ = do(t, client, http.MethodDelete, srv.URL+"/api/queue/"+itoa(added.ID), "")
	if status != http.StatusNoContent {
		t.Fatalf("delete %d", status)
	}
	status, _ = post(t, client, srv.URL+"/api/logout", ``)
	if status != http.StatusNoContent {
		t.Fatalf("logout %d", status)
	}
	status, _ = get(t, client, srv.URL+"/api/queue")
	if status != http.StatusUnauthorized {
		t.Fatalf("after logout %d", status)
	}
}

func TestPortBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	err = ListenAndServe(ln.Addr().String(), http.NewServeMux())
	if apperr.From(err).Code != apperr.CodePortBusy {
		t.Fatal(err)
	}
}

func post(t *testing.T, client *http.Client, url, body string) (int, string) {
	t.Helper()
	return do(t, client, http.MethodPost, url, body)
}

func TestManualCard(t *testing.T) {
	db := storage.OpenTest(t)
	handler := New(auth.New(db), catalog.New(db, catalog.NewHTTPClient("http://127.0.0.1:9")), queue.New(db), io.Discard, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := &http.Client{Jar: jar}
	status, body := post(t, owner, srv.URL+"/api/register", `{"username":"ann","password":"password","email":"ann@example.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("register: %d %s", status, body)
	}
	status, body = post(t, owner, srv.URL+"/api/login", `{"username":"ann","password":"password"}`)
	if status != http.StatusOK {
		t.Fatalf("login: %d %s", status, body)
	}
	status, body = post(t, owner, srv.URL+"/api/titles", `{"name":"Домашний вечер","year":1999,"episodeCount":1,"averageMinutes":90}`)
	if status != http.StatusCreated || !strings.Contains(body, `"kind":"film"`) || !strings.Contains(body, `"year":1999`) {
		t.Fatalf("create: %d %s", status, body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.ID == 0 {
		t.Fatal(err, body)
	}
	status, body = get(t, owner, srv.URL+"/api/titles?q="+url.QueryEscape("домаш"))
	if status != http.StatusOK || !strings.Contains(body, "Домашний вечер") {
		t.Fatalf("search own: %d %s", status, body)
	}
	status, body = post(t, owner, srv.URL+"/api/titles", `{"name":"x","year":1999,"episodeCount":1,"averageMinutes":90}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "Название:") {
		t.Fatalf("short name: %d %s", status, body)
	}
	status, body = post(t, owner, srv.URL+"/api/titles", `{"name":"Сериал дома","year":2011,"episodeCount":4,"averageMinutes":25}`)
	if status != http.StatusCreated || !strings.Contains(body, `"kind":"series"`) {
		t.Fatalf("series: %d %s", status, body)
	}
	var series struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &series); err != nil {
		t.Fatal(err)
	}
	status, body = get(t, owner, srv.URL+"/api/titles/"+itoa(series.ID))
	if status != http.StatusOK || strings.Count(body, `"runtime":25`) != 4 {
		t.Fatalf("episodes: %d %s", status, body)
	}

	otherJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	other := &http.Client{Jar: otherJar}
	status, body = post(t, other, srv.URL+"/api/register", `{"username":"bob","password":"password","email":"bob@example.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("bob: %d %s", status, body)
	}
	status, body = post(t, other, srv.URL+"/api/login", `{"username":"bob","password":"password"}`)
	if status != http.StatusOK {
		t.Fatalf("bob login: %d %s", status, body)
	}
	status, body = get(t, other, srv.URL+"/api/titles?q="+url.QueryEscape("домаш"))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("hidden search: %d %s", status, body)
	}
	status, body = get(t, other, srv.URL+"/api/titles/"+itoa(created.ID))
	if status != http.StatusNotFound {
		t.Fatalf("hidden card: %d %s", status, body)
	}
	status, body = post(t, other, srv.URL+"/api/queue", `{"titleId":`+itoa(created.ID)+`}`)
	if status != http.StatusNotFound {
		t.Fatalf("hidden queue: %d %s", status, body)
	}

	status, body = post(t, owner, srv.URL+"/api/queue", `{"titleId":`+itoa(created.ID)+`}`)
	if status != http.StatusCreated || !strings.Contains(body, `"remainingMinutes":90`) {
		t.Fatalf("queue: %d %s", status, body)
	}
}

func TestPages(t *testing.T) {
	db := storage.OpenTest(t)
	handler := New(auth.New(db), catalog.New(db, catalog.NewHTTPClient("http://127.0.0.1:9")), queue.New(db), io.Discard, web.Files)
	srv := httptest.NewServer(handler)
	defer srv.Close()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/queue.html" {
		t.Fatalf("root: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	page, err := client.Get(srv.URL + "/queue.html")
	if err != nil {
		t.Fatal(err)
	}
	pageBody, err := io.ReadAll(page.Body)
	page.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if page.StatusCode != http.StatusOK || !strings.Contains(string(pageBody), "Очередь") || !strings.Contains(string(pageBody), "js/app.js") || page.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("queue page: %d %s", page.StatusCode, page.Header.Get("Cache-Control"))
	}
	status, body := get(t, client, srv.URL+"/css/app.css")
	if status != http.StatusOK || !strings.Contains(body, "--bg") {
		t.Fatalf("css: %d", status)
	}
	status, body = get(t, client, srv.URL+"/api/queue")
	if status != http.StatusUnauthorized || !strings.Contains(body, apperr.Unauthorized().Message) {
		t.Fatalf("api beside pages: %d %s", status, body)
	}
}

func patch(t *testing.T, client *http.Client, url, body string) (int, string) {
	t.Helper()
	return do(t, client, http.MethodPatch, url, body)
}

func TestAccountEmail(t *testing.T) {
	db := storage.OpenTest(t)
	ctx := context.Background()
	accounts := auth.New(db)
	if _, err := accounts.Register(ctx, "ann", "password", "ann@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.Register(ctx, "bob", "password", "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	handler := New(accounts, catalog.New(db, catalog.NewHTTPClient("http://127.0.0.1:9")), queue.New(db), io.Discard, web.Files)
	srv := httptest.NewServer(handler)
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	status, body := post(t, client, srv.URL+"/api/login", `{"username":"ann","password":"password"}`)
	if status != http.StatusOK {
		t.Fatalf("login %d %s", status, body)
	}
	status, body = get(t, client, srv.URL+"/api/me")
	if status != http.StatusOK || !strings.Contains(body, `"email":"ann@example.com"`) {
		t.Fatalf("me %d %s", status, body)
	}
	status, body = patch(t, client, srv.URL+"/api/me", `{"email":"Ann2@Example.com"}`)
	if status != http.StatusOK || !strings.Contains(body, `"email":"ann2@example.com"`) {
		t.Fatalf("save %d %s", status, body)
	}
	status, body = patch(t, client, srv.URL+"/api/me", `{"email":"bob@example.com"}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "Такой адрес уже есть.") {
		t.Fatalf("taken %d %s", status, body)
	}
	status, body = patch(t, client, srv.URL+"/api/me", `{"email":""}`)
	if status != http.StatusOK || !strings.Contains(body, `"email":""`) {
		t.Fatalf("clear %d %s", status, body)
	}
	page, err := client.Get(srv.URL + "/queue.html")
	if err != nil {
		t.Fatal(err)
	}
	pageBody, err := io.ReadAll(page.Body)
	page.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if page.StatusCode != http.StatusOK || !strings.Contains(string(pageBody), "account-mail") {
		t.Fatalf("queue mail form: %d", page.StatusCode)
	}
}

func TestLoginCookieIsSecureBehindHTTPS(t *testing.T) {
	db := storage.OpenTest(t)
	if _, err := auth.New(db).Register(context.Background(), "ann", "password", "ann@example.com"); err != nil {
		t.Fatal(err)
	}
	handler := New(auth.New(db), catalog.New(db, catalog.NewHTTPClient("http://127.0.0.1:9")), queue.New(db), io.Discard, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/login", strings.NewReader(`{"username":"ann","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var secure bool
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session" && cookie.Secure && cookie.HttpOnly {
			secure = true
		}
	}
	if resp.StatusCode != http.StatusOK || !secure {
		t.Fatalf("status %d cookies %+v", resp.StatusCode, resp.Cookies())
	}
}

func TestGuardStopsRepeatedLogin(t *testing.T) {
	handler := Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), &limit.Memory{})
	var last int
	for i := 0; i < 21; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "10.0.0.8:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status %d", last)
	}
}

func get(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	return do(t, client, http.MethodGet, url, "")
}

func do(t *testing.T, client *http.Client, method, url, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(payload)
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
