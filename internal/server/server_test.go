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
	"net/http"
	"net/http/httptest"
	"testing"

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
