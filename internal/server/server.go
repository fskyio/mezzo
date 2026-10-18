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
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"mezzo/internal/cache"
	"mezzo/internal/config"
	"mezzo/internal/mediacache"
	"mezzo/internal/scraper"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Server wires routes, templates, and the asset filesystem
type Server struct {
	router        *http.ServeMux
	templates     map[string]*template.Template
	patchesURL    string
	assets        fs.FS
	version       string
	gifCache      *cache.Cache[*scraper.GifPage]
	searchCache   *cache.Cache[*scraper.SearchResponse]
	profileCache  *cache.Cache[*scraper.ProfilePage]
	exploreCache  *cache.Cache[*scraper.SearchResponse]
	trendingCache *cache.Cache[[]scraper.Trend]
	// mediaStore is the optional on-disk copy of proxied media
	mediaStore         *mediacache.Store
	avatarCacheControl string
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
	Media           scraper.GifFormat
	Formats         []FormatOption
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
	Downloads       []DownloadOption
	// Related is a page of related GIFs. They start collapsed; RelatedOpen
	// expands them for pages reached through their pagination links.
	Related        []scraper.SearchResult
	RelatedOpen    bool
	RelatedPrevURL string
	RelatedNextURL string
}

// DownloadOption is one entry in the download menu on the view page
type DownloadOption struct {
	Label string
	Href  string // proxied URL that responds with Content-Disposition: attachment
}

// FormatOption is one tab in the SD GIF / HD GIF / MP4 switcher
type FormatOption struct {
	Label  string
	Href   string
	Src    string // proxied media URL, used by view.js to swap media in place
	Video  bool
	Active bool
}

// SearchPage contains template data for the search results page
type SearchPage struct {
	BasePage
	Results []scraper.SearchResult
	NextURL string
	PrevURL string
}

// ExplorePage contains template data for the explore page
type ExplorePage struct {
	BasePage
	Trends  []scraper.Trend
	Results []scraper.SearchResult
	NextURL string
	PrevURL string
}

// ErrorPage contains template data for error pages
type ErrorPage struct {
	BasePage
	Code     int
	Heading  string
	Message  string
	TenorURL string
}

// profileRowSize is the number of uploads in the Memes and Stickers rows of
// a profile page; "See all" links to the rest.
const profileRowSize = 8

// ProfileRow is a short row of one kind of upload on a profile page
type ProfileRow struct {
	Heading string
	SeeAll  string // path of the page that lists every upload of this kind
	Results []scraper.SearchResult
}

// ProfilePageData contains template data for the user profile page and for
// the pages that list all of a profile's stickers or memes
type ProfilePageData struct {
	BasePage
	Username    string
	DisplayName string
	Tagline     string
	AvatarURL   string
	BannerURL   string
	UserType    string
	SocialLinks []scraper.SocialLink
	CTA         *scraper.ProfileCTA
	// Rows are the Memes and Stickers previews, only on the first page of the
	// profile itself
	Rows []ProfileRow
	// Heading and Noun name what Results holds: "GIFs" and "GIFs, stickers or
	// memes" on the profile page itself, "Stickers" and "stickers" on its
	// sticker page
	Heading string
	Noun    string
	// BackURL is the profile page, set on the sticker and meme pages
	BackURL string
	Results []scraper.SearchResult
	NextURL string
	PrevURL string
}

// New creates a new Server instance
func New(assets fs.FS, version string, cfg *config.Config) (*Server, error) {
	s := &Server{
		router:     http.NewServeMux(),
		patchesURL: cfg.PatchesURL,
		assets:     assets,
		templates:  make(map[string]*template.Template),
		version:    version,
		// Avatars can change in place, so browsers revalidate them as often as
		// the server refetches them
		avatarCacheControl: fmt.Sprintf("public, max-age=%d", max(0, int(cfg.CacheAvatarTTL.Seconds()))),
	}

	if !cfg.CacheDisabled {
		s.gifCache = cache.New[*scraper.GifPage](cfg.CacheGifTTL, cfg.CacheGifMax)
		s.searchCache = cache.New[*scraper.SearchResponse](cfg.CacheSearchTTL, cfg.CacheSearchMax)
		s.profileCache = cache.New[*scraper.ProfilePage](cfg.CacheProfileTTL, cfg.CacheProfileMax)
		s.exploreCache = cache.New[*scraper.SearchResponse](cfg.CacheSearchTTL, cfg.CacheSearchMax)
		// There is only one trending list, so one slot is enough
		s.trendingCache = cache.New[[]scraper.Trend](cfg.CacheSearchTTL, 1)

		if cfg.CacheMediaDir != "" {
			if cfg.CacheMediaMaxMB <= 0 {
				return nil, errors.New("MEZZO_CACHE_MEDIA_MAX_MB must be positive")
			}
			if cfg.CacheMediaTTL <= 0 || cfg.CacheAvatarTTL <= 0 {
				return nil, errors.New("MEZZO_CACHE_MEDIA_TTL and MEZZO_CACHE_AVATAR_TTL must be positive")
			}
			store, err := mediacache.Open(cfg.CacheMediaDir, mediacache.Options{
				MaxBytes:     int64(cfg.CacheMediaMaxMB) << 20,
				MaxFileBytes: int64(cfg.CacheMediaMaxFileMB) << 20,
				TTLs:         map[string]time.Duration{kindMedia: cfg.CacheMediaTTL, kindAvatar: cfg.CacheAvatarTTL},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to open media cache: %v", err)
			}
			s.mediaStore = store
		}
	}

	funcMap := template.FuncMap{
		"media":      scraper.MediaPath,
		"avatar":     scraper.AvatarPath,
		"searchPath": searchPath,
		"tileTags":   tileTags,
		"downloads": func(r scraper.SearchResult) []DownloadOption {
			return downloadOptions(r.URL, r.Formats)
		},
		"formatDate": func(s string) string {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return s
			}
			return t.Format("2006-01-02 15:04:05")
		},
	}

	// Parse the shared base template and the partials that pages include
	baseTmpl, err := template.New("base.html").Funcs(funcMap).ParseFS(assets, "templates/base.html", "templates/tile.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse base template: %v", err)
	}

	// Parse each page template on top of the base template
	pages := []string{"index.html", "view.html", "search.html", "explore.html", "error.html", "profile.html"}
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
	s.router.HandleFunc("GET /explore", s.handleExplore)
	s.router.HandleFunc("GET /users/{username}", s.handleProfile)
	s.router.HandleFunc("GET /official/{username}", s.handleProfile)
	s.router.HandleFunc("GET /users/{username}/{kind}", s.handleProfile)
	s.router.HandleFunc("GET /official/{username}/{kind}", s.handleProfile)
	s.router.HandleFunc("GET "+scraper.MediaPrefix+"{host}/{path...}", s.handleMedia(kindMedia))
	s.router.HandleFunc("GET "+scraper.AvatarPrefix+"{host}/{path...}", s.handleMedia(kindAvatar))

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
			if action == "explore" {
				s.handleExplore(w, r)
				return
			}
			if action == "users" || action == "official" {
				s.handleProfile(w, r)
				return
			}
		}
	}

	s.handleError(w, r, http.StatusNotFound)
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
			s.handleError(w, r, scrapeStatusCode(err))
			return
		}

		if s.gifCache != nil {
			s.gifCache.Set(path, gifPage)
		}
	}

	query := r.URL.Query()
	media := gifPage.Format(query.Get("format"))

	// The first page of related GIFs comes with the view page; later ones
	// need the API
	pos := query.Get("pos")
	related := gifPage.Related
	if pos != "" && gifPage.ID != "" {
		more, hit, err := s.relatedGIFs(gifPage.ID, pos)
		if err != nil {
			slog.Error("tenor API failed for related GIFs", "error", err)
			s.handleError(w, r, http.StatusBadGateway)
			return
		}
		related = *more
		if !hit {
			cacheStatus = "MISS"
		}
	}

	// Keep the chosen format while paging through related GIFs
	format := ""
	if media.Key != gifPage.Formats[0].Key {
		format = media.Key
	}
	relatedPrev, relatedNext := relatedLinks(r.URL.EscapedPath(), format, pos, query.Get("prev"), related.Next)

	w.Header().Set("Mezzo-Cache", cacheStatus)

	data := ViewPage{
		BasePage: BasePage{
			Title:      gifPage.Title,
			OGImage:    scraper.MediaPath(gifPage.ImageURL),
			PatchesURL: s.patchesURL,
			Version:    s.version,
			Host:       r.Host,
			Scheme:     scheme,
		},
		Media:           media,
		Formats:         formatOptions(r.URL.EscapedPath(), gifPage.Formats, media.Key),
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
		Downloads:       downloadOptions(r.URL.EscapedPath(), gifPage.Formats),
		Related:         related.Results,
		RelatedOpen:     query.Has("pos"),
		RelatedPrevURL:  relatedPrev,
		RelatedNextURL:  relatedNext,
	}

	s.render(w, "view.html", data)
}

// relatedGIFs returns a page of the GIFs related to the GIF with the given ID
// after the first one, which comes with the view page. hit reports whether it
// was cached.
func (s *Server) relatedGIFs(id, pos string) (resp *scraper.SearchResponse, hit bool, err error) {
	key := "related\x00" + id + "\x00" + pos

	if s.searchCache != nil {
		if cached, ok := s.searchCache.Get(key); ok {
			return cached, true, nil
		}
	}

	resp, err = scraper.APIRelated(id, pos)
	if err != nil {
		return nil, false, err
	}

	if s.searchCache != nil {
		s.searchCache.Set(key, resp)
	}
	return resp, false, nil
}

// relatedLinks returns the Previous and Next links for the related GIFs on the
// view page at viewPath, like pageLinks does for other paginated results. They
// keep format, a non-default ?format= value, and jump to the related GIFs,
// which a pos parameter expands, so the link back to the first page carries an
// empty one.
func relatedLinks(viewPath, format, pos, prev, next string) (prevURL, nextURL string) {
	link := func(cursor, before string) string {
		q := url.Values{}
		if format != "" {
			q.Set("format", format)
		}
		q.Set("pos", cursor)
		if before != "" {
			q.Set("prev", before)
		}
		return viewPath + "?" + q.Encode() + "#related"
	}

	// Carry the current pos as prev so the next page can link back to it
	if next != "" {
		nextURL = link(next, pos)
	}
	if pos != "" {
		prevURL = link(prev, "")
	}
	return prevURL, nextURL
}

// formatOptions builds the format switcher links for the view page at viewPath.
// It returns nil when there is nothing to switch between.
func formatOptions(viewPath string, formats []scraper.GifFormat, active string) []FormatOption {
	if len(formats) < 2 {
		return nil
	}

	options := make([]FormatOption, 0, len(formats))
	for i, f := range formats {
		href := viewPath
		if i > 0 {
			// The first format is the default, so its link stays query-free
			href += "?format=" + f.Key
		}
		options = append(options, FormatOption{
			Label:  f.Label,
			Href:   href,
			Src:    scraper.MediaPath(f.URL),
			Video:  f.Video,
			Active: f.Key == active,
		})
	}
	return options
}

// maxTileTags is how many of a GIF's tags its tile shows on hover.
const maxTileTags = 3

// tileTags returns the tags a GIF tile shows on hover.
func tileTags(tags []string) []string {
	return tags[:min(len(tags), maxTileTags)]
}

// downloadOptions builds the download links for the GIF at viewPath, one per
// available format. They fill the download menu of the view page and of the
// GIF's tile.
func downloadOptions(viewPath string, formats []scraper.GifFormat) []DownloadOption {
	slug := path.Base(viewPath)
	if unescaped, err := url.PathUnescape(slug); err == nil {
		slug = unescaped
	}

	options := make([]DownloadOption, 0, len(formats))
	for _, f := range formats {
		stem := slug
		if !f.Video {
			// SD and HD GIFs share a file name on Tenor's servers
			stem += "-" + f.Key
		}
		options = append(options, DownloadOption{Label: f.Label, Href: scraper.MediaDownload(f.URL, stem)})
	}
	return options
}

func (s *Server) handleSearchRedirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if viewPath := tenorViewPath(q); viewPath != "" {
		http.Redirect(w, r, viewPath, http.StatusFound)
		return
	}

	http.Redirect(w, r, searchPath(q), http.StatusFound)
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
				s.handleError(w, r, scrapeStatusCode(scrapeErr))
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

	prevURL, nextURL := pageLinks(path, pos, prev, searchResp.Next)

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

func (s *Server) handleExplore(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	pos := r.URL.Query().Get("pos")
	prev := r.URL.Query().Get("prev")

	base := BasePage{
		Title:      "Explore",
		PatchesURL: s.patchesURL,
		Version:    s.version,
		Host:       r.Host,
	}

	cacheStatus := "MISS"
	var featured *scraper.SearchResponse

	if s.exploreCache != nil {
		if cached, ok := s.exploreCache.Get(pos); ok {
			featured = cached
			cacheStatus = "HIT"
		}
	}

	if featured == nil {
		var err error
		featured, err = scraper.APIFeatured(pos)
		if err != nil {
			slog.Warn("tenor API failed for featured GIFs", "error", err)
			if pos != "" {
				s.handleError(w, r, http.StatusBadGateway)
				return
			}
			// Fall back to scraping Tenor's homepage (first page only, no
			// pagination, no trending searches)
			results, scrapeErr := scraper.GetSearch("/")
			if scrapeErr != nil {
				slog.Error("HTML scraping fallback also failed", "error", scrapeErr)
				s.handleError(w, r, scrapeStatusCode(scrapeErr))
				return
			}
			w.Header().Set("Mezzo-Cache", "BYPASS")
			s.render(w, "explore.html", ExplorePage{BasePage: base, Results: results})
			return
		}

		if s.exploreCache != nil {
			s.exploreCache.Set(pos, featured)
		}
	}

	w.Header().Set("Mezzo-Cache", cacheStatus)

	// Trending searches only appear above the first page of featured GIFs
	var trends []scraper.Trend
	if pos == "" {
		trends = s.trending()
	}

	prevURL, nextURL := pageLinks(path, pos, prev, featured.Next)
	s.render(w, "explore.html", ExplorePage{
		BasePage: base,
		Trends:   trends,
		Results:  featured.Results,
		NextURL:  nextURL,
		PrevURL:  prevURL,
	})
}

// trending returns Tenor's trending searches. A failure is logged and yields
// nil so the featured GIFs still render.
func (s *Server) trending() []scraper.Trend {
	const key = "trending"

	if s.trendingCache != nil {
		if cached, ok := s.trendingCache.Get(key); ok {
			return cached
		}
	}

	trends, err := scraper.APITrending()
	if err != nil {
		slog.Warn("tenor API failed for trending searches", "error", err)
		return nil
	}

	if s.trendingCache != nil {
		s.trendingCache.Set(key, trends)
	}
	return trends
}

// pageLinks returns the Previous and Next links for a page of cursor-paginated
// results at path. pos is the cursor of the current page and prev the cursor of
// the page before it (both empty on the first page); next is the cursor of the
// following page, empty on the last page. An empty return value means no link.
func pageLinks(path, pos, prev, next string) (prevURL, nextURL string) {
	// Carry the current pos as prev so the next page can link back to it
	if next != "" {
		nextURL = path + "?pos=" + url.QueryEscape(next)
		if pos != "" {
			nextURL += "&prev=" + url.QueryEscape(pos)
		}
	}

	if pos != "" {
		if prev != "" {
			// Go back to the previous cursor position
			prevURL = path + "?pos=" + url.QueryEscape(prev)
		} else {
			// We're on page 2, previous is page 1 (no pos param)
			prevURL = path
		}
	}
	return prevURL, nextURL
}

// searchPath returns the path of the search page for a search term.
func searchPath(term string) string {
	return "/search/" + url.PathEscape(strings.ReplaceAll(term, " ", "-")+"-gifs")
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

func tenorViewPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Host == "" {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return ""
		}
	}

	host := parsed.Hostname()
	if host != "tenor.com" && !strings.HasSuffix(host, ".tenor.com") {
		return ""
	}

	slug, ok := strings.CutPrefix(parsed.EscapedPath(), "/view/")
	if !ok || slug == "" {
		return ""
	}
	return "/view/" + slug
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
	profilePath, section, ok := profileRoute(path)
	if !ok {
		s.handleError(w, r, http.StatusNotFound)
		return
	}
	pos := r.URL.Query().Get("pos")
	prev := r.URL.Query().Get("prev")

	cacheStatus := "MISS"

	var profilePage *scraper.ProfilePage
	var err error

	if s.profileCache != nil {
		if cached, ok := s.profileCache.Get(profilePath); ok {
			profilePage = cached
			cacheStatus = "HIT"
		}
	}

	if profilePage == nil {
		profilePage, err = scraper.GetProfile(profilePath)
		if err != nil {
			slog.Error("failed to scrape profile page", "error", err)
			s.handleError(w, r, scrapeStatusCode(err))
			return
		}

		if s.profileCache != nil {
			s.profileCache.Set(profilePath, profilePage)
		}
	}

	// The profile page lists GIFs; the sticker and meme pages list their own
	kind := section
	if kind == "" {
		kind = scraper.ProfileGIFs
	}
	uploads := profilePage.Uploads(kind)
	if pos != "" {
		more, hit, err := s.profileUploads(profilePage.Username, kind, pos)
		if err != nil {
			slog.Error("tenor API failed for profile uploads", "error", err)
			s.handleError(w, r, http.StatusBadGateway)
			return
		}
		uploads = *more
		if !hit {
			cacheStatus = "MISS"
		}
	}

	w.Header().Set("Mezzo-Cache", cacheStatus)

	displayName := profilePage.DisplayName
	if displayName == "" {
		displayName = profilePage.Username
	}

	heading, noun := profileKindLabels(section)
	title := displayName
	if section != "" {
		title += " " + heading
	}
	prevURL, nextURL := pageLinks(path, pos, prev, uploads.Next)

	data := ProfilePageData{
		BasePage: BasePage{
			Title:      title,
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
		CTA:         profilePage.CTA,
		Heading:     heading,
		Noun:        noun,
		Results:     uploads.Results,
		NextURL:     nextURL,
		PrevURL:     prevURL,
	}

	if section != "" {
		data.BackURL = profilePath
	} else if pos == "" {
		for _, rowKind := range []scraper.ProfileKind{scraper.ProfileMemes, scraper.ProfileStickers} {
			results := profilePage.Uploads(rowKind).Results
			if len(results) == 0 {
				continue
			}
			rowHeading, _ := profileKindLabels(rowKind)
			data.Rows = append(data.Rows, ProfileRow{
				Heading: rowHeading,
				SeeAll:  profilePath + "/" + string(rowKind),
				Results: results[:min(len(results), profileRowSize)],
			})
		}
	}

	s.render(w, "profile.html", data)
}

// profileUploads returns a page of a user's uploads after the first one,
// which comes with the profile page. hit reports whether it was cached.
func (s *Server) profileUploads(username string, kind scraper.ProfileKind, pos string) (resp *scraper.SearchResponse, hit bool, err error) {
	key := "@" + username + "\x00" + string(kind) + "\x00" + pos

	if s.searchCache != nil {
		if cached, ok := s.searchCache.Get(key); ok {
			return cached, true, nil
		}
	}

	resp, err = scraper.APIProfile(username, kind, pos)
	if err != nil {
		return nil, false, err
	}

	if s.searchCache != nil {
		s.searchCache.Set(key, resp)
	}
	return resp, false, nil
}

// profileRoute splits the path of a profile request into the path of the
// profile page and the kind of upload listed by a "See all" page, which is
// empty on the profile page itself. ok is false for anything else under
// /users/ and /official/.
func profileRoute(path string) (profilePath string, section scraper.ProfileKind, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Skip the locale prefix, so parts[i] is "users" or "official" and
	// parts[i+1] the username
	i := 0
	if len(parts[0]) == 5 && parts[0][2] == '-' {
		i = 1
	}

	switch len(parts) - i {
	case 2:
		return path, "", true
	case 3:
		switch kind := scraper.ProfileKind(parts[i+2]); kind {
		case scraper.ProfileStickers, scraper.ProfileMemes:
			return "/" + strings.Join(parts[:i+2], "/"), kind, true
		}
	}
	return "", "", false
}

// profileKindLabels returns the heading above a profile's uploads of the
// given kind and a phrase for them in a sentence. An empty kind is the
// profile page itself, which lists GIFs and, above them, the other kinds.
func profileKindLabels(kind scraper.ProfileKind) (heading, noun string) {
	switch kind {
	case scraper.ProfileStickers:
		return "Stickers", "stickers"
	case scraper.ProfileMemes:
		return "Memes", "memes"
	default:
		return "GIFs", "GIFs, stickers or memes"
	}
}

func (s *Server) handleError(w http.ResponseWriter, r *http.Request, code int) {
	w.WriteHeader(code)
	details := errorDetails(code)
	data := ErrorPage{
		BasePage: BasePage{
			Title:      details.heading,
			PatchesURL: s.patchesURL,
			Version:    s.version,
			Host:       "",
		},
		Code:     code,
		Heading:  details.heading,
		Message:  details.message,
		TenorURL: tenorURLForRequest(r),
	}
	s.render(w, "error.html", data)
}

func tenorURLForRequest(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}

	requestPath := r.URL.Path
	if requestPath == "" || requestPath == "/" ||
		strings.HasPrefix(requestPath, scraper.MediaPrefix) ||
		strings.HasPrefix(requestPath, scraper.AvatarPrefix) ||
		requestPath == "/static" ||
		strings.HasPrefix(requestPath, "/static/") {
		return ""
	}

	rawQuery := r.URL.RawQuery
	if isMezzoTenorRoute(requestPath) {
		rawQuery = ""
	}

	u := url.URL{
		Scheme:   "https",
		Host:     "tenor.com",
		Path:     requestPath,
		RawQuery: rawQuery,
	}
	return u.String()
}

func isMezzoTenorRoute(requestPath string) bool {
	parts := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return false
	}

	actionIndex := 0
	if len(parts[0]) == 5 && parts[0][2] == '-' {
		actionIndex = 1
	}
	if actionIndex >= len(parts) {
		return false
	}

	switch parts[actionIndex] {
	case "view", "search", "users", "official":
		return true
	default:
		return false
	}
}

type errorPageDetails struct {
	heading string
	message string
}

func errorDetails(code int) errorPageDetails {
	switch code {
	case http.StatusBadRequest:
		return errorPageDetails{
			heading: "Bad request",
			message: "Mezzo could not use that request as-is.",
		}
	case http.StatusNotFound:
		return errorPageDetails{
			heading: "Not found",
			message: "That page does not exist here, or Tenor no longer has it.",
		}
	case http.StatusBadGateway:
		return errorPageDetails{
			heading: "Tenor did not respond",
			message: "Mezzo reached Tenor, but Tenor returned an error or the media request failed.",
		}
	default:
		return errorPageDetails{
			heading: http.StatusText(code),
			message: "Mezzo hit an unexpected problem while handling this page.",
		}
	}
}

func scrapeStatusCode(err error) int {
	var httpErr *scraper.HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusNotFound {
			return http.StatusNotFound
		}
		return http.StatusBadGateway
	}

	return http.StatusInternalServerError
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
