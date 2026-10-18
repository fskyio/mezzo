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

func getExplore(t *testing.T, target string) (int, string) {
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
		code, body := getExplore(t, "/explore")
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
		code, body := getExplore(t, "/explore?pos=c2")
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
		code, body := getExplore(t, "/en-us/explore")
		if code != http.StatusOK || !strings.Contains(body, `href="/en-us/explore?pos=c2"`) {
			t.Fatalf("status = %d, want 200 with locale-prefixed pagination", code)
		}
	})
}

func TestExploreWithoutTrending(t *testing.T) {
	stubTenor(t, func(u *url.URL) (int, string) {
		if u.Path == "/v2/featured" {
			return http.StatusOK, featuredPage1
		}
		return http.StatusInternalServerError, ""
	})

	code, body := getExplore(t, "/explore")
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

	code, body := getExplore(t, "/explore")
	if code != http.StatusOK || !strings.Contains(body, `href="/view/home-gif-3"`) {
		t.Fatalf("status = %d, want homepage GIFs when the API fails", code)
	}

	// The fallback has no cursor to continue from
	if code, _ := getExplore(t, "/explore?pos=c2"); code != http.StatusBadGateway {
		t.Fatalf("later page status = %d, want 502 when the API fails", code)
	}
}
