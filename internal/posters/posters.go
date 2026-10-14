package posters

import (
	"context"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type Store struct {
	dir string
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) SaveRemote(ctx context.Context, titleID int64, rawURL string) (string, error) {
	if titleID == 0 || rawURL == "" || strings.HasPrefix(rawURL, "/posters/") {
		return rawURL, nil
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return rawURL, nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Gloaming/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	ext := ".jpg"
	switch resp.Header.Get("Content-Type") {
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	}
	name := strconv.FormatInt(titleID, 10) + ext
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", nil
	}
	if err := os.WriteFile(path.Join(s.dir, name), body, 0o644); err != nil {
		return "", err
	}
	return "/posters/" + name, nil
}

func Mount(dir string, next http.Handler) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/posters/") {
			next.ServeHTTP(w, r)
			return
		}
		clone := r.Clone(r.Context())
		clone.URL.Path = strings.TrimPrefix(r.URL.Path, "/posters")
		files.ServeHTTP(w, clone)
	})
}
