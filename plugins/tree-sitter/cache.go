// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	sitter "github.com/smacker/go-tree-sitter"
)

type CacheEntry struct {
	Tree       *sitter.Tree
	Symbols    []Symbol
	Mtime      time.Time
	Size       int64
	LastAccess time.Time
}

type FileCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	maxSize int
	parser  *Parser
	watcher *fsnotify.Watcher

	hits   atomic.Int64
	misses atomic.Int64
}

type CacheStats struct {
	Hits   int64
	Misses int64
}

func NewFileCache(parser *Parser, maxSize int) *FileCache {
	return &FileCache{
		entries: make(map[string]*CacheEntry),
		maxSize: maxSize,
		parser:  parser,
	}
}

func (c *FileCache) GetOrParse(path string) ([]Symbol, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	c.mu.RLock()
	entry, ok := c.entries[path]
	if ok && entry.Mtime.Equal(info.ModTime()) && entry.Size == info.Size() {
		symbols := entry.Symbols
		c.mu.RUnlock()
		c.hits.Add(1)
		c.mu.Lock()
		entry.LastAccess = time.Now()
		c.mu.Unlock()
		return symbols, nil
	}
	c.mu.RUnlock()

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	tree, lang, err := c.parser.Parse(path, content)
	if err != nil {
		return nil, err
	}

	symbols := ExtractSymbols(tree, content, lang)

	c.mu.Lock()
	c.misses.Add(1)

	if len(c.entries) >= c.maxSize {
		c.evictLRU()
	}

	if old, exists := c.entries[path]; exists && old.Tree != nil {
		old.Tree.Close()
	}

	c.entries[path] = &CacheEntry{
		Tree:       tree,
		Symbols:    symbols,
		Mtime:      info.ModTime(),
		Size:       info.Size(),
		LastAccess: time.Now(),
	}
	c.mu.Unlock()

	return symbols, nil
}

func (c *FileCache) Invalidate(path string) {
	c.mu.Lock()
	if entry, ok := c.entries[path]; ok {
		if entry.Tree != nil {
			entry.Tree.Close()
		}
		delete(c.entries, path)
	}
	c.mu.Unlock()
}

func (c *FileCache) Stats() CacheStats {
	return CacheStats{
		Hits:   c.hits.Load(),
		Misses: c.misses.Load(),
	}
}

func (c *FileCache) CacheHitRate() float64 {
	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

func (c *FileCache) evictLRU() {
	var oldestPath string
	var oldestAccess time.Time

	for path, entry := range c.entries {
		if oldestPath == "" || entry.LastAccess.Before(oldestAccess) {
			oldestPath = path
			oldestAccess = entry.LastAccess
		}
	}
	if oldestPath != "" {
		if entry := c.entries[oldestPath]; entry != nil && entry.Tree != nil {
			entry.Tree.Close()
		}
		delete(c.entries, oldestPath)
	}
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".siply": true, "vendor": true, ".cache": true,
}

func (c *FileCache) StartWatcher(root string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
	if walkErr != nil {
		watcher.Close()
		return fmt.Errorf("walk %s: %w", root, walkErr)
	}

	c.watcher = watcher
	go c.watchLoop()
	return nil
}

func (c *FileCache) watchLoop() {
	for {
		select {
		case event, ok := <-c.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() && !skipDirs[filepath.Base(event.Name)] {
					_ = c.watcher.Add(event.Name)
				}
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				if c.parser.SupportsFile(event.Name) {
					slog.Debug("tree-sitter: file changed, invalidating cache", "path", event.Name)
					c.Invalidate(event.Name)
				}
			}
		case err, ok := <-c.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("tree-sitter: watcher error", "err", err)
		}
	}
}

func (c *FileCache) StopWatcher() {
	if c.watcher != nil {
		c.watcher.Close()
	}
	c.mu.Lock()
	for _, entry := range c.entries {
		if entry.Tree != nil {
			entry.Tree.Close()
		}
	}
	c.entries = make(map[string]*CacheEntry)
	c.mu.Unlock()
}
