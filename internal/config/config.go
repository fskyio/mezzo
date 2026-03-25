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

package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for Mezzo.
type Config struct {
	Port       string
	PatchesURL string

	// Cache settings
	CacheDisabled   bool
	CacheGifTTL     time.Duration
	CacheGifMax     int
	CacheSearchTTL  time.Duration
	CacheSearchMax  int
	CacheProfileTTL time.Duration
	CacheProfileMax int
}

// Load reads configuration from environment variables with sensible defaults.
// For backward compatibility, unprefixed PORT and PATCHES_URL are accepted as
// fallbacks when their MEZZO_ prefixed counterparts are not set.
func Load() *Config {
	return &Config{
		Port:            envOrFallback("MEZZO_PORT", "PORT", "8006"),
		PatchesURL:      envOrFallback("MEZZO_PATCHES_URL", "PATCHES_URL", ""),
		CacheDisabled:   envBool("MEZZO_CACHE_DISABLED", false),
		CacheGifTTL:     envDuration("MEZZO_CACHE_GIF_TTL", 1*time.Hour),
		CacheGifMax:     envInt("MEZZO_CACHE_GIF_MAX", 1000),
		CacheSearchTTL:  envDuration("MEZZO_CACHE_SEARCH_TTL", 10*time.Minute),
		CacheSearchMax:  envInt("MEZZO_CACHE_SEARCH_MAX", 500),
		CacheProfileTTL: envDuration("MEZZO_CACHE_PROFILE_TTL", 30*time.Minute),
		CacheProfileMax: envInt("MEZZO_CACHE_PROFILE_MAX", 200),
	}
}

// envOrFallback returns the value of the primary env var, falling back to the
// legacy env var, and finally to the default value.
func envOrFallback(primary, legacy, defaultVal string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if v := os.Getenv(legacy); v != "" {
		return v
	}
	return defaultVal
}

func envBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}
	return d
}

func envInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
