/*
   Copyright (C) 2026 FSKY <development@fsky.io>

   This file is part of Mezzo

   Mezzo is free software: you can redistribute it and/or modify it under the
   terms of the GNU Affero General Public License as published by the Free
   Software Foundation, either version 3 of the License, or (at your option) any
   later version.

   This program is distributed in the hope that it will be useful, but WITHOUT
   ANY WARRANTY; without even the implied warranty of  MERCHANTABILITY or
   FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Affero General Public License
   for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package server

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mezzo/internal/client"
	"mezzo/internal/scraper"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// CacheControl is the Cache-Control header value used for static assets
	CacheControl = "max-age=604800"
)

// Server wires routes, templates, and the asset filesystem
type Server struct {
	router     *http.ServeMux
	templates  map[string]*template.Template
	patchesURL string
	assets     fs.FS
	version    string
}

// BasePage contains template data shared by all pages
type BasePage struct {
	Title      string
	OGImage    string
	PatchesURL string
	Version    string
	Host       string
	Scheme     string
	Query      string
}

// ViewPage contains template data for the GIF view page
type ViewPage struct {
	BasePage
	ImageURL     string
	Author       string
	AuthorURL    string
	AuthorAvatar string
	UploadDate   string
	Tags         []string
	Description  string
	FileSize     string
	Duration     string
	Dimensions   string
	Created      string
}

// SearchPage contains template data for the search results page
type SearchPage struct {
	BasePage
	Results []scraper.SearchResult
	NextURL string
	PrevURL string
}

// ErrorPage contains template data for error pages
type ErrorPage struct {
	BasePage
	Code int
}

// New creates a new Server instance
func New(assets fs.FS, version string) (*Server, error) {
	s := &Server{
		router:     http.NewServeMux(),
		patchesURL: os.Getenv("PATCHES_URL"),
		assets:     assets,
		templates:  make(map[string]*template.Template),
		version:    version,
	}

	funcMap := template.FuncMap{
		"proxy": func(u string) string {
			return scraper.Proxy(u)
		},
		"formatDate": func(s string) string {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return s
			}
			return t.Format("2006-01-02 15:04:05")
		},
	}

	// Parse the shared base template
	baseTmpl, err := template.New("base.html").Funcs(funcMap).ParseFS(assets, "templates/base.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse base template: %v", err)
	}

	// Parse each page template on top of the base template
	pages := []string{"index.html", "view.html", "search.html", "error.html"}
	for _, page := range pages {
		tmpl, err := baseTmpl.Clone()
		if err != nil {
			return nil, fmt.Errorf("failed to clone base template for %s: %v", page, err)
		}
		_, err = tmpl.ParseFS(assets, "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %v", page, err)
		}
		s.templates[page] = tmpl
	}

	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s: %s", time.Now().Format(time.RFC3339), r.URL.String())
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Application routes
	s.router.HandleFunc("GET /view/{slug...}", s.handleView)
	s.router.HandleFunc("GET /search", s.handleSearchRedirect)
	s.router.HandleFunc("GET /search/{slug...}", s.handleSearch)
	s.router.HandleFunc("GET /proxy.gif", s.handleProxy)

	// Serve everything in static/ under /static/
	staticFS, _ := fs.Sub(s.assets, "static")
	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
	s.router.Handle("GET /static/", fileServer)

	// Fallback for index and locale-prefixed routes
	s.router.HandleFunc("GET /", s.handleCatchAll)
}

func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if path == "/" {
		s.handleIndex(w, r)
		return
	}

	// Locale-prefixed routes look like /xx-xx/view/... or /xx-xx/search/....
	parts := strings.Split(path, "/")
	// Split("/xx-xx/view/...") => ["", "xx-xx", "view", ...]
	if len(parts) >= 3 {
		locale := parts[1]
		if len(locale) == 5 && locale[2] == '-' {
			action := parts[2]
			if action == "view" {
				s.handleView(w, r)
				return
			}
			if action == "search" {
				// Redirect when no search slug is present
				if len(parts) > 3 && parts[3] != "" {
					s.handleSearch(w, r)
				} else {
					s.handleSearchRedirect(w, r)
				}
				return
			}
		}
	}

	s.handleError(w, http.StatusNotFound)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	data := BasePage{
		Title:      "Mezzo",
		PatchesURL: s.patchesURL,
		Version:    s.version,
		Host:       r.Host,
		Scheme:     scheme,
	}

	s.render(w, "index.html", data)
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	path := r.URL.Path

	gifPage, err := scraper.GetGif(path)
	if err != nil {
		log.Printf("Error scraping GIF page: %v", err)
		s.handleError(w, http.StatusInternalServerError)
		return
	}

	data := ViewPage{
		BasePage: BasePage{
			Title:      gifPage.Title,
			OGImage:    scraper.Proxy(gifPage.ImageURL),
			PatchesURL: s.patchesURL,
			Version:    s.version,
			Host:       r.Host,
			Scheme:     scheme,
		},
		ImageURL:     gifPage.ImageURL,
		Author:       gifPage.Author,
		AuthorURL:    gifPage.AuthorURL,
		AuthorAvatar: gifPage.AuthorAvatar,
		UploadDate:   gifPage.UploadDate,
		Tags:         gifPage.Tags,
		Description:  gifPage.Description,
		FileSize:     gifPage.FileSize,
		Duration:     gifPage.Duration,
		Dimensions:   gifPage.Dimensions,
		Created:      gifPage.Created,
	}

	s.render(w, "view.html", data)
}

func (s *Server) handleSearchRedirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if strings.Contains(q, "tenor.com/view/") {
		parts := strings.Split(q, "tenor.com/view/")
		if len(parts) == 2 && parts[1] != "" {
			http.Redirect(w, r, "/view/"+parts[1], http.StatusMovedPermanently)
			return
		}
	}

	slug := strings.ReplaceAll(q, " ", "-") + "-gifs"
	http.Redirect(w, r, "/search/"+slug, http.StatusMovedPermanently)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	pos := r.URL.Query().Get("pos")
	prev := r.URL.Query().Get("prev")

	query := queryFromSearchPath(path)
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	searchResp, err := scraper.APISearch(query, pos)
	if err != nil {
		// Fallback to HTML scraping (first page only, no pagination)
		log.Printf("Tenor API failed, falling back to HTML scraping: %v", err)
		results, scrapeErr := scraper.GetSearch(path)
		if scrapeErr != nil {
			log.Printf("HTML scraping fallback also failed: %v", scrapeErr)
			s.handleError(w, http.StatusInternalServerError)
			return
		}
		data := SearchPage{
			BasePage: BasePage{
				Title:      "Search",
				PatchesURL: s.patchesURL,
				Version:    s.version,
				Host:       r.Host,
				Query:      query,
			},
			Results: results,
		}
		s.render(w, "search.html", data)
		return
	}

	// Build the next page URL, carrying the current pos as prev
	var nextURL string
	if searchResp.Next != "" {
		nextURL = path + "?pos=" + url.QueryEscape(searchResp.Next)
		if pos != "" {
			nextURL += "&prev=" + url.QueryEscape(pos)
		}
	}

	// Build the previous page URL
	var prevURL string
	if pos != "" {
		if prev != "" {
			// Go back to the previous cursor position
			prevURL = path + "?pos=" + url.QueryEscape(prev)
		} else {
			// We're on page 2, previous is page 1 (no pos param)
			prevURL = path
		}
	}

	data := SearchPage{
		BasePage: BasePage{
			Title:      "Search",
			PatchesURL: s.patchesURL,
			Version:    s.version,
			Host:       r.Host,
			Query:      query,
		},
		Results: searchResp.Results,
		NextURL: nextURL,
		PrevURL: prevURL,
	}

	s.render(w, "search.html", data)
}

func queryFromSearchPath(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	slug := parts[len(parts)-1]
	if slug == "search" || slug == "" {
		return ""
	}
	slug = strings.TrimSuffix(slug, "-gifs")
	return strings.ReplaceAll(slug, "-", " ")
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		s.handleError(w, http.StatusBadRequest)
		return
	}

	// Allowlist Tenor media hosts
	if !strings.HasPrefix(targetURL, "https://media.tenor.com/") &&
		!strings.HasPrefix(targetURL, "https://media1.tenor.com/") &&
		!strings.HasPrefix(targetURL, "https://c.tenor.com/") {
		s.handleError(w, http.StatusBadRequest)
		return
	}

	resp, err := client.Default.Get(targetURL)
	if err != nil {
		log.Printf("Error fetching proxy URL: %v", err)
		s.handleError(w, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", CacheControl)

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
	}

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying proxy body: %v", err)
	}
}

func (s *Server) handleError(w http.ResponseWriter, code int) {
	// Write the status code and render the error template
	w.WriteHeader(code)
	data := ErrorPage{
		BasePage: BasePage{
			Title:      fmt.Sprintf("%d", code),
			PatchesURL: s.patchesURL,
			Version:    s.version,
			Host:       "",
		},
		Code: code,
	}
	s.render(w, "error.html", data)
}

func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	tmpl, ok := s.templates[name]
	if !ok {
		log.Printf("Template %s not found", name)
		return
	}

	err := tmpl.ExecuteTemplate(w, "base", data)
	if err != nil {
		log.Printf("Error executing template %s: %v", name, err)
	}
}
