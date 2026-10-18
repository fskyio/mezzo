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

// Package mediacache is an on-disk cache for proxied media.
//
// Files live at <dir>/<kind>/<first two key characters>/<key><extension>. The
// extension encodes the content type, so the layout alone is enough to rebuild
// the index after a restart, and a file's modification time is when it was
// fetched. Entries expire after the TTL of their kind, and the least recently
// used entries are evicted once the total size exceeds the budget.
package mediacache

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const sweepInterval = time.Minute

// extensions maps the content types worth storing to the file extension that
// records them. Anything else, SVG in particular, is never stored.
var extensions = map[string]string{
	"image/gif":  ".gif",
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/avif": ".avif",
	"video/mp4":  ".mp4",
	"video/webm": ".webm",
}

var (
	// ErrNotCacheable is returned by Put for a content type or size that Accepts rejects.
	ErrNotCacheable = errors.New("mediacache: response is not cacheable")
	// ErrIncomplete is returned by Put when the body is not exactly the announced size.
	ErrIncomplete = errors.New("mediacache: body does not match the announced size")
)

// Options configures a Store.
type Options struct {
	// MaxBytes is the total size of all stored files.
	MaxBytes int64
	// MaxFileBytes is the largest single file that is stored. It is capped at MaxBytes.
	MaxFileBytes int64
	// TTLs holds the lifetime of an entry for each kind. A Store only
	// accepts the kinds listed here.
	TTLs map[string]time.Duration
}

// Info describes a stored file.
type Info struct {
	ContentType string
	ModTime     time.Time // when the file was fetched
	Size        int64
}

type entry struct {
	kind, key string
	ext       string
	size      int64
	modTime   time.Time
	elem      *list.Element
}

func (e *entry) id() string { return e.kind + "/" + e.key }

// Store is an on-disk cache. It is safe for concurrent use.
type Store struct {
	dir  string
	opts Options
	now  func() time.Time

	mu      sync.Mutex
	entries map[string]*entry
	lru     *list.List // *entry, most recently used first
	used    int64

	locks keyLocks

	stop     chan struct{}
	stopOnce sync.Once
}

// Key returns the cache key for s, the host and path of a media URL.
func Key(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Open opens the cache in dir, creating it if needed and indexing the files
// that are already there. Use a directory that is only used for this cache.
func Open(dir string, opts Options) (*Store, error) {
	if opts.MaxBytes <= 0 {
		return nil, errors.New("mediacache: MaxBytes must be positive")
	}
	if opts.MaxFileBytes <= 0 || opts.MaxFileBytes > opts.MaxBytes {
		opts.MaxFileBytes = opts.MaxBytes
	}
	if len(opts.TTLs) == 0 {
		return nil, errors.New("mediacache: no kinds configured")
	}
	for kind, ttl := range opts.TTLs {
		if kind == "" || strings.ContainsAny(kind, `/\`) || kind == "." || kind == ".." {
			return nil, fmt.Errorf("mediacache: invalid kind %q", kind)
		}
		if ttl <= 0 {
			return nil, fmt.Errorf("mediacache: TTL for %q must be positive", kind)
		}
	}

	s := &Store{
		dir:     dir,
		opts:    opts,
		now:     time.Now,
		entries: make(map[string]*entry),
		lru:     list.New(),
		locks:   keyLocks{m: make(map[string]*keyLock)},
		stop:    make(chan struct{}),
	}
	for kind := range opts.TTLs {
		if err := os.MkdirAll(filepath.Join(dir, kind), 0o700); err != nil {
			return nil, fmt.Errorf("mediacache: %w", err)
		}
	}
	if err := s.scan(); err != nil {
		return nil, fmt.Errorf("mediacache: scanning %s: %w", dir, err)
	}

	s.mu.Lock()
	s.sweepLocked()
	s.mu.Unlock()
	go s.janitor()
	return s, nil
}

// Close stops the background sweeper. Stored files stay on disk.
func (s *Store) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// Accepts reports whether a response with this content type and size can be
// stored, and returns the normalized content type to pass to Put. An unknown
// size (-1) is never accepted, because a truncated body would go unnoticed.
func (s *Store) Accepts(contentType string, size int64) (string, bool) {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", false
	}
	if _, ok := extensions[mt]; !ok || size <= 0 || size > s.opts.MaxFileBytes {
		return "", false
	}
	return mt, true
}

// Get opens the stored file for key. The caller must close the file. A
// successful Get marks the entry as recently used.
func (s *Store) Get(kind, key string) (*os.File, Info, bool) {
	if !s.valid(kind, key) {
		return nil, Info{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	e := s.entries[kind+"/"+key]
	if e == nil {
		return nil, Info{}, false
	}
	if s.expired(e, s.now()) {
		s.removeLocked(e)
		return nil, Info{}, false
	}
	f, err := os.Open(s.path(e))
	if err != nil {
		slog.Warn("media cache file is unreadable, dropping it", "error", err)
		s.removeLocked(e)
		return nil, Info{}, false
	}
	s.lru.MoveToFront(e.elem)
	return f, s.info(e), true
}

// Put reads exactly size bytes from body and stores them under key, replacing
// any existing entry, then opens the stored file. The caller must close the
// file. Nothing is stored if the body is shorter or longer than size, or on
// any read or write error.
func (s *Store) Put(kind, key, contentType string, size int64, body io.Reader) (*os.File, Info, error) {
	if !s.valid(kind, key) {
		return nil, Info{}, ErrNotCacheable
	}
	mt, ok := s.Accepts(contentType, size)
	if !ok {
		return nil, Info{}, ErrNotCacheable
	}
	e := &entry{kind: kind, key: key, ext: extensions[mt], size: size}

	shard := filepath.Dir(s.path(e))
	if err := os.MkdirAll(shard, 0o700); err != nil {
		return nil, Info{}, err
	}
	tmp, err := os.CreateTemp(shard, "fill-*.tmp")
	if err != nil {
		return nil, Info{}, err
	}
	n, err := io.Copy(tmp, io.LimitReader(body, size+1))
	if err == nil && n != size {
		err = ErrIncomplete
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return nil, Info{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if old := s.entries[e.id()]; old != nil {
		s.removeLocked(old)
	}
	if err := os.Rename(tmp.Name(), s.path(e)); err != nil {
		os.Remove(tmp.Name())
		return nil, Info{}, err
	}
	e.modTime = s.now()
	e.elem = s.lru.PushFront(e)
	s.entries[e.id()] = e
	s.used += e.size
	s.evictLocked(e)

	f, err := os.Open(s.path(e))
	if err != nil {
		s.removeLocked(e)
		return nil, Info{}, err
	}
	return f, s.info(e), nil
}

// Lock serializes work on key, so that concurrent requests for the same
// uncached file result in a single fetch. It returns when the lock is held
// or ctx is done.
func (s *Store) Lock(ctx context.Context, kind, key string) (unlock func(), err error) {
	return s.locks.lock(ctx, kind+"/"+key)
}

func (s *Store) valid(kind, key string) bool {
	if _, ok := s.opts.TTLs[kind]; !ok || len(key) != sha256.Size*2 {
		return false
	}
	for _, c := range key {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

func (s *Store) path(e *entry) string {
	return filepath.Join(s.dir, e.kind, e.key[:2], e.key+e.ext)
}

func (s *Store) info(e *entry) Info {
	return Info{ContentType: contentType(e.ext), ModTime: e.modTime, Size: e.size}
}

func (s *Store) expired(e *entry, now time.Time) bool {
	return now.Sub(e.modTime) > s.opts.TTLs[e.kind]
}

func contentType(ext string) string {
	for mt, x := range extensions {
		if x == ext {
			return mt
		}
	}
	return ""
}

// removeLocked drops e from the index and deletes its file. If the file
// cannot be deleted (Windows refuses while a reader has it open) it stays on
// disk until the next start.
func (s *Store) removeLocked(e *entry) {
	delete(s.entries, e.id())
	s.lru.Remove(e.elem)
	s.used -= e.size
	if err := os.Remove(s.path(e)); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("failed to delete media cache file", "error", err)
	}
}

// evictLocked removes least recently used entries until the cache fits the
// budget. keep is never evicted.
func (s *Store) evictLocked(keep *entry) {
	for s.used > s.opts.MaxBytes {
		back := s.lru.Back()
		if back == nil || back.Value.(*entry) == keep {
			return
		}
		s.removeLocked(back.Value.(*entry))
	}
}

// sweepLocked removes expired entries, then evicts to fit the budget.
func (s *Store) sweepLocked() {
	now := s.now()
	for _, e := range s.entries {
		if s.expired(e, now) {
			s.removeLocked(e)
		}
	}
	s.evictLocked(nil)
}

func (s *Store) janitor() {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.mu.Lock()
			s.sweepLocked()
			s.mu.Unlock()
		case <-s.stop:
			return
		}
	}
}

// scan indexes the files already in the cache, oldest first so that they are
// evicted first. It deletes leftovers of interrupted fills. Files it does not
// recognize are left alone.
func (s *Store) scan() error {
	var found []*entry
	for kind := range s.opts.TTLs {
		shards, err := os.ReadDir(filepath.Join(s.dir, kind))
		if err != nil {
			return err
		}
		for _, shard := range shards {
			if !shard.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(s.dir, kind, shard.Name()))
			if err != nil {
				return err
			}
			for _, f := range files {
				p := filepath.Join(s.dir, kind, shard.Name(), f.Name())
				if strings.HasPrefix(f.Name(), "fill-") && strings.HasSuffix(f.Name(), ".tmp") {
					os.Remove(p)
					continue
				}
				ext := filepath.Ext(f.Name())
				key := strings.TrimSuffix(f.Name(), ext)
				if contentType(ext) == "" || !s.valid(kind, key) || key[:2] != shard.Name() {
					continue
				}
				fi, err := f.Info()
				if err != nil || !fi.Mode().IsRegular() {
					continue
				}
				found = append(found, &entry{kind: kind, key: key, ext: ext, size: fi.Size(), modTime: fi.ModTime()})
			}
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].modTime.Before(found[j].modTime) })
	for _, e := range found {
		e.elem = s.lru.PushFront(e)
		s.entries[e.id()] = e
		s.used += e.size
	}
	return nil
}

// keyLocks is a set of mutexes by key that can be abandoned when a context is
// done, and that are forgotten once nobody holds or waits for them.
type keyLocks struct {
	mu sync.Mutex
	m  map[string]*keyLock
}

type keyLock struct {
	sem  chan struct{}
	refs int
}

func (k *keyLocks) lock(ctx context.Context, id string) (func(), error) {
	k.mu.Lock()
	l := k.m[id]
	if l == nil {
		l = &keyLock{sem: make(chan struct{}, 1)}
		k.m[id] = l
	}
	l.refs++
	k.mu.Unlock()

	release := func() {
		k.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(k.m, id)
		}
		k.mu.Unlock()
	}

	select {
	case l.sem <- struct{}{}:
		return func() {
			<-l.sem
			release()
		}, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}
