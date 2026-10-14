package httpapi

import (
	"net"
	"net/http"
	"strings"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/limit"
)

func Guard(next http.Handler, gate limit.Gate) http.Handler {
	if gate == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		max, key := limitOf(r)
		if max > 0 && !gate.Allow(r.Context(), key+":"+clientIP(r), max) {
			writeErrStatus(w, apperr.TooMany())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func limitOf(r *http.Request) (int, string) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/login":
		return 20, "login"
	case r.Method == http.MethodPost && r.URL.Path == "/api/register":
		return 10, "register"
	case r.Method == http.MethodPost && r.URL.Path == "/api/password/forgot":
		return 5, "forgot"
	case r.Method == http.MethodGet && r.URL.Path == "/api/titles":
		return 30, "search"
	default:
		return 0, ""
	}
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeErrStatus(w http.ResponseWriter, err *apperr.Error) {
	writeJSON(w, statusOf(err.Code), map[string]any{
		"error": map[string]string{"code": err.Code, "message": err.Message},
	})
}
