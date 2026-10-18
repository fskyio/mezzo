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
	"context"
	"io"
	"log/slog"
	"mezzo/internal/client"
	"mezzo/internal/mediacache"
	"mezzo/internal/scraper"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
)

// Media kinds. The route a file is requested through is its kind, and the
// kind selects how long browsers and the media cache may keep it.
const (
	kindMedia  = "media"  // GIFs, MP4s, thumbnails
	kindAvatar = "avatar" // profile avatars and banners
)

// mediaCacheControl is the Cache-Control for kindMedia. Tenor's media URLs
// contain a content id, so the file behind a URL does not change.
const mediaCacheControl = "public, max-age=604800, immutable"

// mediaFill is the outcome of looking up or fetching a file: either a stored
// file to serve, or an upstream response to stream because it can't be stored.
type mediaFill struct {
	file   *os.File
	info   mediacache.Info
	resp   *http.Response
	status string // value for the Mezzo-Cache header
}

// handleMedia serves a file from Tenor through Mezzo, from the media cache
// when there is one.
func (s *Server) handleMedia(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upstream, ok := scraper.MediaURL(r.PathValue("host"), r.PathValue("path"))
		if !ok {
			s.handleError(w, r, http.StatusNotFound)
			return
		}

		if s.mediaStore == nil {
			resp, err := fetchMedia(r.Context(), upstream, r.Header.Get("Range"))
			if err != nil {
				s.mediaFetchFailed(w, r, err)
				return
			}
			s.streamUpstream(w, r, kind, upstream, resp, "")
			return
		}

		key := mediacache.Key(upstream.Host + upstream.Path)
		if f, info, ok := s.mediaStore.Get(kind, key); ok {
			s.serveStored(w, r, kind, upstream, f, info, "HIT")
			return
		}
		fill, err := s.fillMedia(r, kind, key, upstream)
		if err != nil {
			s.mediaFetchFailed(w, r, err)
			return
		}
		if fill.file != nil {
			s.serveStored(w, r, kind, upstream, fill.file, fill.info, fill.status)
			return
		}
		s.streamUpstream(w, r, kind, upstream, fill.resp, fill.status)
	}
}

// fillMedia returns the stored file for key, fetching it from Tenor first if
// it is not stored yet. Concurrent requests for the same file wait for one
// fetch. If the response can't be stored it is returned unread instead.
func (s *Server) fillMedia(r *http.Request, kind, key string, upstream *url.URL) (mediaFill, error) {
	ctx := r.Context()
	unlock, err := s.mediaStore.Lock(ctx, kind, key)
	if err != nil {
		return mediaFill{}, err
	}
	defer unlock()

	// The request that held the lock before this one may have stored the file
	if f, info, ok := s.mediaStore.Get(kind, key); ok {
		return mediaFill{file: f, info: info, status: "HIT"}, nil
	}

	// Fetch the whole file whatever Range the browser sent. A partial
	// response can't be stored, and ranges are served from the stored file.
	resp, err := fetchMedia(ctx, upstream, "")
	if err != nil {
		return mediaFill{}, err
	}
	if resp.StatusCode == http.StatusOK {
		if contentType, ok := s.mediaStore.Accepts(resp.Header.Get("Content-Type"), resp.ContentLength); ok {
			defer resp.Body.Close()
			f, info, err := s.mediaStore.Put(kind, key, contentType, resp.ContentLength, resp.Body)
			if err != nil {
				return mediaFill{}, err
			}
			return mediaFill{file: f, info: info, status: "MISS"}, nil
		}
	}

	// Not storable, so stream it as is, honoring Range this time
	if rng := r.Header.Get("Range"); rng != "" {
		resp.Body.Close()
		if resp, err = fetchMedia(ctx, upstream, rng); err != nil {
			return mediaFill{}, err
		}
	}
	return mediaFill{resp: resp, status: "BYPASS"}, nil
}

// fetchMedia requests upstream from Tenor, with the Range header rng if it is
// not empty.
func fetchMedia(ctx context.Context, upstream *url.URL, rng string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.String(), nil)
	if err != nil {
		return nil, err
	}
	// Browsers need range support to play and seek <video>; Safari refuses to
	// play video from a server that ignores Range.
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	// Ask for the file as is. A body that Go decompressed has no known size,
	// and the media cache needs the size to notice a truncated download.
	req.Header.Set("Accept-Encoding", "identity")
	return client.Default.Do(req)
}

// mediaFetchFailed answers a request whose file could not be fetched.
func (s *Server) mediaFetchFailed(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		// The browser went away, there is nobody to answer
		return
	}
	slog.Error("failed to fetch media", "error", err)
	s.handleError(w, r, http.StatusBadGateway)
}

// serveStored serves a file from the media cache, with support for Range and
// conditional requests. It closes f.
func (s *Server) serveStored(w http.ResponseWriter, r *http.Request, kind string, upstream *url.URL, f *os.File, info mediacache.Info, status string) {
	defer f.Close()

	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("Cache-Control", s.cacheControl(kind))
	w.Header().Set("Mezzo-Cache", status)
	setDisposition(w, r, upstream)
	http.ServeContent(w, r, "", info.ModTime, f)
}

// streamUpstream copies a response from Tenor to the browser. It closes the
// body of resp. An empty status leaves out the Mezzo-Cache header.
func (s *Server) streamUpstream(w http.ResponseWriter, r *http.Request, kind string, upstream *url.URL, resp *http.Response, status string) {
	defer resp.Body.Close()

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// Errors are not worth keeping, least of all for a week
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		w.Header().Set("Cache-Control", s.cacheControl(kind))
	}
	if status != "" {
		w.Header().Set("Mezzo-Cache", status)
	}
	setDisposition(w, r, upstream)

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("failed to copy media body", "error", err)
	}
}

func (s *Server) cacheControl(kind string) string {
	if kind == kindAvatar {
		return s.avatarCacheControl
	}
	return mediaCacheControl
}

// setDisposition names the file for the browser, and makes it save the file
// instead of showing it if the request has a download parameter.
func setDisposition(w http.ResponseWriter, r *http.Request, upstream *url.URL) {
	disposition := "inline"
	filename := path.Base(upstream.Path)
	if q := r.URL.Query(); q.Has("download") {
		disposition = "attachment"
		// The extension always comes from the Tenor URL so a link cannot
		// change the type of the saved file
		if stem := q.Get("download"); stem != "" {
			filename = path.Base(stem) + path.Ext(upstream.Path)
		}
	}
	if filename != "" && filename != "." {
		// FormatMediaType quotes and escapes the name
		if v := mime.FormatMediaType(disposition, map[string]string{"filename": filename}); v != "" {
			w.Header().Set("Content-Disposition", v)
		}
	}
}
