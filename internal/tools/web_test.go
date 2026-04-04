package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeb_BasicFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, userAgent, r.Header.Get("User-Agent"))
		fmt.Fprint(w, "hello from server")
	}))
	defer srv.Close()

	tool := &WebTool{}
	input, _ := json.Marshal(webInput{URL: srv.URL})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello from server", output)
}

func TestWeb_HTMLStripping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><p>Hello</p></body></html>")
	}))
	defer srv.Close()

	tool := &WebTool{}
	input, _ := json.Marshal(webInput{URL: srv.URL})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotContains(t, output, "<")
	assert.Contains(t, output, "Hello")
}

func TestWeb_RedirectFollowing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		fmt.Fprint(w, "final destination")
	}))
	defer srv.Close()

	tool := &WebTool{}
	input, _ := json.Marshal(webInput{URL: srv.URL + "/redirect"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "final destination", output)
}

func TestWeb_BodyTruncation(t *testing.T) {
	largeBody := strings.Repeat("x", maxResponseBody+1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, largeBody)
	}))
	defer srv.Close()

	tool := &WebTool{}
	input, _ := json.Marshal(webInput{URL: srv.URL})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, output, "truncated")
}

func TestWeb_InvalidScheme(t *testing.T) {
	tool := &WebTool{}
	input, _ := json.Marshal(webInput{URL: "ftp://example.com"})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported scheme")
}

func TestWeb_EmptyURL(t *testing.T) {
	tool := &WebTool{}
	input, _ := json.Marshal(webInput{URL: ""})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url is required")
}

func TestWeb_Properties(t *testing.T) {
	tool := &WebTool{}
	assert.Equal(t, "web", tool.Name())
	assert.False(t, tool.Destructive())
}
