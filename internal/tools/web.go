package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
type WebTool struct{}

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
