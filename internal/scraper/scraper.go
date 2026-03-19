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
	"fmt"
	"net/http"
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
			authorURL = TenorBaseURL + href
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
		})
	})

	return results, nil
}

// FetchDocument fetches a URL and returns a goquery Document
func FetchDocument(url string) (*goquery.Document, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	return goquery.NewDocumentFromReader(res.Body)
}
