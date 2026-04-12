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
	"log/slog"
	"mezzo/internal/cache"
	"mezzo/internal/client"
	"mezzo/internal/config"
	"mezzo/internal/scraper"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// CacheControl is the Cache-Control header value used for static assets
	CacheControl = "max-age=604800"
)

// Server wires routes, templates, and the asset filesystem
type Server struct {
	router       *http.ServeMux
	templates    map[string]*template.Template
	patchesURL   string
	assets       fs.FS
	version      string
	gifCache     *cache.Cache[*scraper.GifPage]
	searchCache  *cache.Cache[*scraper.SearchResponse]
	profileCache *cache.Cache[*scraper.ProfilePage]
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
	ImageURL        string
	Author          string
	AuthorURL       string
	AuthorAvatar    string
	AuthorIsPartner bool
	UploadDate      string
	Tags            []string
	Description     string
	FileSize        string
	Duration        string
	Dimensions      string
	Created         string
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

// ProfilePageData contains template data for the user profile page
type ProfilePageData struct {
	BasePage
	Username    string
	DisplayName string
	Tagline     string
	AvatarURL   string
	BannerURL   string
	UserType    string
	SocialLinks []scraper.SocialLink
	GIFs        []scraper.SearchResult
}

// New creates a new Server instance
func New(assets fs.FS, version string, cfg *config.Config) (*Server, error) {
	s := &Server{
		router:     http.NewServeMux(),
		patchesURL: cfg.PatchesURL,
		assets:     assets,
		templates:  make(map[string]*template.Template),
		version:    version,
	}

	if !cfg.CacheDisabled {
		s.gifCache = cache.New[*scraper.GifPage](cfg.CacheGifTTL, cfg.CacheGifMax)
		s.searchCache = cache.New[*scraper.SearchResponse](cfg.CacheSearchTTL, cfg.CacheSearchMax)
		s.profileCache = cache.New[*scraper.ProfilePage](cfg.CacheProfileTTL, cfg.CacheProfileMax)
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
	pages := []string{"index.html", "view.html", "search.html", "error.html", "profile.html"}
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

// responseWriter wraps http.ResponseWriter to capture the status code for logging.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
	s.router.ServeHTTP(rw, r)

	args := []any{
		"method", r.Method,
		"path", r.URL.RequestURI(),
		"status", rw.status,
		"duration", time.Since(start),
	}
	if strings.HasPrefix(r.URL.Path, "/static/") {
		slog.Debug("request", args...)
	} else {
		slog.Info("request", args...)
	}
}

func (s *Server) routes() {
	// Application routes
	s.router.HandleFunc("GET /view/{slug...}", s.handleView)
	s.router.HandleFunc("GET /search", s.handleSearchRedirect)
	s.router.HandleFunc("GET /search/{slug...}", s.handleSearch)
	s.router.HandleFunc("GET /users/{username}", s.handleProfile)
	s.router.HandleFunc("GET /official/{username}", s.handleProfile)
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
			if action == "users" || action == "official" {
				s.handleProfile(w, r)
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
	cacheStatus := "MISS"

	// Check cache first
	var gifPage *scraper.GifPage
	var err error

	if s.gifCache != nil {
		if cached, ok := s.gifCache.Get(path); ok {
			gifPage = cached
			cacheStatus = "HIT"
		}
	}

	if gifPage == nil {
		gifPage, err = scraper.GetGif(path)
		if err != nil {
			slog.Error("failed to scrape GIF page", "error", err)
			s.handleError(w, http.StatusInternalServerError)
			return
		}

		if s.gifCache != nil {
			s.gifCache.Set(path, gifPage)
		}
	}

	w.Header().Set("Mezzo-Cache", cacheStatus)

	data := ViewPage{
		BasePage: BasePage{
			Title:      gifPage.Title,
			OGImage:    scraper.Proxy(gifPage.ImageURL),
			PatchesURL: s.patchesURL,
			Version:    s.version,
			Host:       r.Host,
			Scheme:     scheme,
		},
		ImageURL:        gifPage.ImageURL,
		Author:          gifPage.Author,
		AuthorURL:       gifPage.AuthorURL,
		AuthorAvatar:    gifPage.AuthorAvatar,
		AuthorIsPartner: strings.HasPrefix(gifPage.AuthorURL, "/official/"),
		UploadDate:      gifPage.UploadDate,
		Tags:            gifPage.Tags,
		Description:     gifPage.Description,
		FileSize:        gifPage.FileSize,
		Duration:        gifPage.Duration,
		Dimensions:      gifPage.Dimensions,
		Created:         gifPage.Created,
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

	cacheStatus := "MISS"
	cacheKey := query + "\x00" + pos

	// Check cache first
	var searchResp *scraper.SearchResponse

	if s.searchCache != nil {
		if cached, ok := s.searchCache.Get(cacheKey); ok {
			searchResp = cached
			cacheStatus = "HIT"
		}
	}

	if searchResp == nil {
		var err error
		searchResp, err = scraper.APISearch(query, pos)
		if err != nil {
			// Fallback to HTML scraping (first page only, no pagination)
			slog.Warn("tenor API failed, falling back to HTML scraping", "error", err)
			results, scrapeErr := scraper.GetSearch(path)
			if scrapeErr != nil {
				slog.Error("HTML scraping fallback also failed", "error", scrapeErr)
				s.handleError(w, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Mezzo-Cache", "BYPASS")
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

		if s.searchCache != nil {
			s.searchCache.Set(cacheKey, searchResp)
		}
	}

	w.Header().Set("Mezzo-Cache", cacheStatus)

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

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	path := r.URL.Path
	cacheStatus := "MISS"

	var profilePage *scraper.ProfilePage
	var err error

	if s.profileCache != nil {
		if cached, ok := s.profileCache.Get(path); ok {
			profilePage = cached
			cacheStatus = "HIT"
		}
	}

	if profilePage == nil {
		profilePage, err = scraper.GetProfile(path)
		if err != nil {
			slog.Error("failed to scrape profile page", "error", err)
			s.handleError(w, http.StatusInternalServerError)
			return
		}

		if s.profileCache != nil {
			s.profileCache.Set(path, profilePage)
		}
	}

	w.Header().Set("Mezzo-Cache", cacheStatus)

	displayName := profilePage.DisplayName
	if displayName == "" {
		displayName = profilePage.Username
	}

	data := ProfilePageData{
		BasePage: BasePage{
			Title:      displayName,
			PatchesURL: s.patchesURL,
			Version:    s.version,
			Host:       r.Host,
			Scheme:     scheme,
		},
		Username:    profilePage.Username,
		DisplayName: displayName,
		Tagline:     profilePage.Tagline,
		AvatarURL:   profilePage.AvatarURL,
		BannerURL:   profilePage.BannerURL,
		UserType:    profilePage.UserType,
		SocialLinks: profilePage.SocialLinks,
		GIFs:        profilePage.GIFs,
	}

	s.render(w, "profile.html", data)
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
		slog.Error("failed to fetch proxy URL", "error", err)
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
		slog.Error("failed to copy proxy body", "error", err)
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
		slog.Error("template not found", "name", name)
		return
	}

	err := tmpl.ExecuteTemplate(w, "base", data)
	if err != nil {
		slog.Error("failed to execute template", "name", name, "error", err)
	}
}
