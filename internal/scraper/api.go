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
)

const (
	// TenorAPIURL is the base URL for the Tenor v2 API (same endpoint used
	// by Tenor's own website for infinite scroll).
	TenorAPIURL = "https://tenor.googleapis.com/v2"

	// TenorAPIKey is the public API key embedded in Tenor's website.
	TenorAPIKey = "AIzaSyC-P6_qz3FzCoXGLk6tgitZo4jEJ5mLzD8"

	// TenorClientKey identifies the integration.
	TenorClientKey = "tenor_web"

	// SearchLimit is the number of results per page.
	SearchLimit = 30

	// ProfileLimit is the number of uploads per page on a profile, the same
	// as Tenor's website requests.
	ProfileLimit = 50
)

// apiSearchResponse represents the JSON response from the Tenor v2 search API.
type apiSearchResponse struct {
	Results []apiResult `json:"results"`
	Next    string      `json:"next"`
}

// apiResult represents a single result from the Tenor v2 search API.
type apiResult struct {
	ID           string                    `json:"id"`
	Title        string                    `json:"title"`
	ItemURL      string                    `json:"itemurl"`
	URL          string                    `json:"url"`
	MediaFormats map[string]apiMediaFormat `json:"media_formats"`
	ContentDesc  string                    `json:"content_description"`
	Tags         []string                  `json:"tags"`
}

// apiMediaFormat represents a media format in the API response.
type apiMediaFormat struct {
	URL  string `json:"url"`
	Dims []int  `json:"dims"`
	Size int    `json:"size"`
}

// SearchResponse contains search results along with a pagination cursor.
type SearchResponse struct {
	Results []SearchResult
	Next    string
}

// Trend is one of Tenor's trending searches, as shown in the carousel on
// Tenor's homepage.
type Trend struct {
	Term     string // human-readable search term, e.g. "good night"
	ImageURL string // preview GIF for the term; may be empty
}

// apiCategoriesResponse represents the JSON response from the Tenor v2
// categories API.
type apiCategoriesResponse struct {
	Tags []struct {
		SearchTerm string `json:"searchterm"`
		Image      string `json:"image"`
	} `json:"tags"`
}

// APISearch calls the Tenor v2 API to search for GIFs with pagination support.
// The query parameter is the search term (human-readable, e.g. "funny cats").
// The pos parameter is the pagination cursor (empty string for the first page).
func APISearch(query string, pos string) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	return apiList("/search", params, SearchLimit, pos)
}

// APIProfile calls the Tenor v2 API for the uploads of one kind (GIFs,
// stickers or memes) that a user has made public. Tenor's website lists them
// as a search for "@username". It paginates the same way as APISearch.
func APIProfile(username string, kind ProfileKind, pos string) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", "@"+username)
	if filter := kind.searchFilter(); filter != "" {
		params.Set("searchfilter", filter)
	}
	return apiList("/search", params, ProfileLimit, pos)
}

// APIFeatured calls the Tenor v2 API for the featured GIFs shown on Tenor's
// homepage. It paginates the same way as APISearch.
func APIFeatured(pos string) (*SearchResponse, error) {
	return apiList("/featured", url.Values{}, SearchLimit, pos)
}

// APIRelated calls the Tenor v2 API for the GIFs related to the GIF with the
// given ID, which Tenor lists below it on its view page. It paginates the same
// way as APISearch. The first page starts with the GIF itself and comes with
// the view page (GifPage.Related), so callers only need the later ones.
func APIRelated(id string, pos string) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("id", id)
	return apiList("/related", params, SearchLimit, pos)
}

// APITrending calls the Tenor v2 API for the trending searches shown on
// Tenor's homepage.
func APITrending() ([]Trend, error) {
	params := url.Values{}
	params.Set("type", "trending")

	var apiResp apiCategoriesResponse
	if err := apiGet("/categories", params, &apiResp); err != nil {
		return nil, err
	}

	var trends []Trend
	for _, tag := range apiResp.Tags {
		if tag.SearchTerm == "" {
			continue
		}
		trends = append(trends, Trend{Term: tag.SearchTerm, ImageURL: tag.Image})
	}
	return trends, nil
}

// apiList calls a paginated Tenor v2 endpoint that returns GIF results
// ("search", "featured" and "related") and converts them to SearchResults.
func apiList(endpoint string, params url.Values, limit int, pos string) (*SearchResponse, error) {
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("media_filter", apiMediaFilter)

	if pos != "" {
		params.Set("pos", pos)
	}

	var apiResp apiSearchResponse
	if err := apiGet(endpoint, params, &apiResp); err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, r := range apiResp.Results {
		// Extract the preview image URL from tinygif format
		imageURL := ""
		width := 0
		height := 0
		if tinygif, ok := r.MediaFormats["tinygif"]; ok {
			imageURL = tinygif.URL
			if len(tinygif.Dims) >= 2 {
				width = tinygif.Dims[0]
				height = tinygif.Dims[1]
			}
		}

		// Build the view path from itemurl
		viewPath := ""
		if r.ItemURL != "" {
			viewPath = strings.TrimPrefix(r.ItemURL, TenorBaseURL)
		}

		if viewPath == "" || imageURL == "" {
			continue
		}

		results = append(results, SearchResult{
			URL:      viewPath,
			ImageURL: imageURL,
			Alt:      r.ContentDesc,
			Width:    width,
			Height:   height,
			Tags:     r.Tags,
			Formats:  formatsOf(func(key string) string { return r.MediaFormats[key].URL }),
		})
	}

	return &SearchResponse{
		Results: results,
		Next:    apiResp.Next,
	}, nil
}

// apiGet calls a Tenor v2 endpoint with the credentials every request needs
// and decodes the JSON response into out.
func apiGet(endpoint string, params url.Values, out any) error {
	params.Set("key", TenorAPIKey)
	params.Set("client_key", TenorClientKey)
	params.Set("contentfilter", "low")

	resp, err := client.Default.Get(TenorAPIURL + endpoint + "?" + params.Encode())
	if err != nil {
		return fmt.Errorf("tenor API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("tenor API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode tenor API response: %w", err)
	}
	return nil
}
