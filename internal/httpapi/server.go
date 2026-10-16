package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/auth"
	"github.com/WaltzForLovers/Gloaming/internal/catalog"
	"github.com/WaltzForLovers/Gloaming/internal/plan"
	"github.com/WaltzForLovers/Gloaming/internal/queue"
)

const (
	sessionCookie = "session"
	sessionMaxAge = 7 * 24 * 60 * 60
)

type server struct {
	auth    *auth.Service
	catalog *catalog.Service
	queue   *queue.Service
	log     *log.Logger
}

func New(authSvc *auth.Service, catalogSvc *catalog.Service, queueSvc *queue.Service, logOut io.Writer, files fs.FS) http.Handler {
	if logOut == nil {
		logOut = io.Discard
	}
	s := &server{
		auth:    authSvc,
		catalog: catalogSvc,
		queue:   queueSvc,
		log:     log.New(logOut, "", log.LstdFlags),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("POST /api/password/forgot", s.forgotPassword)
	mux.HandleFunc("POST /api/password/reset", s.resetPassword)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("PATCH /api/me", s.setEmail)
	mux.HandleFunc("GET /api/titles", s.searchTitles)
	mux.HandleFunc("POST /api/titles", s.createTitle)
	mux.HandleFunc("GET /api/titles/{id}", s.getTitle)
	mux.HandleFunc("GET /api/queue", s.listQueue)
	mux.HandleFunc("POST /api/queue", s.addQueue)
	mux.HandleFunc("PATCH /api/queue/{id}", s.moveQueue)
	mux.HandleFunc("DELETE /api/queue/{id}", s.deleteQueue)
	mux.HandleFunc("POST /api/plan", s.plan)
	mux.HandleFunc("PATCH /api/rhythm", s.setRhythm)
	mux.HandleFunc("POST /api/episodes/{id}", s.setWatched)
	if files == nil {
		return mux
	}
	pages := staticHandler(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			mux.ServeHTTP(w, r)
			return
		}
		pages.ServeHTTP(w, r)
	})
}

func staticHandler(files fs.FS) http.Handler {
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/queue.html", http.StatusSeeOther)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." || strings.HasPrefix(name, "..") {
			http.NotFound(w, r)
			return
		}
		info, err := fs.Stat(files, name)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		server.ServeHTTP(w, r)
	})
}

func ListenAndServe(addr string, handler http.Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return apperr.PortBusy()
		}
		return apperr.Internal(err)
	}
	return (&http.Server{Handler: handler}).Serve(ln)
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	user, err := s.auth.Register(r.Context(), body.Username, body.Password, body.Email)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "username": user.Username})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	token, user, err := s.auth.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	http.SetCookie(w, sessionCookieOf(r, token, sessionMaxAge))
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "username": user.Username})
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	email, err := s.auth.Email(r.Context(), user.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "username": user.Username, "email": email})
}

func (s *server) setEmail(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	email, err := s.auth.SetEmail(r.Context(), user.ID, user.Username, body.Email)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			s.writeErr(w, err)
			return
		}
	}
	http.SetCookie(w, sessionCookieOf(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

func sessionCookieOf(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureRequest(r),
		MaxAge:   maxAge,
	}
}

func secureRequest(r *http.Request) bool {
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return os.Getenv("GLOAMING_SECURE") == "1"
}

func (s *server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.auth.Forgot(r.Context(), body.Email); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Если такой адрес есть, письмо уже отправлено."})
}

func (s *server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.auth.Reset(r.Context(), body.Token, body.Password); err != nil {
		s.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) searchTitles(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	cards, err := s.catalog.Search(r.Context(), r.URL.Query().Get("q"))
	if app := apperr.From(err); app != nil && app.Code == apperr.CodeValidation {
		s.writeErr(w, err)
		return
	}
	owned, ownErr := s.catalog.FindManual(r.Context(), user.ID, r.URL.Query().Get("q"))
	if ownErr != nil {
		s.writeErr(w, ownErr)
		return
	}
	if err != nil && len(owned) == 0 {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"titles": cardsJSON(catalog.MergeCards(owned, cards))})
}

func (s *server) createTitle(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var body struct {
		Name           string `json:"name"`
		Year           int    `json:"year"`
		EpisodeCount   int    `json:"episodeCount"`
		AverageMinutes int    `json:"averageMinutes"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	card, err := s.catalog.AddManual(r.Context(), user.ID, body.Name, body.Year, body.EpisodeCount, body.AverageMinutes)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cardJSON(card))
}

func (s *server) getTitle(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	card, seasons, err := s.catalog.Detail(r.Context(), user.ID, id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	body := cardJSON(card)
	body["seasons"] = seasonsJSON(seasons)
	writeJSON(w, http.StatusOK, body)
}

func (s *server) setWatched(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var body struct {
		Watched bool `json:"watched"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.catalog.SetWatched(r.Context(), user.ID, id, body.Watched); err != nil {
		s.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listQueue(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	items, rhythm, err := s.queueView(r, user.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	suggestions, err := s.catalog.Suggestions(r.Context(), user.ID)
	if err != nil {
		suggestions = nil
	}
	letter, err := s.catalog.Letter(r.Context(), user.ID)
	if err != nil {
		letter = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       itemsJSON(items),
		"rhythm":      rhythmJSON(rhythm),
		"suggestions": cardsJSON(suggestions),
		"letter":      letter,
	})
}

func (s *server) setRhythm(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var body struct {
		DailyMinutes int `json:"dailyMinutes"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.queue.SetDailyMinutes(r.Context(), user.ID, body.DailyMinutes); err != nil {
		s.writeErr(w, err)
		return
	}
	_ = s.catalog.ApplyWatched(r.Context(), user.ID, time.Now())
	_, rhythm, err := s.queueView(r, user.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rhythmJSON(rhythm))
}

func (s *server) queueView(r *http.Request, userID int64) ([]queue.Item, plan.Rhythm, error) {
	items, err := s.queue.List(r.Context(), userID)
	if err != nil {
		return nil, plan.Rhythm{}, err
	}
	daily, err := s.queue.DailyMinutes(r.Context(), userID)
	if err != nil {
		return nil, plan.Rhythm{}, err
	}
	return items, plan.RhythmOf(daily, planItems(items)), nil
}

func (s *server) addQueue(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var body struct {
		TitleID  int64 `json:"titleId"`
		SeasonID int64 `json:"seasonId"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	var item queue.Item
	if body.SeasonID > 0 {
		item, err = s.queue.AddSeason(r.Context(), user.ID, body.SeasonID)
	} else {
		item, err = s.queue.Add(r.Context(), user.ID, body.TitleID)
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemJSON(item))
}

func (s *server) moveQueue(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var body struct {
		Direction string `json:"direction"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.queue.Move(r.Context(), user.ID, id, body.Direction); err != nil {
		s.writeErr(w, err)
		return
	}
	items, err := s.queue.List(r.Context(), user.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": itemsJSON(items)})
}

func (s *server) deleteQueue(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.queue.Remove(r.Context(), user.ID, id); err != nil {
		s.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) plan(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var body struct {
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	}
	if err := readJSON(w, r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	free, err := plan.FreeMinutes(body.Hours, body.Minutes)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	items, err := s.queue.List(r.Context(), user.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	result, err := plan.Build(planItems(items), free)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	lines := make([]map[string]any, 0, len(result.Lines))
	for _, line := range result.Lines {
		lines = append(lines, map[string]any{
			"title":    line.Title,
			"outcome":  line.Outcome,
			"episodes": line.Episodes,
			"minutes":  line.Minutes,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lines":        lines,
		"spentMinutes": result.SpentMinutes,
		"leftMinutes":  result.LeftMinutes,
		"queueMinutes": result.QueueMinutes,
	})
}

func (s *server) currentUser(r *http.Request) (auth.User, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return auth.User{}, apperr.Unauthorized()
	}
	return s.auth.User(r.Context(), cookie.Value)
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Validation("Не удалось прочитать запрос.")
	}
	return id, nil
}

func readJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dest); err != nil {
		return apperr.Validation("Не удалось прочитать запрос.")
	}
	return nil
}

func (s *server) writeErr(w http.ResponseWriter, err error) {
	app := apperr.From(err)
	if app.Code == apperr.CodeInternal || app.Code == apperr.CodeCatalogUnavailable {
		if cause := errors.Unwrap(err); cause != nil {
			s.log.Printf("%s: %s (%v)", app.Code, app.Message, cause)
		} else {
			s.log.Printf("%s: %v", app.Code, err)
		}
	}
	writeJSON(w, statusOf(app.Code), map[string]any{
		"error": map[string]string{"code": app.Code, "message": app.Message},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func statusOf(code string) int {
	switch code {
	case apperr.CodeValidation, apperr.CodeInvalidFreeTime:
		return http.StatusBadRequest
	case apperr.CodeUnauthorized, apperr.CodeInvalidCredentials:
		return http.StatusUnauthorized
	case apperr.CodeTitleNotFound, apperr.CodeNotFound:
		return http.StatusNotFound
	case apperr.CodeUserExists, apperr.CodeAlreadyInQueue:
		return http.StatusConflict
	case apperr.CodeCatalogUnavailable:
		return http.StatusServiceUnavailable
	case apperr.CodeTooMany:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func cardsJSON(cards []catalog.Card) []map[string]any {
	out := make([]map[string]any, len(cards))
	for i, card := range cards {
		out[i] = cardJSON(card)
	}
	return out
}

func cardJSON(card catalog.Card) map[string]any {
	return map[string]any{
		"id":             card.ID,
		"externalId":     card.ExternalID,
		"name":           card.Name,
		"year":           card.Year,
		"episodeCount":   card.EpisodeCount,
		"averageMinutes": card.AverageMinutes,
		"kind":           card.Kind,
		"poster":         card.Poster,
		"assembling":     card.Assembling,
	}
}

func planItems(items []queue.Item) []plan.Item {
	input := make([]plan.Item, len(items))
	for i, item := range items {
		remaining := item.Remaining
		if remaining == nil {
			remaining = []int{}
		}
		input[i] = plan.Item{Title: item.Name, Remaining: remaining}
	}
	return input
}

func rhythmJSON(rhythm plan.Rhythm) map[string]any {
	return map[string]any{
		"dailyMinutes":   rhythm.DailyMinutes,
		"currentTitle":   rhythm.CurrentTitle,
		"currentMinutes": rhythm.CurrentMinutes,
		"currentOffset":  rhythm.CurrentOffset,
		"hasCurrent":     rhythm.HasCurrent,
		"queueMinutes":   rhythm.QueueMinutes,
		"queueOffset":    rhythm.QueueOffset,
		"hasQueue":       rhythm.HasQueue,
	}
}

func itemsJSON(items []queue.Item) []map[string]any {
	out := make([]map[string]any, len(items))
	for i, item := range items {
		out[i] = itemJSON(item)
	}
	return out
}

func seasonsJSON(seasons []catalog.SeasonCard) []map[string]any {
	out := make([]map[string]any, len(seasons))
	for i, season := range seasons {
		episodes := make([]map[string]any, len(season.Episodes))
		for j, episode := range season.Episodes {
			episodes[j] = map[string]any{
				"id":      episode.ID,
				"number":  episode.Number,
				"name":    episode.Name,
				"runtime": episode.Runtime,
				"watched": episode.Watched,
			}
		}
		out[i] = map[string]any{
			"id":       season.ID,
			"number":   season.Number,
			"episodes": episodes,
		}
	}
	return out
}

func itemJSON(item queue.Item) map[string]any {
	return map[string]any{
		"id":                item.ID,
		"position":          item.Position,
		"titleId":           item.TitleID,
		"seasonId":          item.SeasonID,
		"seasonNumber":      item.SeasonNumber,
		"name":              item.Name,
		"year":              item.Year,
		"episodeCount":      item.EpisodeCount,
		"seasonEpisodes":    item.SeasonEpisodes,
		"remainingEpisodes": item.RemainingEpisodes,
		"remainingMinutes":  item.RemainingMinutes,
		"averageMinutes":    item.AverageMinutes,
		"kind":              catalog.KindOf(item.EpisodeCount),
		"poster":            item.Poster,
	}
}
