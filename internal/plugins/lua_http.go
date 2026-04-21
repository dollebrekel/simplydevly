// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const httpMaxResponseBytes = 10 << 20 // 10MB

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// registerHTTPAPI registers the siply.http table on the siply global.
func registerHTTPAPI(L *lua.LState, siply *lua.LTable, plugin *Tier2Plugin) {
	httpTbl := L.NewTable()
	httpTbl.RawSetString("get", L.NewFunction(newSiplyHTTPGet(plugin)))
	httpTbl.RawSetString("post", L.NewFunction(newSiplyHTTPPost(plugin)))
	siply.RawSetString("http", httpTbl)
}

// readHTTPBody reads a response body with a size limit and reports truncation.
func readHTTPBody(resp *http.Response) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpMaxResponseBytes+1))
	if err != nil {
		return nil, false, err
	}
	truncated := len(body) > httpMaxResponseBytes
	if truncated {
		body = body[:httpMaxResponseBytes]
	}
	return body, truncated, nil
}

func buildHTTPResult(L *lua.LState, resp *http.Response) (*lua.LTable, error) {
	body, truncated, err := readHTTPBody(resp)
	if err != nil {
		return nil, err
	}

	result := L.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(string(body)))
	result.RawSetString("truncated", lua.LBool(truncated))
	respHeaders := L.NewTable()
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders.RawSetString(k, lua.LString(v[0]))
		}
	}
	result.RawSetString("headers", respHeaders)
	return result, nil
}

func newSiplyHTTPGet(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		rawURL := L.CheckString(1)
		headersTbl := L.OptTable(2, nil)

		if err := validateHTTPURL(rawURL, plugin.HTTPAllowlist); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		req, err := http.NewRequestWithContext(L.Context(), http.MethodGet, rawURL, nil)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		applyLuaHeaders(req, headersTbl)

		resp, err := httpClient.Do(req)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer resp.Body.Close()

		result, err := buildHTTPResult(L, resp)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(result)
		return 1
	}
}

func newSiplyHTTPPost(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		rawURL := L.CheckString(1)
		bodyStr := L.OptString(2, "")
		headersTbl := L.OptTable(3, nil)

		if err := validateHTTPURL(rawURL, plugin.HTTPAllowlist); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		req, err := http.NewRequestWithContext(L.Context(), http.MethodPost, rawURL, strings.NewReader(bodyStr))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		req.Header.Set("Content-Type", "application/json")
		applyLuaHeaders(req, headersTbl)

		resp, err := httpClient.Do(req)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer resp.Body.Close()

		result, err := buildHTTPResult(L, resp)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(result)
		return 1
	}
}

// validateHTTPURL checks that the URL uses http/https and its host is on the plugin's allowlist.
func validateHTTPURL(rawURL string, allowlist []string) error {
	if len(allowlist) == 0 {
		return fmt.Errorf("%w: no HTTP allowlist configured, all requests blocked", ErrLuaSandboxViolation)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL: %v", ErrLuaSandboxViolation, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: only http and https schemes allowed, got %q", ErrLuaSandboxViolation, parsed.Scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: URL has no host", ErrLuaSandboxViolation)
	}

	for _, pattern := range allowlist {
		if matchHost(host, pattern) {
			return nil
		}
	}

	return fmt.Errorf("%w: host %q not on allowlist", ErrLuaSandboxViolation, host)
}

// matchHost checks if a hostname matches an allowlist pattern.
// Supports exact match and wildcard prefix (*.example.com matches sub.example.com).
func matchHost(host, pattern string) bool {
	if host == pattern {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix) && host != suffix[1:]
	}
	return false
}

func applyLuaHeaders(req *http.Request, headersTbl *lua.LTable) {
	if headersTbl == nil {
		return
	}
	headersTbl.ForEach(func(key, value lua.LValue) {
		if k, ok := key.(lua.LString); ok {
			if v, ok := value.(lua.LString); ok {
				req.Header.Set(string(k), string(v))
			}
		}
	})
}
