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

package cache

import (
	"testing"
	"time"
)

func TestCacheSetGetAndLen(t *testing.T) {
	c := New[string](time.Minute, 2)
	defer c.Stop()

	c.Set("first", "value")

	got, ok := c.Get("first")
	if !ok {
		t.Fatal("Get returned ok = false, want true")
	}
	if got != "value" {
		t.Fatalf("Get returned %q, want %q", got, "value")
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1", c.Len())
	}
}

func TestCacheGetMissingKey(t *testing.T) {
	c := New[string](time.Minute, 2)
	defer c.Stop()

	got, ok := c.Get("missing")
	if ok {
		t.Fatal("Get returned ok = true, want false")
	}
	if got != "" {
		t.Fatalf("Get returned %q, want zero value", got)
	}
}

func TestCacheExpiresEntries(t *testing.T) {
	c := New[string](20*time.Millisecond, 2)
	defer c.Stop()

	c.Set("short", "lived")
	time.Sleep(35 * time.Millisecond)

	got, ok := c.Get("short")
	if ok {
		t.Fatalf("Get returned (%q, true), want expired entry", got)
	}
}

func TestCacheEvictsEarliestExpiringEntryAtCapacity(t *testing.T) {
	c := New[string](time.Minute, 2)
	defer c.Stop()

	c.Set("oldest", "first")
	time.Sleep(time.Millisecond)
	c.Set("newer", "second")
	c.Set("newest", "third")

	if _, ok := c.Get("oldest"); ok {
		t.Fatal("oldest key remained in cache after capacity eviction")
	}
	if got, ok := c.Get("newer"); !ok || got != "second" {
		t.Fatalf("newer entry = (%q, %v), want (second, true)", got, ok)
	}
	if got, ok := c.Get("newest"); !ok || got != "third" {
		t.Fatalf("newest entry = (%q, %v), want (third, true)", got, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
}

func TestCacheUpdateDoesNotEvictAtCapacity(t *testing.T) {
	c := New[string](time.Minute, 2)
	defer c.Stop()

	c.Set("first", "old")
	c.Set("second", "kept")
	c.Set("first", "new")

	if got, ok := c.Get("first"); !ok || got != "new" {
		t.Fatalf("first entry = (%q, %v), want (new, true)", got, ok)
	}
	if got, ok := c.Get("second"); !ok || got != "kept" {
		t.Fatalf("second entry = (%q, %v), want (kept, true)", got, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
}
