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

// APISearch calls the Tenor v2 API to search for GIFs with pagination support.
// The query parameter is the search term (human-readable, e.g. "funny cats").
// The pos parameter is the pagination cursor (empty string for the first page).
func APISearch(query string, pos string) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("key", TenorAPIKey)
	params.Set("client_key", TenorClientKey)
	params.Set("limit", fmt.Sprintf("%d", SearchLimit))
	params.Set("media_filter", "tinygif")
	params.Set("contentfilter", "low")

	if pos != "" {
		params.Set("pos", pos)
	}

	apiURL := TenorAPIURL + "/search?" + params.Encode()

	resp, err := client.Default.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("tenor API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("tenor API returned status %d", resp.StatusCode)
	}

	var apiResp apiSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode tenor API response: %w", err)
	}

	var results []SearchResult
	for _, r := range apiResp.Results {
		// Extract the preview image URL from tinygif format
		imageURL := ""
		if tinygif, ok := r.MediaFormats["tinygif"]; ok {
			imageURL = tinygif.URL
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
		})
	}

	return &SearchResponse{
		Results: results,
		Next:    apiResp.Next,
	}, nil
}
