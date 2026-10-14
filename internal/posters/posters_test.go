package posters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
)

func TestSaveRemoteCopiesTheImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	local, err := store.SaveRemote(context.Background(), 7, srv.URL)
	if err != nil || local != "/posters/7.png" {
		t.Fatalf("%q %v", local, err)
	}
	body, err := os.ReadFile(path.Join(dir, "7.png"))
	if err != nil || string(body) != "png" {
		t.Fatalf("%q %v", body, err)
	}
	again, err := store.SaveRemote(context.Background(), 7, local)
	if err != nil || again != local {
		t.Fatalf("local url %q %v", again, err)
	}
}
