// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package hooks

import "time"

// HookFailedEvent is published to the EventBus when a SkipOnFailure hook
// fails or times out. Subscribers (e.g. StatusCollector, ActivityFeed) can
// use this to show warnings like "distillation offline — full context ($$$)".
type HookFailedEvent struct {
	HookName   string // which hook failed
	Point      string // "PreQuery" or "PreTool"
	Err        string // error message
	Fallback   string // what happens instead (e.g. "continuing with unmodified data")
	CostEffect string // cost impact description (e.g. "potential increased token usage")
	Ts         time.Time
}

func (e *HookFailedEvent) Type() string         { return "hook.failed" }
func (e *HookFailedEvent) Timestamp() time.Time { return e.Ts }
