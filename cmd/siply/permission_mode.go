// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newYoloCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "yolo",
		Short: "Show how to enable YOLO permissions",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "Use /yolo inside siply tui, or run one-shot tasks with: siply run --yolo --task \"...\"")
		},
	}
}
