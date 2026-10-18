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
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mezzo/internal/client"
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
			name: "proxy request",
			raw:  "http://mezzo.test/proxy.gif?url=https%3A%2F%2Fexample.com%2Fbad.gif",
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
		{Label: "SD GIF", Href: "/en-us/view/cat%20gif-1", Src: scraper.Proxy(formats[0].URL)},
		{Label: "HD GIF", Href: "/en-us/view/cat%20gif-1?format=hd", Src: scraper.Proxy(formats[1].URL), Active: true},
		{Label: "MP4", Href: "/en-us/view/cat%20gif-1?format=mp4", Src: scraper.Proxy(formats[2].URL), Video: true},
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
	want := []struct{ label, url, download string }{
		{"SD GIF", formats[0].URL, "cat gif-1-sd"},
		{"HD GIF", formats[1].URL, "cat gif-1-hd"},
		{"MP4", formats[2].URL, "cat gif-1"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d", len(got), len(want))
	}
	for i, w := range want {
		u, err := url.Parse(got[i].Href)
		if err != nil {
			t.Fatalf("option %d href %q: %v", i, got[i].Href, err)
		}
		if got[i].Label != w.label || u.Path != "/proxy.gif" ||
			u.Query().Get("url") != w.url || u.Query().Get("download") != w.download {
			t.Fatalf("option %d = %+v, want label %q url %q download %q", i, got[i], w.label, w.url, w.download)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHandleProxyContentDisposition(t *testing.T) {
	orig := client.Default.Transport
	client.Default.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/gif"}},
			Body:       io.NopCloser(strings.NewReader("GIF89a")),
			Request:    r,
		}, nil
	})
	t.Cleanup(func() { client.Default.Transport = orig })

	const media = "https://media1.tenor.com/m/x/AAAAC/cat.gif"
	tests := []struct {
		name         string
		query        string
		wantType     string
		wantFilename string
	}{
		{"displayed by default", "url=" + url.QueryEscape(media), "inline", "cat.gif"},
		{"download stem gets the media extension", "url=" + url.QueryEscape(media) + "&download=cat-hd", "attachment", "cat-hd.gif"},
		{"empty download keeps the media name", "url=" + url.QueryEscape(media) + "&download=", "attachment", "cat.gif"},
		{"download cannot choose the extension", "url=" + url.QueryEscape(media) + "&download=run.exe", "attachment", "run.exe.gif"},
		{"download cannot choose a directory", "url=" + url.QueryEscape(media) + "&download=" + url.QueryEscape("../../etc/cat"), "attachment", "cat.gif"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/proxy.gif?"+tt.query, nil)
			(&Server{}).handleProxy(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			gotType, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
			if err != nil {
				t.Fatalf("Content-Disposition %q: %v", rec.Header().Get("Content-Disposition"), err)
			}
			if gotType != tt.wantType || params["filename"] != tt.wantFilename {
				t.Fatalf("Content-Disposition = %s %q, want %s %q", gotType, params["filename"], tt.wantType, tt.wantFilename)
			}
		})
	}

	t.Run("download name cannot inject headers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		stem := "a\"\r\nX-Evil: 1"
		req := httptest.NewRequest(http.MethodGet, "/proxy.gif?url="+url.QueryEscape(media)+"&download="+url.QueryEscape(stem), nil)
		(&Server{}).handleProxy(rec, req)

		if v := rec.Header().Get("X-Evil"); v != "" {
			t.Fatalf("injected header X-Evil = %q", v)
		}
		_, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
		if err != nil {
			t.Fatalf("Content-Disposition %q: %v", rec.Header().Get("Content-Disposition"), err)
		}
		if want := stem + ".gif"; params["filename"] != want {
			t.Fatalf("filename = %q, want %q", params["filename"], want)
		}
	})
}
