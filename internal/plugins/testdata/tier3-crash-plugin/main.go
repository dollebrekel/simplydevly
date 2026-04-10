// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// tier3-crash-plugin crashes (os.Exit(2)) when Execute is called.
// Used to test crash isolation — core must NOT be affected.
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
)

type pluginServer struct {
	siplyv1.UnimplementedSiplyPluginServiceServer
}

func (s *pluginServer) Initialize(_ context.Context, _ *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
	return &siplyv1.InitializeResponse{Success: true}, nil
}

func (s *pluginServer) Execute(_ context.Context, _ *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	os.Exit(2) // Crash!
	return nil, nil
}

func (s *pluginServer) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	go func() { os.Exit(0) }()
	return &siplyv1.ShutdownResponse{}, nil
}

func main() {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}

	srv := grpc.NewServer()
	siplyv1.RegisterSiplyPluginServiceServer(srv, &pluginServer{})

	fmt.Printf("PLUGIN_ADDR=%s\n", lis.Addr().String())

	if err := srv.Serve(lis); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
}
