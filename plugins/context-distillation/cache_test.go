// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import "testing"

func TestCache_PutGet(t *testing.T) {
	c := NewCache(10)
	c.Put("hash1", "distillate1")

	val, ok := c.Get("hash1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "distillate1" {
		t.Errorf("expected distillate1, got %s", val)
	}
}

func TestCache_Miss(t *testing.T) {
	c := NewCache(10)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	c := NewCache(3)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	c.Put("d", "4")

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be evicted (LRU)")
	}
	if _, ok := c.Get("d"); !ok {
		t.Error("expected 'd' to be present")
	}
}

func TestCache_DuplicatePut(t *testing.T) {
	c := NewCache(3)
	c.Put("a", "1")
	c.Put("a", "updated")

	val, ok := c.Get("a")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "1" {
		t.Error("duplicate put should not overwrite existing entry")
	}
}

func TestHashTurns_DeterministicOutput(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}
	h1 := hashTurns(msgs)
	h2 := hashTurns(msgs)

	if h1 != h2 {
		t.Error("hashTurns should be deterministic")
	}
}

func TestHashTurns_DifferentForNewTurn(t *testing.T) {
	msgs1 := []Message{
		{Role: "user", Content: "Hello"},
	}
	msgs2 := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}
	if hashTurns(msgs1) == hashTurns(msgs2) {
		t.Error("different messages should produce different hashes")
	}
}
