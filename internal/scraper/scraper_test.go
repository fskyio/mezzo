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

package scraper

import (
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"mezzo/internal/client"

	"github.com/PuerkitoBio/goquery"
)

func TestMediaPath(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		media    string
		avatar   string
		download string
	}{
		{
			name:     "gif keeps its real file name",
			src:      "https://media1.tenor.com/m/AAAAC/cat.gif",
			media:    "/media/media1.tenor.com/m/AAAAC/cat.gif",
			avatar:   "/avatar/media1.tenor.com/m/AAAAC/cat.gif",
			download: "/media/media1.tenor.com/m/AAAAC/cat.gif?download=cat-hd",
		},
		{
			name:     "mp4 keeps its extension",
			src:      "https://c.tenor.com/x/AAAPo/cat.mp4",
			media:    "/media/c.tenor.com/x/AAAPo/cat.mp4",
			avatar:   "/avatar/c.tenor.com/x/AAAPo/cat.mp4",
			download: "/media/c.tenor.com/x/AAAPo/cat.mp4?download=cat-hd",
		},
		{
			name:     "path stays escaped and the query is dropped",
			src:      "https://media.tenor.com/example%20tiny.gif?x=1&y=2",
			media:    "/media/media.tenor.com/example%20tiny.gif",
			avatar:   "/avatar/media.tenor.com/example%20tiny.gif",
			download: "/media/media.tenor.com/example%20tiny.gif?download=cat-hd",
		},
		{name: "empty", src: ""},
		{name: "other host", src: "https://example.com/cat.gif"},
		{name: "host that only contains a Tenor host", src: "https://media.tenor.com.evil.example/cat.gif"},
		{name: "tenor host in the query of another host", src: "https://example.com/cat.gif?u=media.tenor.com"},
		{name: "plain http", src: "http://media.tenor.com/cat.gif"},
		{name: "port", src: "https://media.tenor.com:8443/cat.gif"},
		{name: "no path", src: "https://media.tenor.com"},
		{name: "root path", src: "https://media.tenor.com/"},
		{name: "dot dot segment", src: "https://media.tenor.com/a/../b.gif"},
		{name: "trailing slash", src: "https://media.tenor.com/a/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MediaPath(tt.src); got != tt.media {
				t.Errorf("MediaPath = %q, want %q", got, tt.media)
			}
			if got := AvatarPath(tt.src); got != tt.avatar {
				t.Errorf("AvatarPath = %q, want %q", got, tt.avatar)
			}
			if got := MediaDownload(tt.src, "cat-hd"); got != tt.download {
				t.Errorf("MediaDownload = %q, want %q", got, tt.download)
			}
		})
	}
}

func TestMediaURL(t *testing.T) {
	tests := []struct {
		name string
		host string
		path string
		want string // empty when rejected
	}{
		{"media host", "media.tenor.com", "a/b.gif", "https://media.tenor.com/a/b.gif"},
		{"media1 host", "media1.tenor.com", "m/AAAAC/cat.gif", "https://media1.tenor.com/m/AAAAC/cat.gif"},
		{"c host", "c.tenor.com", "a.png", "https://c.tenor.com/a.png"},
		{"unknown host", "example.com", "a.gif", ""},
		{"host with port", "media.tenor.com:443", "a.gif", ""},
		{"user info", "user@media.tenor.com", "a.gif", ""},
		{"empty host", "", "a.gif", ""},
		{"empty path", "media.tenor.com", "", ""},
		{"dot dot", "media.tenor.com", "a/../b.gif", ""},
		{"leading dot dot", "media.tenor.com", "../b.gif", ""},
		{"empty segment", "media.tenor.com", "a//b.gif", ""},
		{"trailing slash", "media.tenor.com", "a/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MediaURL(tt.host, tt.path)
			if ok != (tt.want != "") {
				t.Fatalf("MediaURL(%q, %q) ok = %v, want %v", tt.host, tt.path, ok, tt.want != "")
			}
			if ok && got.String() != tt.want {
				t.Fatalf("MediaURL(%q, %q) = %q, want %q", tt.host, tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTenorMedia(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{src: "https://media.tenor.com/example.gif", want: true},
		{src: "https://media1.tenor.com/example.gif", want: true},
		{src: "https://c.tenor.com/example.gif", want: true},
		{src: "https://example.com/example.gif", want: false},
	}

	for _, tt := range tests {
		if got := isTenorMedia(tt.src); got != tt.want {
			t.Fatalf("isTenorMedia(%q) = %v, want %v", tt.src, got, tt.want)
		}
	}
}

func TestExtractURLFromStyle(t *testing.T) {
	tests := []struct {
		name  string
		style string
		want  string
	}{
		{
			name:  "double quoted url",
			style: `background-image:url("https://media.tenor.com/avatar.png");`,
			want:  "https://media.tenor.com/avatar.png",
		},
		{
			name:  "single quoted url",
			style: `background-image: url('https://media.tenor.com/banner.png')`,
			want:  "https://media.tenor.com/banner.png",
		},
		{
			name:  "bare url",
			style: `background-image:url(https://media.tenor.com/bare.png)`,
			want:  "https://media.tenor.com/bare.png",
		},
		{
			name:  "missing url",
			style: `color: red`,
			want:  "",
		},
		{
			name:  "missing close paren",
			style: `background-image:url(https://media.tenor.com/broken.png`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractURLFromStyle(tt.style); got != tt.want {
				t.Fatalf("extractURLFromStyle(%q) = %q, want %q", tt.style, got, tt.want)
			}
		})
	}
}

func TestParseStoreCache(t *testing.T) {
	jsonText := `{
		"profiles": {
			"user:mezzo": {
				"user": {
					"username": "mezzo",
					"partnername": "Mezzo Official",
					"tagline": "Tiny GIF gateway",
					"usertype": "partner",
					"avatars": {
						"32": "https://media.tenor.com/avatar-32.png",
						"256": "https://media.tenor.com/avatar-256.png"
					},
					"partnerbanner": {
						"690": "https://media.tenor.com/banner-690.png",
						"1110": "https://media.tenor.com/banner-1110.png"
					},
					"partnercta": {
						"url": "https://example.com/watch",
						"text": "Watch now"
					},
					"partnerlinks": [
						{
							"url": "https://example.com",
							"tooltip": "Website",
							"icon": "link"
						},
						{
							"url": "https://social.example/mezzo",
							"tooltip": "",
							"icon": "social"
						},
						{
							"url": "https://www.tiktok.com/@mezzo",
							"tooltip": "Other",
							"icon": "other"
						},
						{
							"url": "https://www.generic.example/mezzo",
							"tooltip": "Other",
							"icon": "other"
						},
						{
							"url": "",
							"tooltip": "Ignored",
							"icon": "ignored"
						}
					]
				}
			}
		},
		"gifs": {
			"searchByUsername": {
				"mezzo-gifs-profile-public": {
					"results": [
						{
							"id": "1",
							"title": "Title fallback",
							"h1_title": "H1 fallback",
							"content_description": "A waving cat",
							"itemurl": "https://tenor.com/view/waving-cat-1",
							"media_formats": {
								"tinygif": {
									"url": "https://media.tenor.com/waving-cat-tiny.gif",
									"dims": [220, 123]
								}
							}
						},
						{
							"id": "2",
							"title": "Second title",
							"h1_title": "",
							"content_description": "",
							"itemurl": "https://tenor.com/view/second-cat-2",
							"media_formats": {
								"gif": {
									"url": "https://media.tenor.com/second-cat.gif",
									"dims": [320, 240]
								}
							}
						},
						{
							"id": "3",
							"title": "Skipped",
							"itemurl": "https://tenor.com/view/skipped-3",
							"media_formats": {}
						}
					]
				}
			}
		}
	}`

	profile, ok := parseStoreCache(jsonText, "/official/mezzo")
	if !ok {
		t.Fatal("parseStoreCache returned ok = false, want true")
	}

	if profile.Username != "mezzo" {
		t.Fatalf("Username = %q, want mezzo", profile.Username)
	}
	if profile.DisplayName != "Mezzo Official" {
		t.Fatalf("DisplayName = %q, want Mezzo Official", profile.DisplayName)
	}
	if profile.Tagline != "Tiny GIF gateway" {
		t.Fatalf("Tagline = %q, want Tiny GIF gateway", profile.Tagline)
	}
	if profile.UserType != "partner" {
		t.Fatalf("UserType = %q, want partner", profile.UserType)
	}
	if profile.AvatarURL != "https://media.tenor.com/avatar-256.png" {
		t.Fatalf("AvatarURL = %q, want largest avatar", profile.AvatarURL)
	}
	if profile.BannerURL != "https://media.tenor.com/banner-1110.png" {
		t.Fatalf("BannerURL = %q, want largest banner", profile.BannerURL)
	}

	if len(profile.SocialLinks) != 4 {
		t.Fatalf("len(SocialLinks) = %d, want 4", len(profile.SocialLinks))
	}
	if profile.SocialLinks[0].Tooltip != "Website" {
		t.Fatalf("first social tooltip = %q, want Website", profile.SocialLinks[0].Tooltip)
	}
	if profile.SocialLinks[1].Tooltip != "Social" {
		t.Fatalf("second social tooltip = %q, want icon fallback", profile.SocialLinks[1].Tooltip)
	}
	if profile.SocialLinks[2].Tooltip != "TikTok" {
		t.Fatalf("third social tooltip = %q, want host label fallback", profile.SocialLinks[2].Tooltip)
	}
	if profile.SocialLinks[3].Tooltip != "generic.example" {
		t.Fatalf("fourth social tooltip = %q, want hostname fallback", profile.SocialLinks[3].Tooltip)
	}
	if profile.CTA == nil {
		t.Fatal("CTA = nil, want parsed CTA")
	}
	if profile.CTA.URL != "https://example.com/watch" {
		t.Fatalf("CTA URL = %q, want https://example.com/watch", profile.CTA.URL)
	}
	if profile.CTA.Label != "Watch now" {
		t.Fatalf("CTA Label = %q, want Watch now", profile.CTA.Label)
	}

	if len(profile.GIFs.Results) != 2 {
		t.Fatalf("len(GIFs) = %d, want 2", len(profile.GIFs.Results))
	}
	first := profile.GIFs.Results[0]
	if first.URL != "/view/waving-cat-1" {
		t.Fatalf("first GIF URL = %q, want /view/waving-cat-1", first.URL)
	}
	if first.ImageURL != "https://media.tenor.com/waving-cat-tiny.gif" {
		t.Fatalf("first GIF ImageURL = %q, want tinygif URL", first.ImageURL)
	}
	if first.Alt != "A waving cat" {
		t.Fatalf("first GIF Alt = %q, want content description", first.Alt)
	}
	if first.Width != 220 || first.Height != 123 {
		t.Fatalf("first GIF dimensions = %dx%d, want 220x123", first.Width, first.Height)
	}

	second := profile.GIFs.Results[1]
	if second.ImageURL != "https://media.tenor.com/second-cat.gif" {
		t.Fatalf("second GIF ImageURL = %q, want gif fallback URL", second.ImageURL)
	}
	if second.Alt != "Second title" {
		t.Fatalf("second GIF Alt = %q, want title fallback", second.Alt)
	}
}

func TestParseStoreCacheRejectsInvalidOrEmptyData(t *testing.T) {
	tests := []string{
		`not-json`,
		`{"profiles":{}}`,
		`{"profiles":{"user:empty":{"user":{"username":""}}}}`,
	}

	for _, jsonText := range tests {
		if profile, ok := parseStoreCache(jsonText, "/users/mezzo"); ok || profile != nil {
			t.Fatalf("parseStoreCache(%q) = (%#v, %v), want (nil, false)", jsonText, profile, ok)
		}
	}
}

func TestParseStoreCacheProfileUploads(t *testing.T) {
	// Tenor keys the uploads by lowercased username, while the profile keeps
	// the name's own case. Stickers and memes are separate sets with their own
	// cursors.
	const blob = `{
		"profiles": {"p": {"user": {"username": "BenJammins", "usertype": "partner"}}},
		"gifs": {"searchByUsername": {"benjammins-gifs-profile-public": {
			"next": "gif-next",
			"results": [{"itemurl": "https://tenor.com/view/gif-1", "media_formats": {"tinygif": {"url": "https://media.tenor.com/g.gif"}}}]
		}}},
		"stickers": {"searchByUsername": {"benjammins-stickers-profile-public": {
			"next": "sticker-next",
			"results": [
				{"itemurl": "https://tenor.com/view/sticker-2", "media_formats": {"tinygif": {"url": "https://media.tenor.com/s1.gif"}}},
				{"itemurl": "https://tenor.com/view/sticker-3", "media_formats": {"tinygif": {"url": "https://media.tenor.com/s2.gif"}}}
			]
		}}},
		"memes": {"searchByUsername": {"benjammins-memes-profile-public": {
			"next": "",
			"results": [{"itemurl": "https://tenor.com/view/meme-4", "media_formats": {"tinygif": {"url": "https://media.tenor.com/m.gif"}}}]
		}}}
	}`

	profile, ok := parseStoreCache(blob, "/official/benjammins")
	if !ok {
		t.Fatal("parseStoreCache returned ok = false, want true")
	}

	tests := []struct {
		kind     ProfileKind
		wantURLs []string
		wantNext string
	}{
		{ProfileGIFs, []string{"/view/gif-1"}, "gif-next"},
		{ProfileStickers, []string{"/view/sticker-2", "/view/sticker-3"}, "sticker-next"},
		{ProfileMemes, []string{"/view/meme-4"}, ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			got := profile.Uploads(tt.kind)
			var urls []string
			for _, r := range got.Results {
				urls = append(urls, r.URL)
			}
			if !slices.Equal(urls, tt.wantURLs) || got.Next != tt.wantNext {
				t.Fatalf("Uploads(%s) = %v next %q, want %v next %q", tt.kind, urls, got.Next, tt.wantURLs, tt.wantNext)
			}
		})
	}
}

func TestParseProfileDOMFallbackReadsOnlyGIFs(t *testing.T) {
	// Without a usable store-cache the page's DOM is all there is. Its
	// sticker and meme carousels must not leak into the GIF list.
	const page = `<html><body><div class="ProfilePageHeader"><h1>Mezzo</h1></div>
		<div class="media-section"><h2>Mezzo Stickers</h2><div class="Carousel">
			<figure><a href="/view/sticker-1"><img src="https://media.tenor.com/s.gif" alt="sticker"></a></figure>
		</div></div>
		<div class="media-section"><h2>Mezzo GIFs</h2><div class="UniversalGifList">
			<figure><a href="/view/gif-2"><img src="https://media.tenor.com/g.gif" alt="gif" width="10" height="20"></a></figure>
			<figure><a href="/gif-maker"><img src="https://media.tenor.com/maker.gif"></a></figure>
		</div></div></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}

	profile := parseProfile(doc, "/users/mezzo")

	want := []SearchResult{{URL: "/view/gif-2", ImageURL: "https://media.tenor.com/g.gif", Alt: "gif", Width: 10, Height: 20}}
	if !slices.Equal(profile.GIFs.Results, want) {
		t.Fatalf("GIFs = %+v, want %+v", profile.GIFs.Results, want)
	}
	if len(profile.Stickers.Results) != 0 || len(profile.Memes.Results) != 0 {
		t.Fatalf("stickers %+v and memes %+v, want none", profile.Stickers.Results, profile.Memes.Results)
	}
}

func TestParseProfileKeepsStickersOutOfEmptyGIFList(t *testing.T) {
	// A user with only stickers has an empty GIF set. The DOM fallback must
	// not run just because of that and fill the GIF list with the stickers.
	const blob = `{
		"profiles": {"p": {"user": {"username": "Mezzo"}}},
		"gifs": {"searchByUsername": {"mezzo-gifs-profile-public": {"results": []}}},
		"stickers": {"searchByUsername": {"mezzo-stickers-profile-public": {
			"results": [{"itemurl": "https://tenor.com/view/sticker-1", "media_formats": {"tinygif": {"url": "https://media.tenor.com/s.gif"}}}]
		}}}
	}`
	page := `<html><body><script id="store-cache">` + blob + `</script>
		<div class="UniversalGifList"><figure><a href="/view/sticker-1"><img src="https://media.tenor.com/s.gif"></a></figure></div>
		</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}

	profile := parseProfile(doc, "/users/mezzo")

	if len(profile.Stickers.Results) != 1 {
		t.Fatalf("Stickers = %+v, want the one sticker", profile.Stickers.Results)
	}
	if len(profile.GIFs.Results) != 0 {
		t.Fatalf("GIFs = %+v, want none", profile.GIFs.Results)
	}
}

func TestParseGifFormats(t *testing.T) {
	const blob = `{"gifs":{"byId":{"123":{"results":[{"media_formats":{
		"mediumgif":{"url":"https://media1.tenor.com/m/x/AAAAd/cat.gif"},
		"gif":{"url":"https://media1.tenor.com/m/x/AAAAC/cat.gif"},
		"mp4":{"url":"https://media.tenor.com/x/AAAPo/cat.mp4"},
		"tinygif":{"url":"https://media.tenor.com/x/AAAAM/cat.gif"}
	}}]}}}}`

	t.Run("maps tenor formats in switcher order", func(t *testing.T) {
		got := parseGifFormats(blob, "/view/cat-gif-123")
		want := []GifFormat{
			{Key: "sd", Label: "SD GIF", URL: "https://media1.tenor.com/m/x/AAAAd/cat.gif"},
			{Key: "hd", Label: "HD GIF", URL: "https://media1.tenor.com/m/x/AAAAC/cat.gif"},
			{Key: "mp4", Label: "MP4", URL: "https://media.tenor.com/x/AAAPo/cat.mp4", Video: true},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d formats, want %d: %+v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("format %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("skips formats tenor does not offer", func(t *testing.T) {
		got := parseGifFormats(`{"gifs":{"byId":{"1":{"results":[{"media_formats":{
			"mediumgif":{"url":"https://media.tenor.com/sd.gif"},
			"gif":{"url":""}
		}}]}}}}`, "/view/cat-gif-1")
		if len(got) != 1 || got[0].Key != "sd" {
			t.Fatalf("got %+v, want only the SD GIF", got)
		}
	})

	t.Run("falls back to the only entry when the slug id is stale", func(t *testing.T) {
		if got := parseGifFormats(blob, "/view/cat-gif-999"); len(got) != 3 {
			t.Fatalf("got %d formats, want 3", len(got))
		}
	})

	t.Run("does not guess between several entries", func(t *testing.T) {
		two := `{"gifs":{"byId":{
			"1":{"results":[{"media_formats":{"gif":{"url":"https://media.tenor.com/a.gif"}}}]},
			"2":{"results":[{"media_formats":{"gif":{"url":"https://media.tenor.com/b.gif"}}}]}
		}}}`
		if got := parseGifFormats(two, "/view/cat-gif-3"); got != nil {
			t.Fatalf("got %+v, want nil", got)
		}
	})

	t.Run("returns nil for missing or malformed blobs", func(t *testing.T) {
		for _, text := range []string{"", "not json", `{}`, `{"gifs":{"byId":{"1":{"results":[]}}}}`} {
			if got := parseGifFormats(text, "/view/cat-gif-1"); got != nil {
				t.Fatalf("parseGifFormats(%q) = %+v, want nil", text, got)
			}
		}
	})
}

func TestGifPageFormatFallsBackToDefault(t *testing.T) {
	page := &GifPage{Formats: []GifFormat{
		{Key: "sd", URL: "sd.gif"},
		{Key: "mp4", URL: "cat.mp4", Video: true},
	}}

	if got := page.Format("mp4"); got.URL != "cat.mp4" {
		t.Fatalf("Format(mp4) = %+v, want the MP4", got)
	}
	// "hd" is a valid key that this page does not offer; "" and junk are invalid.
	for _, key := range []string{"hd", "", "../etc/passwd"} {
		if got := page.Format(key); got.URL != "sd.gif" {
			t.Fatalf("Format(%q) = %+v, want the default SD GIF", key, got)
		}
	}
}

// stubAPI answers every Tenor request with body and returns the URLs requested.
func stubAPI(t *testing.T, body string) *[]*url.URL {
	t.Helper()
	var got []*url.URL
	orig := client.Default.Transport
	client.Default.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = append(got, r.URL)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})
	t.Cleanup(func() { client.Default.Transport = orig })
	return &got
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const apiGIFs = `{"next":"n2","results":[` +
	`{"itemurl":"https://tenor.com/view/a-gif-1","content_description":"a","media_formats":{"tinygif":{"url":"https://media.tenor.com/a.gif","dims":[10,20]}}},` +
	`{"itemurl":"https://tenor.com/view/no-media-gif-2","media_formats":{}},` +
	`{"itemurl":"","media_formats":{"tinygif":{"url":"https://media.tenor.com/c.gif"}}}]}`

func TestAPISearchAndFeatured(t *testing.T) {
	tests := []struct {
		name     string
		call     func() (*SearchResponse, error)
		wantPath string
		wantQ    string
	}{
		{"search", func() (*SearchResponse, error) { return APISearch("funny cats", "p1") }, "/v2/search", "funny cats"},
		{"featured", func() (*SearchResponse, error) { return APIFeatured("p1") }, "/v2/featured", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := stubAPI(t, apiGIFs)
			resp, err := tt.call()
			if err != nil {
				t.Fatal(err)
			}

			if len(*reqs) != 1 {
				t.Fatalf("made %d requests, want 1", len(*reqs))
			}
			u := (*reqs)[0]
			q := u.Query()
			if u.Path != tt.wantPath || q.Get("q") != tt.wantQ || q.Has("q") != (tt.wantQ != "") {
				t.Fatalf("requested %s, want path %s with q=%q", u, tt.wantPath, tt.wantQ)
			}
			if q.Get("pos") != "p1" || q.Get("limit") != "30" || q.Get("media_filter") != "tinygif" ||
				q.Get("contentfilter") != "low" || q.Get("key") != TenorAPIKey || q.Get("client_key") != TenorClientKey {
				t.Fatalf("unexpected query %s", u.RawQuery)
			}

			if resp.Next != "n2" {
				t.Fatalf("Next = %q, want n2", resp.Next)
			}
			// Results without media or a view link cannot be rendered
			want := SearchResult{URL: "/view/a-gif-1", ImageURL: "https://media.tenor.com/a.gif", Alt: "a", Width: 10, Height: 20}
			if len(resp.Results) != 1 || resp.Results[0] != want {
				t.Fatalf("Results = %+v, want [%+v]", resp.Results, want)
			}
		})
	}
}

func TestAPIProfileSelectsKindWithSearchFilter(t *testing.T) {
	tests := []struct {
		kind       ProfileKind
		wantFilter string // empty: the parameter must be absent
	}{
		{ProfileGIFs, ""},
		{ProfileStickers, "sticker"},
		{ProfileMemes, "static,-sticker"},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			reqs := stubAPI(t, apiGIFs)
			resp, err := APIProfile("BenJammins", tt.kind, "p1")
			if err != nil {
				t.Fatal(err)
			}

			u := (*reqs)[0]
			q := u.Query()
			if u.Path != "/v2/search" || q.Get("q") != "@BenJammins" || q.Get("pos") != "p1" || q.Get("limit") != "50" {
				t.Fatalf("requested %s, want /v2/search for @BenJammins at pos p1 with limit 50", u)
			}
			if q.Get("searchfilter") != tt.wantFilter || q.Has("searchfilter") != (tt.wantFilter != "") {
				t.Fatalf("requested %s, want searchfilter %q", u.RawQuery, tt.wantFilter)
			}
			if resp.Next != "n2" || len(resp.Results) != 1 {
				t.Fatalf("got next %q and %d results, want n2 and 1", resp.Next, len(resp.Results))
			}
		})
	}
}

func TestAPIFeaturedFirstPageSendsNoCursor(t *testing.T) {
	reqs := stubAPI(t, apiGIFs)
	if _, err := APIFeatured(""); err != nil {
		t.Fatal(err)
	}
	if (*reqs)[0].Query().Has("pos") {
		t.Fatalf("first page sent a cursor: %s", (*reqs)[0].RawQuery)
	}
}

func TestAPITrending(t *testing.T) {
	reqs := stubAPI(t, `{"tags":[`+
		`{"searchterm":"good night","image":"https://media.tenor.com/a.gif"},`+
		`{"searchterm":"","image":"https://media.tenor.com/b.gif"},`+
		`{"searchterm":"no image"}]}`)

	trends, err := APITrending()
	if err != nil {
		t.Fatal(err)
	}

	u := (*reqs)[0]
	if u.Path != "/v2/categories" || u.Query().Get("type") != "trending" {
		t.Fatalf("requested %s, want /v2/categories?type=trending", u)
	}

	want := []Trend{{Term: "good night", ImageURL: "https://media.tenor.com/a.gif"}, {Term: "no image"}}
	if len(trends) != len(want) || trends[0] != want[0] || trends[1] != want[1] {
		t.Fatalf("trends = %+v, want %+v", trends, want)
	}
}
