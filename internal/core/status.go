// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import "time"

// StatusUpdate holds a status report from a subsystem.
type StatusUpdate struct {
	Source    string
	Metrics   map[string]any
	Timestamp time.Time
}

// StatusCollector aggregates status updates from subsystems.
type StatusCollector interface {
	Lifecycle
	Publish(update StatusUpdate)
	Subscribe() (updates <-chan StatusUpdate, unsubscribe func())
	Snapshot() map[string]StatusUpdate
}
