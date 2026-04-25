// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

type Cache struct {
	mu       sync.RWMutex
	entries  map[string]string
	order    []string
	maxSize  int
}

func NewCache(maxSize int) *Cache {
	return &Cache{
		entries: make(map[string]string),
		maxSize: maxSize,
	}
}

func (c *Cache) Get(hash string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.entries[hash]
	if ok {
		for i, h := range c.order {
			if h == hash {
				c.order = append(c.order[:i], c.order[i+1:]...)
				c.order = append(c.order, hash)
				break
			}
		}
	}
	return val, ok
}

func (c *Cache) Put(hash, distillate string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[hash]; exists {
		return
	}

	if len(c.entries) >= c.maxSize {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}

	c.entries[hash] = distillate
	c.order = append(c.order, hash)
}

func hashTurns(msgs []Message) string {
	h := sha256.New()
	for _, m := range msgs {
		h.Write([]byte(m.Role))
		h.Write([]byte{0})
		h.Write([]byte(m.Content))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
