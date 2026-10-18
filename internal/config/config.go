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
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// Media cache on disk. It is off unless CacheMediaDir is set, and
	// CacheDisabled turns it off as well.
	CacheMediaDir       string
	CacheMediaMaxMB     int
	CacheMediaMaxFileMB int
	CacheMediaTTL       time.Duration
	CacheAvatarTTL      time.Duration
}

// Load reads configuration from environment variables with sensible defaults.
// For backward compatibility, unprefixed PORT and PATCHES_URL are accepted as
// fallbacks when their MEZZO_ prefixed counterparts are not set.
//
// An unset or empty variable yields its default. A variable that is set but
// cannot be parsed is an error, and Load reports every such variable at once.
func Load() (*Config, error) {
	var p parser

	cfg := &Config{
		Port:            envOrFallback("MEZZO_PORT", "PORT", "8006"),
		PatchesURL:      envOrFallback("MEZZO_PATCHES_URL", "PATCHES_URL", ""),
		CacheDisabled:   p.bool("MEZZO_CACHE_DISABLED", false),
		CacheGifTTL:     p.duration("MEZZO_CACHE_GIF_TTL", 1*time.Hour),
		CacheGifMax:     p.int("MEZZO_CACHE_GIF_MAX", 1000),
		CacheSearchTTL:  p.duration("MEZZO_CACHE_SEARCH_TTL", 10*time.Minute),
		CacheSearchMax:  p.int("MEZZO_CACHE_SEARCH_MAX", 500),
		CacheProfileTTL: p.duration("MEZZO_CACHE_PROFILE_TTL", 30*time.Minute),
		CacheProfileMax: p.int("MEZZO_CACHE_PROFILE_MAX", 200),

		CacheMediaDir:       os.Getenv("MEZZO_CACHE_MEDIA_DIR"),
		CacheMediaMaxMB:     p.int("MEZZO_CACHE_MEDIA_MAX_MB", 512),
		CacheMediaMaxFileMB: p.int("MEZZO_CACHE_MEDIA_MAX_FILE_MB", 16),
		CacheMediaTTL:       p.duration("MEZZO_CACHE_MEDIA_TTL", 168*time.Hour),
		CacheAvatarTTL:      p.duration("MEZZO_CACHE_AVATAR_TTL", 24*time.Hour),
	}

	if len(p.errs) > 0 {
		return nil, fmt.Errorf("invalid environment: %s", strings.Join(p.errs, "; "))
	}
	return cfg, nil
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

// parser records every variable that is set but unparsable, so Load can report
// them together instead of one per restart.
type parser struct {
	errs []string
}

func (p *parser) bool(key string, defaultVal bool) bool {
	return parseEnv(p, key, defaultVal, strconv.ParseBool)
}

func (p *parser) duration(key string, defaultVal time.Duration) time.Duration {
	return parseEnv(p, key, defaultVal, time.ParseDuration)
}

func (p *parser) int(key string, defaultVal int) int {
	return parseEnv(p, key, defaultVal, strconv.Atoi)
}

func parseEnv[T any](p *parser, key string, defaultVal T, parse func(string) (T, error)) T {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	parsed, err := parse(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Sprintf("%s: %v", key, err))
		return defaultVal
	}
	return parsed
}
