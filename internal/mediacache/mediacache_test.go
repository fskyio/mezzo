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

package mediacache

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testOptions(maxBytes int64) Options {
	return Options{
		MaxBytes: maxBytes,
		TTLs:     map[string]time.Duration{"media": time.Hour, "avatar": 10 * time.Minute},
	}
}

func openStore(t *testing.T, dir string, opts Options) *Store {
	t.Helper()
	s, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func put(t *testing.T, s *Store, kind, key, body string) {
	t.Helper()
	f, _, err := s.Put(kind, key, "image/gif", int64(len(body)), strings.NewReader(body))
	if err != nil {
		t.Fatalf("Put(%s, %.8s): %v", kind, key, err)
	}
	f.Close()
}

func read(t *testing.T, s *Store, kind, key string) (string, bool) {
	t.Helper()
	f, _, ok := s.Get(kind, key)
	if !ok {
		return "", false
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// files lists every regular file below dir.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			found = append(found, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestPutThenGet(t *testing.T) {
	s := openStore(t, t.TempDir(), testOptions(1<<20))
	key := Key("media.tenor.com/a/cat.gif")

	f, info, err := s.Put("media", key, "image/gif; charset=binary", 6, strings.NewReader("GIF89a"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if info.ContentType != "image/gif" || info.Size != 6 {
		t.Fatalf("Put info = %+v, want image/gif, 6 bytes", info)
	}

	f, got, ok := s.Get("media", key)
	if !ok {
		t.Fatal("Get missed a stored file")
	}
	defer f.Close()
	if got.ContentType != "image/gif" || got.Size != 6 || got.ModTime.IsZero() {
		t.Fatalf("Get info = %+v, want image/gif, 6 bytes and a time", got)
	}
	if body, _ := io.ReadAll(f); string(body) != "GIF89a" {
		t.Fatalf("stored body = %q, want GIF89a", body)
	}

	if _, ok := read(t, s, "avatar", key); ok {
		t.Fatal("an entry of one kind was found under another")
	}
}

func TestPutRejectsWhatCannotBeStored(t *testing.T) {
	opts := testOptions(1 << 20)
	opts.MaxFileBytes = 8
	s := openStore(t, t.TempDir(), opts)
	key := Key("media.tenor.com/a/cat.gif")

	tests := []struct {
		name        string
		contentType string
		size        int64
		body        string
		wantErr     error
	}{
		{"svg", "image/svg+xml", 6, "<svg/>", ErrNotCacheable},
		{"html", "text/html", 6, "<html>", ErrNotCacheable},
		{"empty", "image/gif", 0, "", ErrNotCacheable},
		{"unknown size", "image/gif", -1, "GIF89a", ErrNotCacheable},
		{"over the file limit", "image/gif", 9, "GIF89a123", ErrNotCacheable},
		{"body shorter than announced", "image/gif", 6, "GIF", ErrIncomplete},
		{"body longer than announced", "image/gif", 3, "GIF89a", ErrIncomplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := s.Put("media", key, tt.contentType, tt.size, strings.NewReader(tt.body))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Put error = %v, want %v", err, tt.wantErr)
			}
			if _, ok := read(t, s, "media", key); ok {
				t.Fatal("a rejected body was stored")
			}
		})
	}

	if left := files(t, s.dir); len(left) != 0 {
		t.Fatalf("rejected puts left files behind: %v", left)
	}
}

func TestPutFailingBodyStoresNothing(t *testing.T) {
	s := openStore(t, t.TempDir(), testOptions(1<<20))
	key := Key("media.tenor.com/a/cat.gif")

	boom := errors.New("connection reset")
	body := io.MultiReader(strings.NewReader("GIF"), &failingReader{boom})
	if _, _, err := s.Put("media", key, "image/gif", 6, body); !errors.Is(err, boom) {
		t.Fatalf("Put error = %v, want %v", err, boom)
	}
	if _, ok := read(t, s, "media", key); ok {
		t.Fatal("a failed download was stored")
	}
	if left := files(t, s.dir); len(left) != 0 {
		t.Fatalf("failed put left files behind: %v", left)
	}
}

type failingReader struct{ err error }

func (r *failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestKeysCannotEscapeTheDirectory(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "cache")
	s := openStore(t, dir, testOptions(1<<20))

	for _, key := range []string{"../../escape", Key("x")[:63], Key("x") + "0", strings.ToUpper(Key("x")), ""} {
		if _, _, err := s.Put("media", key, "image/gif", 6, strings.NewReader("GIF89a")); !errors.Is(err, ErrNotCacheable) {
			t.Errorf("Put with key %q: error = %v, want %v", key, err, ErrNotCacheable)
		}
		if _, ok := read(t, s, "media", key); ok {
			t.Errorf("Get found key %q", key)
		}
	}
	if _, _, err := s.Put("../media", Key("x"), "image/gif", 6, strings.NewReader("GIF89a")); !errors.Is(err, ErrNotCacheable) {
		t.Errorf("Put with an unknown kind: error = %v, want %v", err, ErrNotCacheable)
	}
	if left := files(t, parent); len(left) != 0 {
		t.Fatalf("files were written: %v", left)
	}
}

func TestEntriesExpireAfterTheTTLOfTheirKind(t *testing.T) {
	s := openStore(t, t.TempDir(), testOptions(1<<20)) // media: 1h, avatar: 10m
	now := time.Now()
	s.now = func() time.Time { return now }
	media, avatar := Key("media"), Key("avatar")
	put(t, s, "media", media, "GIF89a")
	put(t, s, "avatar", avatar, "GIF89a")

	now = now.Add(30 * time.Minute)
	if _, ok := read(t, s, "avatar", avatar); ok {
		t.Fatal("avatar is still served after its TTL")
	}
	if _, ok := read(t, s, "media", media); !ok {
		t.Fatal("media expired before its TTL")
	}
	if left := files(t, s.dir); len(left) != 1 {
		t.Fatalf("%d files on disk, want 1 (the expired avatar is deleted)", len(left))
	}

	now = now.Add(time.Hour)
	if _, ok := read(t, s, "media", media); ok {
		t.Fatal("media is still served after its TTL")
	}
	if left := files(t, s.dir); len(left) != 0 {
		t.Fatalf("%d files on disk, want 0", len(left))
	}
}

func TestSweepDeletesExpiredEntriesNobodyAsksFor(t *testing.T) {
	s := openStore(t, t.TempDir(), testOptions(1<<20))
	now := time.Now()
	s.now = func() time.Time { return now }
	put(t, s, "avatar", Key("avatar"), "GIF89a")

	now = now.Add(time.Hour)
	s.mu.Lock()
	s.sweepLocked()
	s.mu.Unlock()

	if left := files(t, s.dir); len(left) != 0 {
		t.Fatalf("expired file survived the sweep: %v", left)
	}
}

func TestEvictsLeastRecentlyUsedOverBudget(t *testing.T) {
	s := openStore(t, t.TempDir(), testOptions(30))
	a, b, c, d := Key("a"), Key("b"), Key("c"), Key("d")
	put(t, s, "media", a, "0123456789")
	put(t, s, "media", b, "0123456789")
	put(t, s, "media", c, "0123456789")

	if _, ok := read(t, s, "media", a); !ok { // a is now newer than b
		t.Fatal("a is missing")
	}
	put(t, s, "media", d, "0123456789")

	for key, want := range map[string]bool{a: true, b: false, c: true, d: true} {
		if _, ok := read(t, s, "media", key); ok != want {
			t.Errorf("entry %.4s stored = %v, want %v", key, ok, want)
		}
	}
	if left := files(t, s.dir); len(left) != 3 {
		t.Fatalf("%d files on disk, want 3", len(left))
	}
}

func TestReplacingAnEntryDoesNotCountTwice(t *testing.T) {
	s := openStore(t, t.TempDir(), testOptions(20))
	a, b := Key("a"), Key("b")
	put(t, s, "media", a, "0123456789")
	put(t, s, "media", b, "0123456789")
	put(t, s, "media", a, "9876543210") // 20 bytes in total, so nothing is evicted

	if body, ok := read(t, s, "media", a); !ok || body != "9876543210" {
		t.Fatalf("a = %q, %v, want the new body", body, ok)
	}
	if _, ok := read(t, s, "media", b); !ok {
		t.Fatal("b was evicted by a replacement that fits")
	}
}

func TestOpenIndexesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, testOptions(1<<20))
	a, b, c := Key("a"), Key("b"), Key("c")
	put(t, s, "media", a, "0123456789")
	put(t, s, "media", b, "0123456789")
	put(t, s, "avatar", c, "0123456789")
	s.Close()

	// Age the files so the order they are evicted in is known
	old := time.Now().Add(-5 * time.Minute)
	for i, key := range []string{a, b} {
		p := filepath.Join(dir, "media", key[:2], key+".gif")
		if err := os.Chtimes(p, old, old.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	// Things Open must not mistake for entries or delete
	stranger := filepath.Join(dir, "media", "README")
	leftover := filepath.Join(dir, "media", a[:2], "fill-123.tmp")
	if err := os.WriteFile(stranger, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte("half a gif"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A smaller budget than what is on disk: the oldest entry goes
	opts := testOptions(25)
	s = openStore(t, dir, opts)
	if _, ok := read(t, s, "media", a); ok {
		t.Error("the oldest entry survived a budget that does not fit")
	}
	for kind, key := range map[string]string{"media": b, "avatar": c} {
		if body, ok := read(t, s, kind, key); !ok || body != "0123456789" {
			t.Errorf("%s entry %.4s = %q, %v, want it indexed from disk", kind, key, body, ok)
		}
	}
	if _, err := os.Stat(stranger); err != nil {
		t.Errorf("Open deleted a file that is not an entry: %v", err)
	}
	if _, err := os.Stat(leftover); err == nil {
		t.Error("Open kept the leftover of an interrupted fill")
	}
}

func TestOpenRejectsBadOptions(t *testing.T) {
	good := testOptions(1 << 20)
	tests := map[string]func(*Options){
		"no budget":       func(o *Options) { o.MaxBytes = 0 },
		"no kinds":        func(o *Options) { o.TTLs = nil },
		"zero TTL":        func(o *Options) { o.TTLs = map[string]time.Duration{"media": 0} },
		"kind with slash": func(o *Options) { o.TTLs = map[string]time.Duration{"a/b": time.Hour} },
		"dot dot kind":    func(o *Options) { o.TTLs = map[string]time.Duration{"..": time.Hour} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			opts := good
			mutate(&opts)
			if s, err := Open(t.TempDir(), opts); err == nil {
				s.Close()
				t.Fatal("Open succeeded")
			}
		})
	}
}

func TestLockSerializesAKeyAndYieldsToTheContext(t *testing.T) {
	s := openStore(t, t.TempDir(), testOptions(1<<20))
	key := Key("a")

	unlock, err := s.Lock(context.Background(), "media", key)
	if err != nil {
		t.Fatal(err)
	}

	// A second holder has to wait for the first
	acquired := make(chan func())
	go func() {
		u, err := s.Lock(context.Background(), "media", key)
		if err != nil {
			t.Error(err)
			return
		}
		acquired <- u
	}()
	select {
	case <-acquired:
		t.Fatal("second Lock returned while the first was held")
	case <-time.After(50 * time.Millisecond):
	}

	// A waiter whose context ends gives up
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.Lock(ctx, "media", key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lock with an expiring context: error = %v, want %v", err, context.DeadlineExceeded)
	}

	// Other keys are not held up
	other, err := s.Lock(context.Background(), "media", Key("b"))
	if err != nil {
		t.Fatal(err)
	}
	other()

	unlock()
	select {
	case u := <-acquired:
		u()
	case <-time.After(time.Second):
		t.Fatal("second Lock did not return after the first was released")
	}

	s.locks.mu.Lock()
	defer s.locks.mu.Unlock()
	if n := len(s.locks.m); n != 0 {
		t.Fatalf("%d locks are still tracked after all were released", n)
	}
}
