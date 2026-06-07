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
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"PORT",
		"PATCHES_URL",
		"MEZZO_PORT",
		"MEZZO_PATCHES_URL",
		"MEZZO_CACHE_DISABLED",
		"MEZZO_CACHE_GIF_TTL",
		"MEZZO_CACHE_GIF_MAX",
		"MEZZO_CACHE_SEARCH_TTL",
		"MEZZO_CACHE_SEARCH_MAX",
		"MEZZO_CACHE_PROFILE_TTL",
		"MEZZO_CACHE_PROFILE_MAX",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)

	cfg := Load()

	if cfg.Port != "8006" {
		t.Fatalf("Port = %q, want %q", cfg.Port, "8006")
	}
	if cfg.PatchesURL != "" {
		t.Fatalf("PatchesURL = %q, want empty", cfg.PatchesURL)
	}
	if cfg.CacheDisabled {
		t.Fatal("CacheDisabled = true, want false")
	}
	if cfg.CacheGifTTL != time.Hour {
		t.Fatalf("CacheGifTTL = %s, want %s", cfg.CacheGifTTL, time.Hour)
	}
	if cfg.CacheGifMax != 1000 {
		t.Fatalf("CacheGifMax = %d, want 1000", cfg.CacheGifMax)
	}
	if cfg.CacheSearchTTL != 10*time.Minute {
		t.Fatalf("CacheSearchTTL = %s, want %s", cfg.CacheSearchTTL, 10*time.Minute)
	}
	if cfg.CacheSearchMax != 500 {
		t.Fatalf("CacheSearchMax = %d, want 500", cfg.CacheSearchMax)
	}
	if cfg.CacheProfileTTL != 30*time.Minute {
		t.Fatalf("CacheProfileTTL = %s, want %s", cfg.CacheProfileTTL, 30*time.Minute)
	}
	if cfg.CacheProfileMax != 200 {
		t.Fatalf("CacheProfileMax = %d, want 200", cfg.CacheProfileMax)
	}
}

func TestLoadUsesPrefixedEnvironmentBeforeLegacyFallbacks(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("PORT", "9000")
	t.Setenv("PATCHES_URL", "https://legacy.example/patches")
	t.Setenv("MEZZO_PORT", "7000")
	t.Setenv("MEZZO_PATCHES_URL", "https://mezzo.example/patches")

	cfg := Load()

	if cfg.Port != "7000" {
		t.Fatalf("Port = %q, want prefixed value", cfg.Port)
	}
	if cfg.PatchesURL != "https://mezzo.example/patches" {
		t.Fatalf("PatchesURL = %q, want prefixed value", cfg.PatchesURL)
	}
}

func TestLoadUsesLegacyFallbacks(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("PORT", "9000")
	t.Setenv("PATCHES_URL", "https://legacy.example/patches")

	cfg := Load()

	if cfg.Port != "9000" {
		t.Fatalf("Port = %q, want legacy value", cfg.Port)
	}
	if cfg.PatchesURL != "https://legacy.example/patches" {
		t.Fatalf("PatchesURL = %q, want legacy value", cfg.PatchesURL)
	}
}

func TestLoadParsesCacheEnvironment(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("MEZZO_CACHE_DISABLED", "true")
	t.Setenv("MEZZO_CACHE_GIF_TTL", "2h")
	t.Setenv("MEZZO_CACHE_GIF_MAX", "123")
	t.Setenv("MEZZO_CACHE_SEARCH_TTL", "15m")
	t.Setenv("MEZZO_CACHE_SEARCH_MAX", "45")
	t.Setenv("MEZZO_CACHE_PROFILE_TTL", "90m")
	t.Setenv("MEZZO_CACHE_PROFILE_MAX", "67")

	cfg := Load()

	if !cfg.CacheDisabled {
		t.Fatal("CacheDisabled = false, want true")
	}
	if cfg.CacheGifTTL != 2*time.Hour {
		t.Fatalf("CacheGifTTL = %s, want 2h", cfg.CacheGifTTL)
	}
	if cfg.CacheGifMax != 123 {
		t.Fatalf("CacheGifMax = %d, want 123", cfg.CacheGifMax)
	}
	if cfg.CacheSearchTTL != 15*time.Minute {
		t.Fatalf("CacheSearchTTL = %s, want 15m", cfg.CacheSearchTTL)
	}
	if cfg.CacheSearchMax != 45 {
		t.Fatalf("CacheSearchMax = %d, want 45", cfg.CacheSearchMax)
	}
	if cfg.CacheProfileTTL != 90*time.Minute {
		t.Fatalf("CacheProfileTTL = %s, want 90m", cfg.CacheProfileTTL)
	}
	if cfg.CacheProfileMax != 67 {
		t.Fatalf("CacheProfileMax = %d, want 67", cfg.CacheProfileMax)
	}
}

func TestLoadFallsBackForInvalidCacheEnvironment(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("MEZZO_CACHE_DISABLED", "definitely")
	t.Setenv("MEZZO_CACHE_GIF_TTL", "later")
	t.Setenv("MEZZO_CACHE_GIF_MAX", "many")

	cfg := Load()

	if cfg.CacheDisabled {
		t.Fatal("CacheDisabled = true, want default false")
	}
	if cfg.CacheGifTTL != time.Hour {
		t.Fatalf("CacheGifTTL = %s, want default 1h", cfg.CacheGifTTL)
	}
	if cfg.CacheGifMax != 1000 {
		t.Fatalf("CacheGifMax = %d, want default 1000", cfg.CacheGifMax)
	}
}
