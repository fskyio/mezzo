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
	"strings"
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
		"MEZZO_CACHE_MEDIA_DIR",
		"MEZZO_CACHE_MEDIA_MAX_MB",
		"MEZZO_CACHE_MEDIA_MAX_FILE_MB",
		"MEZZO_CACHE_MEDIA_TTL",
		"MEZZO_CACHE_AVATAR_TTL",
	} {
		t.Setenv(key, "")
	}
}

func mustLoad(t *testing.T) *Config {
	t.Helper()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return cfg
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)

	cfg := mustLoad(t)

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
	if cfg.CacheMediaDir != "" {
		t.Fatalf("CacheMediaDir = %q, want empty (disk cache off)", cfg.CacheMediaDir)
	}
}

func TestLoadUsesPrefixedEnvironmentBeforeLegacyFallbacks(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("PORT", "9000")
	t.Setenv("PATCHES_URL", "https://legacy.example/patches")
	t.Setenv("MEZZO_PORT", "7000")
	t.Setenv("MEZZO_PATCHES_URL", "https://mezzo.example/patches")

	cfg := mustLoad(t)

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

	cfg := mustLoad(t)

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
	t.Setenv("MEZZO_CACHE_MEDIA_DIR", "/var/cache/mezzo")
	t.Setenv("MEZZO_CACHE_MEDIA_MAX_MB", "256")
	t.Setenv("MEZZO_CACHE_MEDIA_MAX_FILE_MB", "8")
	t.Setenv("MEZZO_CACHE_MEDIA_TTL", "48h")
	t.Setenv("MEZZO_CACHE_AVATAR_TTL", "6h")

	cfg := mustLoad(t)

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
	if cfg.CacheMediaDir != "/var/cache/mezzo" {
		t.Fatalf("CacheMediaDir = %q, want /var/cache/mezzo", cfg.CacheMediaDir)
	}
	if cfg.CacheMediaMaxMB != 256 {
		t.Fatalf("CacheMediaMaxMB = %d, want 256", cfg.CacheMediaMaxMB)
	}
	if cfg.CacheMediaMaxFileMB != 8 {
		t.Fatalf("CacheMediaMaxFileMB = %d, want 8", cfg.CacheMediaMaxFileMB)
	}
	if cfg.CacheMediaTTL != 48*time.Hour {
		t.Fatalf("CacheMediaTTL = %s, want 48h", cfg.CacheMediaTTL)
	}
	if cfg.CacheAvatarTTL != 6*time.Hour {
		t.Fatalf("CacheAvatarTTL = %s, want 6h", cfg.CacheAvatarTTL)
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	for _, tc := range []struct {
		key, value string
	}{
		{"MEZZO_CACHE_DISABLED", "definitely"},
		{"MEZZO_CACHE_GIF_TTL", "later"},
		{"MEZZO_CACHE_GIF_TTL", "1hour"},
		{"MEZZO_CACHE_GIF_MAX", "many"},
		{"MEZZO_CACHE_MEDIA_MAX_MB", "1.5"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv(tc.key, tc.value)

			cfg, err := Load()
			if err == nil {
				t.Fatalf("Load() = %+v, want error", cfg)
			}
			if cfg != nil {
				t.Fatalf("Load() config = %+v, want nil on error", cfg)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error %q does not name %s", err, tc.key)
			}
		})
	}
}

func TestLoadReportsEveryInvalidVariable(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("MEZZO_CACHE_DISABLED", "definitely")
	t.Setenv("MEZZO_CACHE_GIF_TTL", "later")
	t.Setenv("MEZZO_CACHE_GIF_MAX", "many")
	t.Setenv("MEZZO_CACHE_SEARCH_MAX", "500")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	for _, key := range []string{"MEZZO_CACHE_DISABLED", "MEZZO_CACHE_GIF_TTL", "MEZZO_CACHE_GIF_MAX"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error %q does not name %s", err, key)
		}
	}
	if strings.Contains(err.Error(), "MEZZO_CACHE_SEARCH_MAX") {
		t.Fatalf("error %q names valid variable MEZZO_CACHE_SEARCH_MAX", err)
	}
}
