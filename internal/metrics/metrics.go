package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Metrics struct {
	mu       sync.Mutex
	codes    map[string]int
	seconds  float64
	requests int
}

func Wrap(next http.Handler) http.Handler {
	m := &Metrics{codes: map[string]int{}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" && r.Method == http.MethodGet {
			m.write(w)
			return
		}
		rec := &recorder{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		m.observe(rec.code, time.Since(start))
	})
}

func (m *Metrics) observe(code int, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.codes == nil {
		m.codes = map[string]int{}
	}
	m.codes[strconv.Itoa(code)]++
	m.requests++
	m.seconds += d.Seconds()
}

func (m *Metrics) write(w http.ResponseWriter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# TYPE gloaming_http_requests_total counter")
	for code, n := range m.codes {
		fmt.Fprintf(w, "gloaming_http_requests_total{code=%q} %d\n", code, n)
	}
	fmt.Fprintln(w, "# TYPE gloaming_http_request_seconds_sum gauge")
	fmt.Fprintf(w, "gloaming_http_request_seconds_sum %g\n", m.seconds)
	fmt.Fprintln(w, "# TYPE gloaming_http_request_seconds_count counter")
	fmt.Fprintf(w, "gloaming_http_request_seconds_count %d\n", m.requests)
}

type recorder struct {
	http.ResponseWriter
	code int
	set  bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.set {
		r.code = code
		r.set = true
	}
	r.ResponseWriter.WriteHeader(code)
}
