// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Distillate struct {
	SessionID  string           `json:"session_id"`
	Workspace  string           `json:"workspace"`
	Timestamp  time.Time        `json:"timestamp"`
	Model      string           `json:"model"`
	TokenCount int              `json:"token_count"`
	Content    DistillateContent `json:"content"`
}

type DistillateContent struct {
	KeyDecisions []string  `json:"key_decisions"`
	ActiveFiles  []string  `json:"active_files"`
	CurrentTask  string    `json:"current_task"`
	Constraints  []string  `json:"constraints"`
	Patterns     []Pattern `json:"patterns"`
}

type Pattern struct {
	Pattern    string `json:"pattern"`
	Confidence string `json:"confidence"`
}

type DistillateMeta struct {
	SessionID  string    `json:"session_id"`
	Timestamp  time.Time `json:"timestamp"`
	TokenCount int       `json:"token_count"`
	Path       string    `json:"path"`
}

type DistillateStore struct {
	baseDir string
}

func NewDistillateStore(baseDir string) *DistillateStore {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.TempDir()
		}
		baseDir = filepath.Join(home, ".siply", "sessions")
	}
	return &DistillateStore{baseDir: baseDir}
}

func (s *DistillateStore) workspaceDir(workspacePath string) string {
	return filepath.Join(s.baseDir, workspaceHash(workspacePath))
}

func validateSessionID(id string) error {
	if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid session ID: contains path separator or traversal")
	}
	return nil
}

func (s *DistillateStore) Save(sessionID, workspacePath string, distillate *Distillate) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	dir := s.workspaceDir(workspacePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}

	data, err := json.MarshalIndent(distillate, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal distillate: %w", err)
	}

	target := filepath.Join(dir, sessionID+"-distillate.json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func (s *DistillateStore) LoadLatest(workspacePath string) (*Distillate, error) {
	dir := s.workspaceDir(workspacePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workspace dir: %w", err)
	}

	var latest *Distillate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "-distillate.json") {
			continue
		}
		d, loadErr := s.loadFile(filepath.Join(dir, e.Name()))
		if loadErr != nil {
			continue
		}
		if latest == nil || d.Timestamp.After(latest.Timestamp) {
			latest = d
		}
	}

	return latest, nil
}

func (s *DistillateStore) Load(workspacePath, sessionID string) (*Distillate, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
	path := filepath.Join(s.workspaceDir(workspacePath), sessionID+"-distillate.json")
	return s.loadFile(path)
}

func (s *DistillateStore) ListAll(workspacePath string) ([]*DistillateMeta, error) {
	dir := s.workspaceDir(workspacePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workspace dir: %w", err)
	}

	var metas []*DistillateMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "-distillate.json") {
			continue
		}
		d, loadErr := s.loadFile(filepath.Join(dir, e.Name()))
		if loadErr != nil {
			continue
		}
		metas = append(metas, &DistillateMeta{
			SessionID:  d.SessionID,
			Timestamp:  d.Timestamp,
			TokenCount: d.TokenCount,
			Path:       filepath.Join(dir, e.Name()),
		})
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Timestamp.Before(metas[j].Timestamp)
	})

	return metas, nil
}

func (s *DistillateStore) Clear(workspacePath string) error {
	dir := s.workspaceDir(workspacePath)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clear workspace distillates: %w", err)
	}
	return nil
}

func (s *DistillateStore) SaveConsolidated(workspacePath string, distillate *Distillate, replacedIDs []string) error {
	dir := s.workspaceDir(workspacePath)

	data, err := json.MarshalIndent(distillate, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal consolidated distillate: %w", err)
	}

	target := filepath.Join(dir, "consolidated-distillate.json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}

	for _, id := range replacedIDs {
		path := filepath.Join(dir, id+"-distillate.json")
		os.Remove(path)
	}

	return nil
}

func (s *DistillateStore) Count(workspacePath string) int {
	dir := s.workspaceDir(workspacePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "-distillate.json") {
			count++
		}
	}
	return count
}

func (s *DistillateStore) LoadOldest(workspacePath string, n int) ([]*Distillate, error) {
	if n <= 0 {
		return nil, nil
	}
	metas, err := s.ListAll(workspacePath)
	if err != nil {
		return nil, err
	}
	if n > len(metas) {
		n = len(metas)
	}
	var distillates []*Distillate
	for _, m := range metas[:n] {
		d, loadErr := s.loadFile(m.Path)
		if loadErr != nil {
			continue
		}
		distillates = append(distillates, d)
	}
	return distillates, nil
}

func (s *DistillateStore) loadFile(path string) (*Distillate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read distillate: %w", err)
	}
	var d Distillate
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("unmarshal distillate: %w", err)
	}
	return &d, nil
}

func workspaceHash(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%x", h[:6])
}
