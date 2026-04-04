package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxResponseBody = 1024 * 1024 // 1 MB
	webTimeout      = 30 * time.Second
	maxRedirects    = 5
	userAgent       = "siply/1.0"
)

// WebTool fetches URL content.
type WebTool struct {
	// allowLocalhost disables SSRF checks for testing with httptest servers.
	allowLocalhost bool
}

type webInput struct {
	URL string `json:"url"`
}

func (t *WebTool) Name() string        { return "web" }
func (t *WebTool) Description() string { return "Fetch URL content" }
func (t *WebTool) Destructive() bool   { return false }
func (t *WebTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"URL to fetch"}},"required":["url"]}`)
}

func (t *WebTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params webInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("web: invalid input: %w", err)
	}
	if params.URL == "" {
		return "", fmt.Errorf("web: url is required")
	}

	// Only allow http:// and https:// schemes.
	if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
		return "", fmt.Errorf("web: unsupported scheme (only http and https allowed)")
	}

	// Block local/private network targets to mitigate SSRF.
	if !t.allowLocalhost {
		if err := rejectPrivateURL(params.URL); err != nil {
			return "", err
		}
	}

	client := &http.Client{
		Timeout: webTimeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("web: too many redirects (max %d)", maxRedirects)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return "", fmt.Errorf("web: creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP error status codes.
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("web: HTTP %d %s: %s", resp.StatusCode, http.StatusText(resp.StatusCode), strings.TrimSpace(string(body)))
	}

	// Read up to maxResponseBody + 1 to detect truncation.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return "", fmt.Errorf("web: reading response: %w", err)
	}

	output := string(body)
	truncated := false
	if len(body) > maxResponseBody {
		output = string(body[:maxResponseBody])
		truncated = true
	}

	// Strip HTML tags for HTML content.
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		output = stripHTMLTags(output)
	}

	if truncated {
		output += fmt.Sprintf("\n... response truncated (exceeded %d bytes)", maxResponseBody)
	}

	return output, nil
}

// rejectPrivateURL blocks requests to loopback, link-local, and private network addresses.
func rejectPrivateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("web: invalid URL: %w", err)
	}
	host := u.Hostname()

	// Reject well-known loopback names.
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return fmt.Errorf("web: requests to localhost/loopback are blocked")
	}

	// Resolve and check IP ranges.
	ips, err := net.LookupHost(host)
	if err != nil {
		return nil // DNS failure will be caught by http.Client
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("web: requests to private/internal networks are blocked (%s resolves to %s)", host, ipStr)
		}
	}
	return nil
}

// stripHTMLTags removes HTML tags from content, keeping text.
func stripHTMLTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}
