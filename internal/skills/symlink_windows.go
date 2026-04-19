// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

//go:build windows

package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

func readFileNoFollow(path string, maxSize int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("skills: file %s exceeds %d bytes", filepath.Base(path), maxSize)
	}
	return data, nil
}
