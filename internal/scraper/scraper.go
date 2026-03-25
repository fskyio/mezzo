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
	"encoding/json"
	"fmt"
	"mezzo/internal/client"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const (
	TenorBaseURL = "https://tenor.com"
)

// SearchResult represents a single item in the search results
type SearchResult struct {
	URL      string
	ImageURL string
	Alt      string
}

// GifPage represents the scraped data from a GIF view page
type GifPage struct {
	Title        string
	ImageURL     string
	Author       string
	AuthorURL    string
	AuthorAvatar string
	UploadDate   string
	Tags         []string
	Description  string
	FileSize     string
	Duration     string
	Dimensions   string
	Created      string
}

// Proxy generates a proxied URL for a given image URL
func Proxy(imageURL string) string {
	if imageURL == "" {
		return ""
	}
	return fmt.Sprintf("/proxy.gif?url=%s", url.QueryEscape(imageURL))
}

func isTenorMedia(src string) bool {
	return strings.Contains(src, "media.tenor.com") || strings.Contains(src, "media1.tenor.com") || strings.Contains(src, "c.tenor.com")
}

// GetGif scrapes a Tenor GIF page for the title and image URL
func GetGif(path string) (*GifPage, error) {
	fullURL := TenorBaseURL + path
	doc, err := FetchDocument(fullURL)
	if err != nil {
		return nil, err
	}

	title := doc.Find("h1").First().Text()

	var imageURL string
	doc.Find("div > div > div > div > div > img").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if exists && isTenorMedia(src) {
			if imageURL == "" {
				imageURL = src
			}
		}
	})

	// Fallback: if the primary selector doesn't match, scan all images for Tenor media URLs
	if imageURL == "" {
		doc.Find("img").Each(func(i int, s *goquery.Selection) {
			src, exists := s.Attr("src")
			if exists && isTenorMedia(src) {
				if imageURL == "" {
					imageURL = src
				}
			}
		})
	}

	if title == "" {
		// Fallback: use the document title and strip Tenor's suffix
		title = doc.Find("title").Text()
		title = strings.TrimSuffix(title, " - Tenor GIF Keyboard")
	}

	if imageURL == "" {
		return nil, fmt.Errorf("could not find image URL")
	}

	// Extract metadata
	author := doc.Find("meta[itemprop='author']").AttrOr("content", "")
	authorURL := ""
	authorAvatar := ""
	doc.Find("a.author-username").Each(func(i int, s *goquery.Selection) {
		if href, ok := s.Attr("href"); ok {
			// Keep as a local path so it links to the Mezzo profile
			authorURL = href
		}
	})

	// Try to find the avatar from the profile-info section
	doc.Find(".profile-info .ProfileImage").Each(func(i int, s *goquery.Selection) {
		style, ok := s.Attr("style")
		if ok && strings.Contains(style, "background-image") {
			// background-image:url("https://...");
			start := strings.Index(style, "url(")
			if start != -1 {
				style = style[start+4:]
				end := strings.Index(style, ")")
				if end != -1 {
					avatar := style[:end]
					avatar = strings.Trim(avatar, "\"")
					avatar = strings.Trim(avatar, "'")
					authorAvatar = avatar
				}
			}
		}
	})

	uploadDate := doc.Find("meta[itemprop='uploadDate']").AttrOr("content", "")

	var tags []string
	doc.Find("ul.tag-list li a div").Each(func(i int, s *goquery.Selection) {
		tags = append(tags, s.Text())
	})
	// Fallback for tags if the structure is slightly different (e.g. just a inside li)
	if len(tags) == 0 {
		doc.Find("ul.tag-list li a").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				tags = append(tags, text)
			}
		})
	}

	// Extract additional details from <dl> inside .gif-details
	description := ""
	fileSize := ""
	duration := ""
	dimensions := ""
	created := ""

	doc.Find(".gif-details.non-mobile-only dl dd").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if strings.HasPrefix(text, "Content Description:") {
			description = strings.TrimSpace(strings.TrimPrefix(text, "Content Description:"))
		} else if strings.HasPrefix(text, "File Size:") {
			fileSize = strings.TrimSpace(strings.TrimPrefix(text, "File Size:"))
		} else if strings.HasPrefix(text, "Duration:") {
			duration = strings.TrimSpace(strings.TrimPrefix(text, "Duration:"))
		} else if strings.HasPrefix(text, "Dimensions:") {
			dimensions = strings.TrimSpace(strings.TrimPrefix(text, "Dimensions:"))
		} else if strings.HasPrefix(text, "Created:") {
			created = strings.TrimSpace(strings.TrimPrefix(text, "Created:"))
		}
	})

	return &GifPage{
		Title:        title,
		ImageURL:     imageURL,
		Author:       author,
		AuthorURL:    authorURL,
		AuthorAvatar: authorAvatar,
		UploadDate:   uploadDate,
		Tags:         tags,
		Description:  description,
		FileSize:     fileSize,
		Duration:     duration,
		Dimensions:   dimensions,
		Created:      created,
	}, nil
}

// GetSearch scrapes a Tenor search page for results
func GetSearch(path string) ([]SearchResult, error) {
	fullURL := TenorBaseURL + path
	doc, err := FetchDocument(fullURL)
	if err != nil {
		return nil, err
	}

	var results []SearchResult

	// Tenor search results are linked from <figure><a ...> elements
	doc.Find("figure a").Each(func(i int, s *goquery.Selection) {
		href, hrefExists := s.Attr("href")
		if !hrefExists {
			return
		}

		// The preview image is nested inside the anchor
		img := s.Find("img").First()
		alt, _ := img.Attr("alt")
		src, srcExists := img.Attr("src")
		if !srcExists {
			return
		}

		// Filter out gif-maker links
		if strings.HasPrefix(href, "/gif-maker") {
			return
		}

		results = append(results, SearchResult{
			URL:      href,
			ImageURL: src,
			Alt:      alt,
		})
	})

	return results, nil
}

// SocialLink represents a link on a user's profile (e.g. Twitter, Instagram)
type SocialLink struct {
	URL     string
	Tooltip string
}

// ProfilePage represents scraped data from a Tenor user profile page
type ProfilePage struct {
	Username    string
	DisplayName string
	Tagline     string
	AvatarURL   string
	BannerURL   string
	UserType    string // "partner" or "user"
	SocialLinks []SocialLink
	GIFs        []SearchResult
}

// storeCacheData represents the top-level JSON blob from <script id="store-cache">
type storeCacheData struct {
	Profiles map[string]storeCacheProfileWrapper `json:"profiles"`
	GIFs     storeCacheGIFs                      `json:"gifs"`
}

type storeCacheProfileWrapper struct {
	User storeCacheProfile `json:"user"`
}

type storeCacheProfile struct {
	Username      string                  `json:"username"`
	PartnerName   string                  `json:"partnername"`
	Tagline       string                  `json:"tagline"`
	UserType      string                  `json:"usertype"`
	Avatars       map[string]string       `json:"avatars"`
	PartnerBanner map[string]string       `json:"partnerbanner"`
	PartnerLinks  []storeCachePartnerLink `json:"partnerlinks"`
}

type storeCachePartnerLink struct {
	URL     string `json:"url"`
	Tooltip string `json:"tooltip"`
	Icon    string `json:"icon"`
}

type storeCacheGIFs struct {
	SearchByUsername map[string]storeCacheGIFSet `json:"searchByUsername"`
}

type storeCacheGIFSet struct {
	Results []storeCacheGIF `json:"results"`
}

type storeCacheGIF struct {
	ID           string                     `json:"id"`
	Title        string                     `json:"title"`
	H1Title      string                     `json:"h1_title"`
	ContentDesc  string                     `json:"content_description"`
	ItemURL      string                     `json:"itemurl"`
	MediaFormats map[string]storeCacheMedia `json:"media_formats"`
}

type storeCacheMedia struct {
	URL string `json:"url"`
}

// GetProfile scrapes a Tenor user profile page for user info and GIFs
func GetProfile(path string) (*ProfilePage, error) {
	fullURL := TenorBaseURL + path
	doc, err := FetchDocument(fullURL)
	if err != nil {
		return nil, err
	}

	profile := &ProfilePage{}

	// Try to extract structured data from the store-cache JSON blob first
	if scriptEl := doc.Find("script#store-cache"); scriptEl.Length() > 0 {
		jsonText := scriptEl.Text()
		if parsed, ok := parseStoreCache(jsonText, path); ok {
			profile = parsed
		}
	}

	// Fallback / supplement with DOM scraping if the JSON blob didn't provide data
	if profile.Username == "" {
		// Extract username from the profile header
		profile.Username = strings.TrimSpace(doc.Find(".ProfilePageHeader h1").First().Text())
		if profile.Username == "" {
			profile.Username = strings.TrimSpace(doc.Find("h1.partnername").First().Text())
		}
	}

	if profile.Tagline == "" {
		profile.Tagline = strings.TrimSpace(doc.Find(".ProfilePageHeader .tagline").First().Text())
	}

	if profile.AvatarURL == "" {
		doc.Find(".ProfilePageHeader .ProfileImage").Each(func(i int, s *goquery.Selection) {
			style, ok := s.Attr("style")
			if ok && strings.Contains(style, "background-image") {
				profile.AvatarURL = extractURLFromStyle(style)
			}
		})
	}

	if profile.BannerURL == "" {
		doc.Find(".ProfilePageHeader .banner-image").Each(func(i int, s *goquery.Selection) {
			style, ok := s.Attr("style")
			if ok && strings.Contains(style, "background-image") {
				profile.BannerURL = extractURLFromStyle(style)
			}
		})
	}

	// Determine user type from path or page class if not set
	if profile.UserType == "" {
		if strings.Contains(path, "/official/") {
			profile.UserType = "partner"
		} else {
			profile.UserType = "user"
		}
		if doc.Find(".BrandedPartnerPage").Length() > 0 {
			profile.UserType = "partner"
		}
	}

	// Fallback: scrape GIFs from the DOM if the JSON blob didn't provide them
	if len(profile.GIFs) == 0 {
		doc.Find("figure a").Each(func(i int, s *goquery.Selection) {
			href, hrefExists := s.Attr("href")
			if !hrefExists {
				return
			}
			img := s.Find("img").First()
			alt, _ := img.Attr("alt")
			src, srcExists := img.Attr("src")
			if !srcExists {
				return
			}
			if strings.HasPrefix(href, "/gif-maker") {
				return
			}
			profile.GIFs = append(profile.GIFs, SearchResult{
				URL:      href,
				ImageURL: src,
				Alt:      alt,
			})
		})
	}

	// If we still don't have a username, try to extract from the path
	if profile.Username == "" {
		parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
		if len(parts) > 0 {
			profile.Username = parts[len(parts)-1]
		}
	}

	return profile, nil
}

// parseStoreCache attempts to parse the store-cache JSON and extract profile data
func parseStoreCache(jsonText string, path string) (*ProfilePage, bool) {
	var data storeCacheData
	if err := json.Unmarshal([]byte(jsonText), &data); err != nil {
		return nil, false
	}

	// Find the profile entry -- there's usually only one
	var prof storeCacheProfile
	found := false
	for _, wrapper := range data.Profiles {
		if wrapper.User.Username != "" {
			prof = wrapper.User
			found = true
			break
		}
	}
	if !found {
		return nil, false
	}

	profile := &ProfilePage{
		Username:    prof.Username,
		DisplayName: prof.PartnerName,
		Tagline:     prof.Tagline,
		UserType:    prof.UserType,
	}

	// Pick the largest available avatar
	for _, size := range []string{"256", "128", "75", "32"} {
		if u, ok := prof.Avatars[size]; ok && u != "" {
			profile.AvatarURL = u
			break
		}
	}

	// Pick the largest available banner
	for _, size := range []string{"1110", "910", "690"} {
		if u, ok := prof.PartnerBanner[size]; ok && u != "" {
			profile.BannerURL = u
			break
		}
	}

	// Extract social links
	for _, link := range prof.PartnerLinks {
		if link.URL != "" {
			label := link.Tooltip
			if label == "" {
				label = link.Icon
			}
			profile.SocialLinks = append(profile.SocialLinks, SocialLink{
				URL:     link.URL,
				Tooltip: label,
			})
		}
	}

	// Extract GIFs from the gifs.searchByUsername section
	username := prof.Username
	gifKeys := []string{
		username + "-gifs-profile-public",
		username + "-GIFs-profile-public",
	}

	for _, key := range gifKeys {
		if gifSet, ok := data.GIFs.SearchByUsername[key]; ok {
			for _, g := range gifSet.Results {
				imageURL := ""
				// Prefer tinygif for thumbnails
				if m, ok := g.MediaFormats["tinygif"]; ok && m.URL != "" {
					imageURL = m.URL
				} else if m, ok := g.MediaFormats["gif"]; ok && m.URL != "" {
					imageURL = m.URL
				}

				viewPath := ""
				if g.ItemURL != "" {
					viewPath = strings.TrimPrefix(g.ItemURL, TenorBaseURL)
				}

				if viewPath == "" || imageURL == "" {
					continue
				}

				alt := g.ContentDesc
				if alt == "" {
					alt = g.H1Title
				}
				if alt == "" {
					alt = g.Title
				}

				profile.GIFs = append(profile.GIFs, SearchResult{
					URL:      viewPath,
					ImageURL: imageURL,
					Alt:      alt,
				})
			}
			break
		}
	}

	return profile, true
}

// extractURLFromStyle extracts a URL from a CSS background-image style string
func extractURLFromStyle(style string) string {
	start := strings.Index(style, "url(")
	if start == -1 {
		return ""
	}
	style = style[start+4:]
	end := strings.Index(style, ")")
	if end == -1 {
		return ""
	}
	u := style[:end]
	u = strings.Trim(u, "\"")
	u = strings.Trim(u, "'")
	return u
}

// FetchDocument fetches a URL and returns a goquery Document
func FetchDocument(url string) (*goquery.Document, error) {
	res, err := client.Default.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	return goquery.NewDocumentFromReader(res.Body)
}
