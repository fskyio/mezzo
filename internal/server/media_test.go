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
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mezzo/internal/assets"
	"mezzo/internal/client"
	"mezzo/internal/config"
)

const (
	gifPath     = "/media/media1.tenor.com/m/x/AAAAC/cat.gif"
	gifUpstream = "https://media1.tenor.com/m/x/AAAAC/cat.gif"
	gifBody     = "GIF89a-0123456789"
)

// tenorStub records the requests that reach Tenor.
type tenorStub struct {
	mu    sync.Mutex
	calls []stubCall
}

type stubCall struct{ url, rng string }

func (s *tenorStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *tenorStub) call(i int) stubCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[i]
}

// stubMedia answers requests to Tenor with respond and records them.
func stubMedia(t *testing.T, respond func(r *http.Request) *http.Response) *tenorStub {
	t.Helper()
	stub := &tenorStub{}
	orig := client.Default.Transport
	client.Default.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		stub.mu.Lock()
		stub.calls = append(stub.calls, stubCall{url: r.URL.String(), rng: r.Header.Get("Range")})
		stub.mu.Unlock()
		resp := respond(r)
		resp.Request = r
		return resp, nil
	})
	t.Cleanup(func() { client.Default.Transport = orig })
	return stub
}

func mediaResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type":   {contentType},
			"Content-Length": {strconv.Itoa(len(body))},
		},
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(strings.NewReader(body)),
	}
}

func gifResponse(*http.Request) *http.Response {
	return mediaResponse(http.StatusOK, "image/gif", gifBody)
}

// cachingConfig is a config with the media cache on in a temporary directory.
func cachingConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		CacheGifTTL:         time.Hour,
		CacheGifMax:         1,
		CacheSearchTTL:      time.Hour,
		CacheSearchMax:      1,
		CacheProfileTTL:     time.Hour,
		CacheProfileMax:     1,
		CacheMediaDir:       t.TempDir(),
		CacheMediaMaxMB:     8,
		CacheMediaMaxFileMB: 1,
		CacheMediaTTL:       time.Hour,
		CacheAvatarTTL:      6 * time.Hour,
	}
}

func newMediaServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	s, err := New(assets.Assets, "test", &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.mediaStore != nil {
		t.Cleanup(s.mediaStore.Close)
	}
	return s
}

// get requests target from s; header holds alternating names and values.
func get(s *Server, target string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestMediaContentDisposition(t *testing.T) {
	stubMedia(t, gifResponse)
	s := newMediaServer(t, config.Config{CacheDisabled: true})

	tests := []struct {
		name         string
		query        string
		wantType     string
		wantFilename string
	}{
		{"displayed by default", "", "inline", "cat.gif"},
		{"download stem gets the media extension", "?download=cat-hd", "attachment", "cat-hd.gif"},
		{"empty download keeps the media name", "?download=", "attachment", "cat.gif"},
		{"download cannot choose the extension", "?download=run.exe", "attachment", "run.exe.gif"},
		{"download cannot choose a directory", "?download=" + url.QueryEscape("../../etc/cat"), "attachment", "cat.gif"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(s, gifPath+tt.query)

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
		stem := "a\"\r\nX-Evil: 1"
		rec := get(s, gifPath+"?download="+url.QueryEscape(stem))

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

func TestMediaOnlyServesTenorHosts(t *testing.T) {
	stub := stubMedia(t, gifResponse)
	for name, cfg := range map[string]config.Config{
		"without media cache": {CacheDisabled: true},
		"with media cache":    cachingConfig(t),
	} {
		t.Run(name, func(t *testing.T) {
			s := newMediaServer(t, cfg)
			for _, target := range []string{
				"/media/example.com/cat.gif",
				"/avatar/example.com/cat.gif",
				"/media/media.tenor.com.evil.example/cat.gif",
				"/media/media.tenor.com:443/cat.gif",
				"/media/media.tenor.com/",
			} {
				if rec := get(s, target); rec.Code != http.StatusNotFound {
					t.Errorf("GET %s = %d, want 404", target, rec.Code)
				}
			}
		})
	}
	if n := stub.count(); n != 0 {
		t.Fatalf("%d requests reached Tenor, want none", n)
	}
}

func TestMediaStoredOnFirstRequest(t *testing.T) {
	stub := stubMedia(t, gifResponse)
	s := newMediaServer(t, cachingConfig(t))

	first := get(s, gifPath)
	if first.Code != http.StatusOK || first.Body.String() != gifBody {
		t.Fatalf("first response = %d %q, want 200 %q", first.Code, first.Body.String(), gifBody)
	}
	if got := first.Header().Get("Mezzo-Cache"); got != "MISS" {
		t.Fatalf("first Mezzo-Cache = %q, want MISS", got)
	}
	if got := first.Header().Get("Content-Type"); got != "image/gif" {
		t.Fatalf("Content-Type = %q, want image/gif", got)
	}

	// The query string is not part of what is stored
	for _, target := range []string{gifPath, gifPath + "?x=1", gifPath + "?x=2&download=cat"} {
		rec := get(s, target)
		if rec.Code != http.StatusOK || rec.Body.String() != gifBody {
			t.Fatalf("GET %s = %d %q, want 200 %q", target, rec.Code, rec.Body.String(), gifBody)
		}
		if got := rec.Header().Get("Mezzo-Cache"); got != "HIT" {
			t.Fatalf("GET %s Mezzo-Cache = %q, want HIT", target, got)
		}
	}

	if n := stub.count(); n != 1 {
		t.Fatalf("%d requests reached Tenor, want 1", n)
	}
	if got := stub.call(0).url; got != gifUpstream {
		t.Fatalf("Tenor was asked for %q, want %q", got, gifUpstream)
	}
}

func TestMediaStoredFileHonorsRange(t *testing.T) {
	stub := stubMedia(t, gifResponse)
	s := newMediaServer(t, cachingConfig(t))
	get(s, gifPath)

	rec := get(s, gifPath, "Range", "bytes=2-4")
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got, want := rec.Body.String(), gifBody[2:5]; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Content-Range"), "bytes 2-4/"+strconv.Itoa(len(gifBody)); got != want {
		t.Fatalf("Content-Range = %q, want %q", got, want)
	}
	if n := stub.count(); n != 1 {
		t.Fatalf("%d requests reached Tenor, want 1", n)
	}
}

func TestMediaRangeOnFirstRequestStillStoresTheWholeFile(t *testing.T) {
	// Browsers open <video> with a Range request, so a miss that only
	// forwarded the range would never be stored
	stub := stubMedia(t, gifResponse)
	s := newMediaServer(t, cachingConfig(t))

	rec := get(s, gifPath, "Range", "bytes=0-3")
	if rec.Code != http.StatusPartialContent || rec.Body.String() != gifBody[:4] {
		t.Fatalf("response = %d %q, want 206 %q", rec.Code, rec.Body.String(), gifBody[:4])
	}
	if got := stub.call(0).rng; got != "" {
		t.Fatalf("Tenor was sent Range %q, want the whole file", got)
	}

	if rec := get(s, gifPath); rec.Header().Get("Mezzo-Cache") != "HIT" {
		t.Fatalf("second Mezzo-Cache = %q, want HIT", rec.Header().Get("Mezzo-Cache"))
	}
	if n := stub.count(); n != 1 {
		t.Fatalf("%d requests reached Tenor, want 1", n)
	}
}

func TestMediaThatCannotBeStoredIsStreamed(t *testing.T) {
	big := strings.Repeat("x", 1<<20+1) // over the 1 MiB file limit

	tests := []struct {
		name string
		resp func() *http.Response
	}{
		{"not found", func() *http.Response { return mediaResponse(http.StatusNotFound, "text/plain", "gone") }},
		{"svg", func() *http.Response { return mediaResponse(http.StatusOK, "image/svg+xml", "<svg/>") }},
		{"over the file limit", func() *http.Response { return mediaResponse(http.StatusOK, "image/gif", big) }},
		{"unknown length", func() *http.Response {
			resp := mediaResponse(http.StatusOK, "image/gif", gifBody)
			resp.ContentLength = -1
			resp.Header.Del("Content-Length")
			return resp
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := stubMedia(t, func(*http.Request) *http.Response { return tt.resp() })
			s := newMediaServer(t, cachingConfig(t))
			want := tt.resp()
			wantBody, _ := io.ReadAll(want.Body)

			for i := 1; i <= 2; i++ {
				rec := get(s, gifPath)
				if rec.Code != want.StatusCode || rec.Body.String() != string(wantBody) {
					t.Fatalf("request %d = %d (%d bytes), want %d (%d bytes)", i, rec.Code, rec.Body.Len(), want.StatusCode, len(wantBody))
				}
				if got := rec.Header().Get("Mezzo-Cache"); got != "BYPASS" {
					t.Fatalf("request %d Mezzo-Cache = %q, want BYPASS", i, got)
				}
				if want.StatusCode != http.StatusOK && rec.Header().Get("Cache-Control") != "" {
					t.Fatalf("request %d: error response has Cache-Control %q", i, rec.Header().Get("Cache-Control"))
				}
				if n := stub.count(); n != i {
					t.Fatalf("after request %d, %d requests reached Tenor, want %d (nothing stored)", i, n, i)
				}
			}
		})
	}

	t.Run("range is passed on", func(t *testing.T) {
		stub := stubMedia(t, func(r *http.Request) *http.Response {
			if r.Header.Get("Range") != "" {
				resp := mediaResponse(http.StatusPartialContent, "image/svg+xml", "<svg")
				resp.Header.Set("Content-Range", "bytes 0-3/6")
				return resp
			}
			return mediaResponse(http.StatusOK, "image/svg+xml", "<svg/>")
		})
		s := newMediaServer(t, cachingConfig(t))

		rec := get(s, gifPath, "Range", "bytes=0-3")
		if rec.Code != http.StatusPartialContent || rec.Body.String() != "<svg" {
			t.Fatalf("response = %d %q, want 206 %q", rec.Code, rec.Body.String(), "<svg")
		}
		if got := rec.Header().Get("Content-Range"); got != "bytes 0-3/6" {
			t.Fatalf("Content-Range = %q, want %q", got, "bytes 0-3/6")
		}
		if n := stub.count(); n != 2 || stub.call(1).rng != "bytes=0-3" {
			t.Fatalf("Tenor saw %+v, want the whole file and then the range", stub.calls)
		}
	})
}

func TestMediaTruncatedDownloadIsNotStored(t *testing.T) {
	stub := stubMedia(t, func(*http.Request) *http.Response {
		resp := mediaResponse(http.StatusOK, "image/gif", "GIF89a")
		resp.ContentLength = 100
		resp.Header.Set("Content-Length", "100")
		return resp
	})
	s := newMediaServer(t, cachingConfig(t))

	for i := 1; i <= 2; i++ {
		if rec := get(s, gifPath); rec.Code != http.StatusBadGateway {
			t.Fatalf("request %d status = %d, want 502", i, rec.Code)
		}
		if n := stub.count(); n != i {
			t.Fatalf("after request %d, %d requests reached Tenor, want %d (nothing stored)", i, n, i)
		}
	}
}

func TestMediaConcurrentMissesShareOneFetch(t *testing.T) {
	release := make(chan struct{})
	stub := stubMedia(t, func(r *http.Request) *http.Response {
		<-release
		return gifResponse(r)
	})
	s := newMediaServer(t, cachingConfig(t))

	const requests = 8
	bodies := make([]string, requests)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bodies[i] = get(s, gifPath).Body.String()
		}()
	}
	time.Sleep(100 * time.Millisecond) // let the others queue up behind the first
	close(release)
	wg.Wait()

	for i, b := range bodies {
		if b != gifBody {
			t.Fatalf("request %d body = %q, want %q", i, b, gifBody)
		}
	}
	if n := stub.count(); n != 1 {
		t.Fatalf("%d requests reached Tenor, want 1", n)
	}
}

func TestMediaAndAvatarsHaveSeparateLifetimes(t *testing.T) {
	stubMedia(t, gifResponse)
	s := newMediaServer(t, cachingConfig(t)) // avatars: 6h

	for range 2 { // a miss, then a hit
		if got := get(s, gifPath).Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
			t.Fatalf("media Cache-Control = %q, want immutable", got)
		}
	}
	for range 2 { // a miss, then a hit
		rec := get(s, "/avatar/media.tenor.com/a/avatar.png")
		if got, want := rec.Header().Get("Cache-Control"), "public, max-age=21600"; got != want {
			t.Fatalf("avatar Cache-Control = %q, want %q", got, want)
		}
	}
}
