package search

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"time"

	//TODO remove this
	"github.com/caddyserver/caddy/caddyhttp/httpserver"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/caddyserver/caddy/v2/modules/caddy-search/indexer"
)

// ServerHTTP is the HTTP handler for this middleware
func (s *Search) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if r.URL.Path == s.Endpoint {
		if r.Header.Get("Accept") == "application/json" || s.Template == nil {
			return s.SearchJSON(w, r)
		}
		return s.SearchHTML(w, r)
	}

	record := s.Indexer.Record(GetUrlPath(r.URL))

	err := next.ServeHTTP(&searchResponseWriter{w, record}, r)

	modif := w.Header().Get("Last-Modified")
	if len(modif) > 0 {
		modTime, err := time.Parse(`Mon, 2 Jan 2006 15:04:05 MST`, modif)
		if err == nil {
			record.SetModified(modTime)
		}
	}

	s.IndexManager.Feed(record)

	return err
}

// Result is the structure for the search result
type Result struct {
	Path     string
	Title    string
	Body     template.HTML
	Json     string
	Modified time.Time
	Indexed  time.Time
	From     int
	Size     int
}

// SearchJSON renders the search results in JSON format
func (s *Search) SearchJSON(w http.ResponseWriter, r *http.Request) error {
	qry := r.URL.Query()
	q := qry.Get("q")
	from := 0
	size := 100
	if f, err := strconv.Atoi(qry.Get("f")); err == nil {
		from = f
	}
	if s, err := strconv.Atoi(qry.Get("s")); err == nil {
		size = s
	}
	indexResult := s.Indexer.Search(q, from, size)

	results := make([]Result, len(indexResult))

	for i, result := range indexResult {
		body := result.Body()
		results[i] = Result{
			Path:     result.Path(),
			Title:    result.Title(),
			Modified: result.Modified(),
			Indexed:  result.Indexed(),
			Json:     string(body),
			From:     from,
			Size:     size,
		}
	}

	jresp, err := json.Marshal(results)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	w.Write(jresp)
	return err
}

// SearchHTML renders the search results in the HTML template
func (s *Search) SearchHTML(w http.ResponseWriter, r *http.Request) error {
	qry := r.URL.Query()
	q := qry.Get("q")
	from := 0
	size := 100
	if f, err := strconv.Atoi(qry.Get("f")); err == nil {
		from = f
	}
	if s, err := strconv.Atoi(qry.Get("s")); err == nil {
		size = s
	}

	indexResult := s.Indexer.Search(q, from, size)

	results := make([]Result, len(indexResult))

	for i, result := range indexResult {
		results[i] = Result{
			Path:     result.Path(),
			Title:    result.Title(),
			Modified: result.Modified(),
			Body:     template.HTML(result.Body()),
			From:     from,
			Size:     size,
		}
	}

	qresults := QueryResults{
		Context: httpserver.Context{
			Root: http.Dir(s.SiteRoot),
			Req:  r,
			URL:  r.URL,
		},
		Query:   q,
		Results: results,
	}

	var buf bytes.Buffer
	err := s.Template.Execute(&buf, qresults)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	buf.WriteTo(w)
	return nil
}

type QueryResults struct {
	httpserver.Context
	Query   string
	Results []Result
}

type searchResponseWriter struct {
	w      http.ResponseWriter
	record indexer.Record
}

func (r *searchResponseWriter) Header() http.Header {
	return r.w.Header()
}

func (r *searchResponseWriter) WriteHeader(code int) {
	if code != http.StatusOK {
		r.record.Ignore()
	}
	r.w.WriteHeader(code)
}

func (r *searchResponseWriter) Write(p []byte) (int, error) {
	r.record.Write(p)
	n, err := r.w.Write(p)
	return n, err
}
