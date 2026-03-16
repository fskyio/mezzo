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
	Title    string
	ImageURL string
}

// Proxy generates a proxied URL for a given image URL
func Proxy(imageURL string) string {
	return fmt.Sprintf("/proxy.gif?url=%s", url.QueryEscape(imageURL))
}

func isTenorMedia(src string) bool {
	return strings.Contains(src, "media.tenor.com") || strings.Contains(src, "media1.tenor.com")
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

	return &GifPage{
		Title:    title,
		ImageURL: imageURL,
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
