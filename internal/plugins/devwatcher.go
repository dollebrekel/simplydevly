// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
)

const debounceDelay = 500 * time.Millisecond

var watchedExtensions = map[string]bool{
	".go":   true,
	".yaml": true,
	".lua":  true,
}

// DevWatcher watches a plugin directory for file changes and triggers reloads.
type DevWatcher struct {
	pluginDir  string
	pluginName string
	registry   core.PluginRegistry
	eventBus   core.EventBus
	extMgr     core.ExtensionRegistration
	watcher    *fsnotify.Watcher
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewDevWatcher creates a file watcher for dev-mode plugin hot-reload.
func NewDevWatcher(pluginDir, pluginName string, registry core.PluginRegistry, eventBus core.EventBus, extMgr core.ExtensionRegistration) *DevWatcher {
	return &DevWatcher{
		pluginDir:  pluginDir,
		pluginName: pluginName,
		registry:   registry,
		eventBus:   eventBus,
		extMgr:     extMgr,
	}
}

// Start begins watching the plugin directory for changes.
func (dw *DevWatcher) Start(ctx context.Context) error {
	if dw.watcher != nil {
		return fmt.Errorf("devwatcher: already started")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dw.watcher = watcher

	if err := dw.addDirRecursive(dw.pluginDir); err != nil {
		watcher.Close()
		return err
	}

	watchCtx, cancel := context.WithCancel(ctx)
	dw.cancel = cancel

	dw.wg.Add(1)
	go dw.loop(watchCtx)

	slog.Info("devwatcher: watching for changes", "plugin", dw.pluginName, "dir", dw.pluginDir)
	return nil
}

// Stop stops the file watcher.
func (dw *DevWatcher) Stop() error {
	if dw.cancel != nil {
		dw.cancel()
	}
	dw.wg.Wait()
	if dw.watcher != nil {
		return dw.watcher.Close()
	}
	return nil
}

func (dw *DevWatcher) addDirRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return dw.watcher.Add(path)
		}
		return nil
	})
}

func (dw *DevWatcher) loop(ctx context.Context) {
	defer dw.wg.Done()

	var timer *time.Timer
	var timerC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return

		case event, ok := <-dw.watcher.Events:
			if !ok {
				return
			}

			// Watch newly created subdirectories.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if !strings.HasPrefix(filepath.Base(event.Name), ".") {
						_ = dw.watcher.Add(event.Name)
					}
					continue
				}
			}

			if !dw.isWatchedFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(debounceDelay)
			timerC = timer.C

		case <-timerC:
			timerC = nil
			dw.reload(ctx)

		case err, ok := <-dw.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("devwatcher: error", "error", err)
		}
	}
}

func (dw *DevWatcher) isWatchedFile(path string) bool {
	ext := filepath.Ext(path)
	return watchedExtensions[ext]
}

func (dw *DevWatcher) reload(ctx context.Context) {
	slog.Info("devwatcher: reloading plugin", "plugin", dw.pluginName)

	if dw.extMgr != nil {
		if um, ok := dw.extMgr.(interface{ UnregisterAll(string) }); ok {
			um.UnregisterAll(dw.pluginName)
		}
	}

	if err := dw.registry.Load(ctx, dw.pluginName); err != nil {
		slog.Error("devwatcher: reload failed", "plugin", dw.pluginName, "error", err)
		return
	}

	if dw.eventBus != nil {
		_ = dw.eventBus.Publish(ctx, events.NewPluginReloadedEvent(dw.pluginName))
	}

	slog.Info("devwatcher: plugin reloaded", "plugin", dw.pluginName)
}
