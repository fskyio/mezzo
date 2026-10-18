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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"mezzo/internal/assets"
	"mezzo/internal/client"
	"mezzo/internal/config"
	"mezzo/internal/scraper"
)

func TestQueryFromSearchPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "basic search slug",
			path: "/search/funny-cats-gifs",
			want: "funny cats",
		},
		{
			name: "trailing slash",
			path: "/search/funny-cats-gifs/",
			want: "funny cats",
		},
		{
			name: "locale prefixed search slug",
			path: "/en-us/search/funny-cats-gifs",
			want: "funny cats",
		},
		{
			name: "search root",
			path: "/search",
			want: "",
		},
		{
			name: "empty last segment",
			path: "/search/",
			want: "",
		},
		{
			name: "slug without gifs suffix",
			path: "/search/funny-cats",
			want: "funny cats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryFromSearchPath(tt.path); got != tt.want {
				t.Fatalf("queryFromSearchPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestTenorViewPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "full tenor URL",
			raw:  "https://tenor.com/view/cat-wave-12345",
			want: "/view/cat-wave-12345",
		},
		{
			name: "tenor URL without scheme",
			raw:  "tenor.com/view/cat-wave-12345",
			want: "/view/cat-wave-12345",
		},
		{
			name: "localized tenor subdomain",
			raw:  "https://www.tenor.com/view/cat-wave-12345?utm_source=share",
			want: "/view/cat-wave-12345",
		},
		{
			name: "escaped path is preserved",
			raw:  "https://tenor.com/view/cat%20wave-12345",
			want: "/view/cat%20wave-12345",
		},
		{
			name: "non tenor host",
			raw:  "https://example.com/view/cat-wave-12345",
			want: "",
		},
		{
			name: "tenor host without view path",
			raw:  "https://tenor.com/search/cat-gifs",
			want: "",
		},
		{
			name: "spoofed suffix",
			raw:  "https://nottenor.com/view/cat-wave-12345",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenorViewPath(tt.raw); got != tt.want {
				t.Fatalf("tenorViewPath(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestTenorURLForRequest(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "view page",
			raw:  "http://mezzo.test/view/cat-wave-12345",
			want: "https://tenor.com/view/cat-wave-12345",
		},
		{
			name: "search page omits local pagination query",
			raw:  "http://mezzo.test/search/funny-cats-gifs?pos=abc",
			want: "https://tenor.com/search/funny-cats-gifs",
		},
		{
			name: "profile page",
			raw:  "http://mezzo.test/users/example",
			want: "https://tenor.com/users/example",
		},
		{
			name: "locale prefixed page",
			raw:  "http://mezzo.test/en-us/view/cat-wave-12345",
			want: "https://tenor.com/en-us/view/cat-wave-12345",
		},
		{
			name: "escaped path",
			raw:  "http://mezzo.test/view/cat%20wave-12345",
			want: "https://tenor.com/view/cat%20wave-12345",
		},
		{
			name: "unknown page",
			raw:  "http://mezzo.test/about",
			want: "https://tenor.com/about",
		},
		{
			name: "unknown page preserves query",
			raw:  "http://mezzo.test/legal/privacy?hl=en",
			want: "https://tenor.com/legal/privacy?hl=en",
		},
		{
			name: "missing route slug",
			raw:  "http://mezzo.test/view/",
			want: "https://tenor.com/view/",
		},
		{
			name: "media request",
			raw:  "http://mezzo.test/media/media1.tenor.com/m/AAAAC/missing.gif",
			want: "",
		},
		{
			name: "avatar request",
			raw:  "http://mezzo.test/avatar/media.tenor.com/missing.png",
			want: "",
		},
		{
			name: "static request",
			raw:  "http://mezzo.test/static/missing.png",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.raw, nil)
			if got := tenorURLForRequest(req); got != tt.want {
				t.Fatalf("tenorURLForRequest(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestErrorDetails(t *testing.T) {
	details := errorDetails(http.StatusNotFound)
	if details.heading != "Not found" {
		t.Fatalf("404 heading = %q, want Not found", details.heading)
	}
	if details.message == "" {
		t.Fatal("404 message is empty")
	}

	details = errorDetails(http.StatusTeapot)
	if details.heading != http.StatusText(http.StatusTeapot) {
		t.Fatalf("teapot heading = %q, want %q", details.heading, http.StatusText(http.StatusTeapot))
	}
}

func TestScrapeStatusCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "tenor not found stays not found",
			err:  &scraper.HTTPError{StatusCode: http.StatusNotFound, Status: "404 Not Found"},
			want: http.StatusNotFound,
		},
		{
			name: "other tenor errors become bad gateway",
			err:  &scraper.HTTPError{StatusCode: http.StatusInternalServerError, Status: "500 Internal Server Error"},
			want: http.StatusBadGateway,
		},
		{
			name: "other errors become internal server error",
			err:  errors.New("parse failed"),
			want: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scrapeStatusCode(tt.err); got != tt.want {
				t.Fatalf("scrapeStatusCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestFormatOptions(t *testing.T) {
	formats := []scraper.GifFormat{
		{Key: "sd", Label: "SD GIF", URL: "https://media1.tenor.com/sd.gif"},
		{Key: "hd", Label: "HD GIF", URL: "https://media1.tenor.com/hd.gif"},
		{Key: "mp4", Label: "MP4", URL: "https://media.tenor.com/cat.mp4", Video: true},
	}

	if got := formatOptions("/view/cat-gif-1", formats[:1], "sd"); got != nil {
		t.Fatalf("single format produced a switcher: %+v", got)
	}

	got := formatOptions("/en-us/view/cat%20gif-1", formats, "hd")
	want := []FormatOption{
		{Label: "SD GIF", Href: "/en-us/view/cat%20gif-1", Src: "/media/media1.tenor.com/sd.gif"},
		{Label: "HD GIF", Href: "/en-us/view/cat%20gif-1?format=hd", Src: "/media/media1.tenor.com/hd.gif", Active: true},
		{Label: "MP4", Href: "/en-us/view/cat%20gif-1?format=mp4", Src: "/media/media.tenor.com/cat.mp4", Video: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("option %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDownloadOptions(t *testing.T) {
	formats := []scraper.GifFormat{
		{Key: "sd", Label: "SD GIF", URL: "https://media1.tenor.com/m/x/AAAAd/cat.gif"},
		{Key: "hd", Label: "HD GIF", URL: "https://media1.tenor.com/m/x/AAAAC/cat.gif"},
		{Key: "mp4", Label: "MP4", URL: "https://media.tenor.com/x/AAAPo/cat.mp4", Video: true},
	}

	got := downloadOptions("/en-us/view/cat%20gif-1", formats)
	want := []struct{ label, path, download string }{
		{"SD GIF", "/media/media1.tenor.com/m/x/AAAAd/cat.gif", "cat gif-1-sd"},
		{"HD GIF", "/media/media1.tenor.com/m/x/AAAAC/cat.gif", "cat gif-1-hd"},
		{"MP4", "/media/media.tenor.com/x/AAAPo/cat.mp4", "cat gif-1"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d", len(got), len(want))
	}
	for i, w := range want {
		u, err := url.Parse(got[i].Href)
		if err != nil {
			t.Fatalf("option %d href %q: %v", i, got[i].Href, err)
		}
		if got[i].Label != w.label || u.Path != w.path || u.Query().Get("download") != w.download {
			t.Fatalf("option %d = %+v, want label %q path %q download %q", i, got[i], w.label, w.path, w.download)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPageLinks(t *testing.T) {
	tests := []struct {
		name               string
		pos, prev, next    string
		wantPrev, wantNext string
	}{
		{name: "first page", next: "b", wantNext: "/p?pos=b"},
		{name: "second page", pos: "b", next: "c", wantPrev: "/p", wantNext: "/p?pos=c&prev=b"},
		{name: "third page", pos: "c", prev: "b", next: "d", wantPrev: "/p?pos=b", wantNext: "/p?pos=d&prev=c"},
		{name: "last page", pos: "c", prev: "b", wantPrev: "/p?pos=b"},
		{name: "only page"},
		{name: "cursor is escaped", pos: "1.5", next: "a&b=c", wantPrev: "/p", wantNext: "/p?pos=a%26b%3Dc&prev=1.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrev, gotNext := pageLinks("/p", tt.pos, tt.prev, tt.next)
			if gotPrev != tt.wantPrev || gotNext != tt.wantNext {
				t.Fatalf("pageLinks = (%q, %q), want (%q, %q)", gotPrev, gotNext, tt.wantPrev, tt.wantNext)
			}
		})
	}
}

func TestRelatedLinks(t *testing.T) {
	tests := []struct {
		name               string
		format             string
		pos, prev, next    string
		wantPrev, wantNext string
	}{
		{name: "first page", next: "b", wantNext: "/v?pos=b#related"},
		// The first page needs an empty pos to open the related GIFs
		{name: "second page", pos: "b", next: "c", wantPrev: "/v?pos=#related", wantNext: "/v?pos=c&prev=b#related"},
		{name: "third page", pos: "c", prev: "b", next: "d", wantPrev: "/v?pos=b#related", wantNext: "/v?pos=d&prev=c#related"},
		{name: "last page", pos: "c", prev: "b", wantPrev: "/v?pos=b#related"},
		{name: "only page"},
		{name: "keeps the format", format: "mp4", pos: "b", next: "c", wantPrev: "/v?format=mp4&pos=#related", wantNext: "/v?format=mp4&pos=c&prev=b#related"},
		{name: "cursor is escaped", pos: "1.5", next: "a&b=c", wantPrev: "/v?pos=#related", wantNext: "/v?pos=a%26b%3Dc&prev=1.5#related"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrev, gotNext := relatedLinks("/v", tt.format, tt.pos, tt.prev, tt.next)
			if gotPrev != tt.wantPrev || gotNext != tt.wantNext {
				t.Fatalf("relatedLinks = (%q, %q), want (%q, %q)", gotPrev, gotNext, tt.wantPrev, tt.wantNext)
			}
		})
	}
}

func TestSearchPathRoundTrips(t *testing.T) {
	for _, term := range []string{"good night", "what? #1", "100% real", "café"} {
		p := searchPath(term)
		got := queryFromSearchPath(httptest.NewRequest(http.MethodGet, p, nil).URL.Path)
		if got != term {
			t.Fatalf("searchPath(%q) = %q, which searches for %q", term, p, got)
		}
	}
}

// stubTenor replaces the HTTP transport so Tenor requests are answered by
// handler, which receives the request URL and returns a status and body.
func stubTenor(t *testing.T, handler func(u *url.URL) (int, string)) {
	t.Helper()
	orig := client.Default.Transport
	client.Default.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status, body := handler(r.URL)
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})
	t.Cleanup(func() { client.Default.Transport = orig })
}

func getPage(t *testing.T, target string) (int, string) {
	t.Helper()
	s, err := New(assets.Assets, "test", &config.Config{CacheDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Code, rec.Body.String()
}

const (
	featuredPage1 = `{"results":[{"itemurl":"https://tenor.com/view/cat-gif-1","content_description":"a cat",` +
		`"media_formats":{"tinygif":{"url":"https://media.tenor.com/a/cat.gif","dims":[220,124]}}}],"next":"c2"}`
	featuredPage2 = `{"results":[{"itemurl":"https://tenor.com/view/dog-gif-2","content_description":"a dog",` +
		`"media_formats":{"tinygif":{"url":"https://media.tenor.com/b/dog.gif","dims":[220,124]}}}],"next":""}`
	trendingTags = `{"tags":[{"searchterm":"good night","image":"https://media.tenor.com/n/night.gif"}]}`
)

func TestExplorePagination(t *testing.T) {
	var categoryCalls int
	stubTenor(t, func(u *url.URL) (int, string) {
		switch u.Path {
		case "/v2/featured":
			if u.Query().Get("pos") == "c2" {
				return http.StatusOK, featuredPage2
			}
			return http.StatusOK, featuredPage1
		case "/v2/categories":
			categoryCalls++
			return http.StatusOK, trendingTags
		}
		return http.StatusNotFound, ""
	})

	t.Run("first page", func(t *testing.T) {
		code, body := getPage(t, "/explore")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		for _, want := range []string{
			`href="/search/good-night-gifs"`, // trending search
			`href="/view/cat-gif-1"`,         // featured GIF
			`href="/explore?pos=c2"`,         // next page
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("first page is missing %s", want)
			}
		}
		if strings.Contains(body, "Previous page") {
			t.Fatal("first page has a Previous link")
		}
	})

	t.Run("later page", func(t *testing.T) {
		before := categoryCalls
		code, body := getPage(t, "/explore?pos=c2")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if !strings.Contains(body, `href="/view/dog-gif-2"`) {
			t.Fatal("later page is missing its featured GIF")
		}
		if !strings.Contains(body, `href="/explore"`) || !strings.Contains(body, "Previous page") {
			t.Fatal("later page does not link back to the first page")
		}
		if strings.Contains(body, "Next page") {
			t.Fatal("last page has a Next link")
		}
		if strings.Contains(body, "Trending searches") || categoryCalls != before {
			t.Fatal("later page shows or fetches trending searches")
		}
	})

	t.Run("locale prefix", func(t *testing.T) {
		code, body := getPage(t, "/en-us/explore")
		if code != http.StatusOK || !strings.Contains(body, `href="/en-us/explore?pos=c2"`) {
			t.Fatalf("status = %d, want 200 with locale-prefixed pagination", code)
		}
	})
}

func TestTileHoverControls(t *testing.T) {
	// The first GIF has everything, the second only a single format and the
	// third neither tags nor a download, like a result scraped from HTML.
	stubTenor(t, func(u *url.URL) (int, string) {
		if u.Path != "/v2/featured" {
			return http.StatusNotFound, ""
		}
		return http.StatusOK, `{"results":[` +
			`{"itemurl":"https://tenor.com/view/full-gif-1","tags":["one","two","three","four","five"],"media_formats":{` +
			`"tinygif":{"url":"https://media.tenor.com/t/full.gif"},` +
			`"mediumgif":{"url":"https://media.tenor.com/m/full.gif"},` +
			`"gif":{"url":"https://media.tenor.com/h/full.gif"},` +
			`"mp4":{"url":"https://media.tenor.com/v/full.mp4"}}},` +
			`{"itemurl":"https://tenor.com/view/single-gif-2","media_formats":{` +
			`"tinygif":{"url":"https://media.tenor.com/t/single.gif"},` +
			`"mediumgif":{"url":"https://media.tenor.com/m/single.gif"}}},` +
			`{"itemurl":"https://tenor.com/view/bare-gif-3","media_formats":{` +
			`"tinygif":{"url":"https://media.tenor.com/t/bare.gif"}}}],"next":""}`
	})

	code, body := getPage(t, "/explore")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	tiles := strings.Split(body, `<div class="tile">`)[1:]
	if len(tiles) != 3 {
		t.Fatalf("got %d tiles, want 3", len(tiles))
	}
	full, single, bare := tiles[0], tiles[1], tiles[2]

	for _, want := range []string{`>one</a>`, `>two</a>`, `>three</a>`} {
		if !strings.Contains(full, want) {
			t.Errorf("full tile is missing the tag %s", want)
		}
	}
	if strings.Contains(full, `>four</a>`) {
		t.Error("full tile shows a fourth tag")
	}
	for _, want := range []string{`<details class="tile-menu">`, `>SD GIF</a>`, `>HD GIF</a>`, `>MP4</a>`, `?download=full-gif-1-sd"`} {
		if !strings.Contains(full, want) {
			t.Errorf("full tile is missing %s", want)
		}
	}

	if strings.Contains(single, "tile-tags") || strings.Contains(single, "<details") {
		t.Error("single-format tile without tags has a tag list or a menu")
	}
	if !strings.Contains(single, `<a class="tile-download" href="/media/media.tenor.com/m/single.gif?download=single-gif-2-sd"`) {
		t.Errorf("single-format tile has no direct download link:\n%s", single)
	}

	if strings.Contains(bare, "tile-hover") {
		t.Error("tile without tags or formats has hover controls")
	}
}

func TestExploreWithoutTrending(t *testing.T) {
	stubTenor(t, func(u *url.URL) (int, string) {
		if u.Path == "/v2/featured" {
			return http.StatusOK, featuredPage1
		}
		return http.StatusInternalServerError, ""
	})

	code, body := getPage(t, "/explore")
	if code != http.StatusOK || !strings.Contains(body, `href="/view/cat-gif-1"`) {
		t.Fatalf("status = %d, want featured GIFs rendered without trending searches", code)
	}
	if strings.Contains(body, "Trending searches") {
		t.Fatal("heading rendered with no trending searches")
	}
}

func TestExploreFallsBackToHomepage(t *testing.T) {
	const home = `<figure><a href="/view/home-gif-3"><img src="https://media.tenor.com/h/home.gif" width="180" height="120" alt="home"></a></figure>`
	stubTenor(t, func(u *url.URL) (int, string) {
		if u.Host == "tenor.com" && u.Path == "/" {
			return http.StatusOK, home
		}
		return http.StatusForbidden, ""
	})

	code, body := getPage(t, "/explore")
	if code != http.StatusOK || !strings.Contains(body, `href="/view/home-gif-3"`) {
		t.Fatalf("status = %d, want homepage GIFs when the API fails", code)
	}

	// The fallback has no cursor to continue from
	if code, _ := getPage(t, "/explore?pos=c2"); code != http.StatusBadGateway {
		t.Fatalf("later page status = %d, want 502 when the API fails", code)
	}
}

func TestProfileRoute(t *testing.T) {
	tests := []struct {
		path        string
		wantProfile string
		wantSection scraper.ProfileKind
		wantOK      bool
	}{
		{"/users/mezzo", "/users/mezzo", "", true},
		{"/official/mezzo", "/official/mezzo", "", true},
		{"/users/mezzo/stickers", "/users/mezzo", scraper.ProfileStickers, true},
		{"/official/mezzo/memes", "/official/mezzo", scraper.ProfileMemes, true},
		{"/en-us/users/mezzo", "/en-us/users/mezzo", "", true},
		{"/en-us/official/mezzo/stickers", "/en-us/official/mezzo", scraper.ProfileStickers, true},
		// Tenor has no page for GIFs alone, and nothing deeper
		{"/users/mezzo/gifs", "", "", false},
		{"/users/mezzo/other", "", "", false},
		{"/users/mezzo/stickers/more", "", "", false},
		{"/en-us/users", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			profile, section, ok := profileRoute(tt.path)
			if profile != tt.wantProfile || section != tt.wantSection || ok != tt.wantOK {
				t.Fatalf("profileRoute(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.path, profile, section, ok, tt.wantProfile, tt.wantSection, tt.wantOK)
			}
		})
	}
}

// profileUpload returns one result of a store-cache upload set.
func profileUpload(slug, image string) string {
	return `{"itemurl":"https://tenor.com/view/` + slug + `","content_description":"` + slug + `",` +
		`"media_formats":{"tinygif":{"url":"https://media.tenor.com/` + image + `","dims":[220,124]}}}`
}

// profileHTML returns a Tenor profile page whose store-cache holds the given
// result lists and cursors for the user "Mezzo".
func profileHTML(gifs, stickers, memes []string, gifsNext, stickersNext string) string {
	set := func(results []string, next string) string {
		return `{"next":"` + next + `","results":[` + strings.Join(results, ",") + `]}`
	}
	return `<html><body><script id="store-cache">{` +
		`"profiles":{"p":{"user":{"username":"Mezzo","partnername":"Mezzo","usertype":"user"}}},` +
		`"gifs":{"searchByUsername":{"mezzo-gifs-profile-public":` + set(gifs, gifsNext) + `}},` +
		`"stickers":{"searchByUsername":{"mezzo-stickers-profile-public":` + set(stickers, stickersNext) + `}},` +
		`"memes":{"searchByUsername":{"mezzo-memes-profile-public":` + set(memes, "") + `}}` +
		`}</script></body></html>`
}

func TestProfilePages(t *testing.T) {
	var scraped []string // Tenor pages fetched
	var apiCalls []url.Values
	stubTenor(t, func(u *url.URL) (int, string) {
		switch {
		case u.Host == "tenor.com":
			scraped = append(scraped, u.Path)
			return http.StatusOK, profileHTML(
				[]string{profileUpload("gif-1", "g1.gif")},
				[]string{profileUpload("sticker-2", "s2.gif"), profileUpload("sticker-3", "s3.gif")},
				[]string{profileUpload("meme-4", "m4.gif")},
				"gif-next", "sticker-next")
		case u.Path == "/v2/search":
			q := u.Query()
			apiCalls = append(apiCalls, q)
			switch q.Get("pos") {
			case "gif-next":
				return http.StatusOK, `{"results":[` + profileUpload("gif-5", "g5.gif") + `],"next":""}`
			case "sticker-next":
				return http.StatusOK, `{"results":[` + profileUpload("sticker-6", "s6.gif") + `],"next":""}`
			}
		}
		return http.StatusNotFound, ""
	})

	// reset forgets the requests of earlier subtests
	reset := func() { scraped, apiCalls = nil, nil }

	t.Run("profile page", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/users/mezzo")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		for _, want := range []string{
			`<h2>Memes</h2>`, `<a href="/users/mezzo/memes">See all</a>`, `href="/view/meme-4"`,
			`<h2>Stickers</h2>`, `<a href="/users/mezzo/stickers">See all</a>`, `href="/view/sticker-2"`, `href="/view/sticker-3"`,
			`<h2>GIFs</h2>`, `href="/view/gif-1"`,
			`href="/users/mezzo?pos=gif-next"`, // next page of GIFs, not of stickers
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("profile page is missing %s", want)
			}
		}
		if strings.Contains(body, "Previous page") || strings.Contains(body, "sticker-next") {
			t.Fatal("first page has a Previous link or links to the sticker cursor")
		}
		if len(apiCalls) != 0 {
			t.Fatalf("first page called the API: %v", apiCalls)
		}
	})

	t.Run("later GIF page", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/users/mezzo?pos=gif-next")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if !strings.Contains(body, `href="/view/gif-5"`) || !strings.Contains(body, `href="/users/mezzo"`) || !strings.Contains(body, "Previous page") {
			t.Fatal("later page is missing its GIF or the link back to the first page")
		}
		if strings.Contains(body, "Next page") {
			t.Fatal("last page has a Next link")
		}
		if strings.Contains(body, "See all") || strings.Contains(body, `href="/view/sticker-2"`) || strings.Contains(body, `href="/view/gif-1"`) {
			t.Fatal("later page repeats the first page's rows and GIFs")
		}
		if len(apiCalls) != 1 || apiCalls[0].Get("q") != "@Mezzo" || apiCalls[0].Has("searchfilter") {
			t.Fatalf("API calls = %v, want one unfiltered search for @Mezzo", apiCalls)
		}
	})

	t.Run("sticker page", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/users/mezzo/stickers")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		for _, want := range []string{
			`<h2>Stickers</h2>`, `href="/view/sticker-2"`, `href="/view/sticker-3"`,
			`<a href="/users/mezzo">Back to profile</a>`,
			`href="/users/mezzo/stickers?pos=sticker-next"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("sticker page is missing %s", want)
			}
		}
		if strings.Contains(body, `href="/view/gif-1"`) || strings.Contains(body, `href="/view/meme-4"`) {
			t.Fatal("sticker page lists GIFs or memes")
		}
		if !slices.Equal(scraped, []string{"/users/mezzo"}) {
			t.Fatalf("fetched %v from Tenor, want only the profile page", scraped)
		}
	})

	t.Run("later sticker page", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/users/mezzo/stickers?pos=sticker-next")
		if code != http.StatusOK || !strings.Contains(body, `href="/view/sticker-6"`) {
			t.Fatalf("status = %d, want 200 with the second page of stickers", code)
		}
		if !strings.Contains(body, `href="/users/mezzo/stickers"`) {
			t.Fatal("later sticker page does not link back to the first one")
		}
		if len(apiCalls) != 1 || apiCalls[0].Get("searchfilter") != "sticker" {
			t.Fatalf("API calls = %v, want one search filtered to stickers", apiCalls)
		}
	})

	t.Run("locale prefix", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/en-us/official/mezzo/memes")
		if code != http.StatusOK || !strings.Contains(body, `href="/view/meme-4"`) {
			t.Fatalf("status = %d, want 200 with the memes", code)
		}
		if !strings.Contains(body, `<a href="/en-us/official/mezzo">Back to profile</a>`) {
			t.Fatal("meme page does not link back to the locale-prefixed profile")
		}
		if !slices.Equal(scraped, []string{"/en-us/official/mezzo"}) {
			t.Fatalf("fetched %v from Tenor, want the locale-prefixed profile page", scraped)
		}

		_, body = getPage(t, "/en-us/users/mezzo")
		if !strings.Contains(body, `<a href="/en-us/users/mezzo/stickers">See all</a>`) {
			t.Fatal("profile page does not link to the locale-prefixed sticker page")
		}
	})

	t.Run("unknown section", func(t *testing.T) {
		reset()
		for _, target := range []string{"/users/mezzo/gifs", "/users/mezzo/other", "/en-us/official/mezzo/other"} {
			if code, _ := getPage(t, target); code != http.StatusNotFound {
				t.Fatalf("GET %s status = %d, want 404", target, code)
			}
		}
		if len(scraped) != 0 {
			t.Fatalf("fetched %v from Tenor for unknown sections", scraped)
		}
	})
}

func TestProfileWithOnlyStickers(t *testing.T) {
	stubTenor(t, func(u *url.URL) (int, string) {
		return http.StatusOK, profileHTML(nil, []string{profileUpload("sticker-2", "s2.gif")}, nil, "", "")
	})

	code, body := getPage(t, "/users/mezzo")
	if code != http.StatusOK || !strings.Contains(body, `<h2>Stickers</h2>`) {
		t.Fatalf("status = %d, want 200 with a Stickers row", code)
	}
	// Tenor hides sections with nothing in them
	for _, unwanted := range []string{`<h2>GIFs</h2>`, `<h2>Memes</h2>`, "hasn't uploaded"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("profile page shows %q", unwanted)
		}
	}
}

func TestProfileWithNoUploads(t *testing.T) {
	stubTenor(t, func(u *url.URL) (int, string) {
		return http.StatusOK, profileHTML(nil, nil, nil, "", "")
	})

	_, body := getPage(t, "/users/mezzo")
	if !strings.Contains(body, "hasn't uploaded any GIFs, stickers or memes.") {
		t.Fatal("empty profile has no explanation")
	}

	_, body = getPage(t, "/users/mezzo/memes")
	if !strings.Contains(body, "hasn't uploaded any memes.") || !strings.Contains(body, `href="/users/mezzo"`) {
		t.Fatal("empty meme page has no explanation or link back to the profile")
	}
}

func TestProfileLaterPageNeedsTheAPI(t *testing.T) {
	stubTenor(t, func(u *url.URL) (int, string) {
		if u.Host == "tenor.com" {
			return http.StatusOK, profileHTML([]string{profileUpload("gif-1", "g1.gif")}, nil, nil, "gif-next", "")
		}
		return http.StatusForbidden, ""
	})

	if code, _ := getPage(t, "/users/mezzo"); code != http.StatusOK {
		t.Fatalf("first page status = %d, want 200 without the API", code)
	}
	if code, _ := getPage(t, "/users/mezzo?pos=gif-next"); code != http.StatusBadGateway {
		t.Fatalf("later page status = %d, want 502 when the API fails", code)
	}
}

// viewHTML returns a Tenor view page for the GIF with ID 1 whose store-cache
// holds the given related GIFs and cursor.
func viewHTML(related []string, next string) string {
	return `<html><body><h1>Cat</h1><img src="https://media.tenor.com/c/cat.gif">` +
		`<script id="store-cache">{"gifs":{` +
		`"byId":{"1":{"results":[{"id":"1","media_formats":{"mediumgif":{"url":"https://media.tenor.com/c/cat.gif"}}}]}},` +
		`"related":{"1":{"next":"` + next + `","results":[` + strings.Join(related, ",") + `]}}` +
		`}}</script></body></html>`
}

func TestViewRelatedGIFs(t *testing.T) {
	var apiCalls []url.Values
	apiFails := false
	stubTenor(t, func(u *url.URL) (int, string) {
		switch {
		case u.Host == "tenor.com" && u.Path == "/view/lonely-gif-1":
			return http.StatusOK, viewHTML(nil, "")
		case u.Host == "tenor.com":
			return http.StatusOK, viewHTML([]string{profileUpload("dog-gif-2", "d2.gif")}, "r2")
		case u.Path == "/v2/related":
			apiCalls = append(apiCalls, u.Query())
			if apiFails {
				return http.StatusInternalServerError, ""
			}
			return http.StatusOK, `{"results":[` + profileUpload("bird-gif-3", "b3.gif") + `],"next":""}`
		}
		return http.StatusNotFound, ""
	})

	// reset forgets the requests of earlier subtests
	reset := func() { apiCalls = nil }

	t.Run("first page starts collapsed", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/view/cat-gif-1")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		for _, want := range []string{
			`<details class="related" id="related">`,
			`href="/view/dog-gif-2"`,
			`href="/view/cat-gif-1?pos=r2#related"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("view page is missing %s", want)
			}
		}
		if strings.Contains(body, "Previous page") {
			t.Fatal("first page has a Previous link")
		}
		if len(apiCalls) != 0 {
			t.Fatalf("first page called the API: %v", apiCalls)
		}
	})

	t.Run("later page is expanded", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/view/cat-gif-1?pos=r2")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		for _, want := range []string{
			`<details class="related" id="related" open>`,
			`href="/view/bird-gif-3"`,
			`href="/view/cat-gif-1?pos=#related"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("later page is missing %s", want)
			}
		}
		if strings.Contains(body, `href="/view/dog-gif-2"`) || strings.Contains(body, "Next page") {
			t.Fatal("later page repeats the first page or has a Next link on the last page")
		}
		if len(apiCalls) != 1 || apiCalls[0].Get("id") != "1" || apiCalls[0].Get("pos") != "r2" {
			t.Fatalf("API calls = %v, want one for the GIF's related GIFs at r2", apiCalls)
		}
	})

	t.Run("link back to the first page is expanded", func(t *testing.T) {
		reset()
		_, body := getPage(t, "/view/cat-gif-1?pos=")
		if !strings.Contains(body, `<details class="related" id="related" open>`) || !strings.Contains(body, `href="/view/dog-gif-2"`) {
			t.Fatal("first page is not expanded or misses its related GIFs")
		}
		if len(apiCalls) != 0 {
			t.Fatalf("first page called the API: %v", apiCalls)
		}
	})

	t.Run("no related GIFs", func(t *testing.T) {
		reset()
		code, body := getPage(t, "/view/lonely-gif-1")
		if code != http.StatusOK || strings.Contains(body, "Related GIFs") {
			t.Fatalf("status = %d, want 200 without a related section", code)
		}
	})

	t.Run("later page needs the API", func(t *testing.T) {
		reset()
		apiFails = true
		t.Cleanup(func() { apiFails = false })
		if code, _ := getPage(t, "/view/cat-gif-1?pos=r2"); code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502 when the API fails", code)
		}
	})
}
