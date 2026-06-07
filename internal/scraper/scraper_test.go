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

import "testing"

func TestProxy(t *testing.T) {
	if got := Proxy(""); got != "" {
		t.Fatalf("Proxy(empty) = %q, want empty", got)
	}

	got := Proxy("https://media.tenor.com/example tiny.gif?x=1&y=2")
	want := "/proxy.gif?url=https%3A%2F%2Fmedia.tenor.com%2Fexample+tiny.gif%3Fx%3D1%26y%3D2"
	if got != want {
		t.Fatalf("Proxy URL = %q, want %q", got, want)
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

	if len(profile.SocialLinks) != 2 {
		t.Fatalf("len(SocialLinks) = %d, want 2", len(profile.SocialLinks))
	}
	if profile.SocialLinks[0].Tooltip != "Website" {
		t.Fatalf("first social tooltip = %q, want Website", profile.SocialLinks[0].Tooltip)
	}
	if profile.SocialLinks[1].Tooltip != "social" {
		t.Fatalf("second social tooltip = %q, want icon fallback", profile.SocialLinks[1].Tooltip)
	}

	if len(profile.GIFs) != 2 {
		t.Fatalf("len(GIFs) = %d, want 2", len(profile.GIFs))
	}
	first := profile.GIFs[0]
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

	second := profile.GIFs[1]
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
