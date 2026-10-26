// SPDX-License-Identifier: MPL-2.0
package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client uses only the documented Manifest API. Credentials never enter URLs.
type Client struct {
	token    string
	endpoint string
	http     *http.Client
}

func NewClient(token string, transport ...http.RoundTripper) *Client {
	var rt http.RoundTripper
	if len(transport) > 0 {
		rt = transport[0]
	}
	return &Client{token: token, endpoint: "https://slack.com/api/", http: &http.Client{
		Timeout: 30 * time.Second, Transport: rt,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type Response struct {
	OK          bool            `json:"ok"`
	Error       string          `json:"error"`
	AppID       string          `json:"app_id"`
	Manifest    json.RawMessage `json:"manifest"`
	Credentials struct {
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		SigningSecret string `json:"signing_secret"`
	} `json:"credentials"`
	OAuthAuthorizeURL  string `json:"oauth_authorize_url"`
	PermissionsUpdated bool   `json:"permissions_updated"`
}

type APIError struct{ Code string }

func (e *APIError) Error() string {
	if e.Code == "token_expired" {
		return "Slack configuration token expired. Supply a fresh SLACK_APP_CONFIGURATION_TOKEN; rotate tokens outside Terraform and persist both rotated tokens."
	}
	return "Slack API error: " + e.Code
}
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Code == "app_not_found"
}

var errorCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

// Call bounds rate-limit waits and retries only requests Slack explicitly rejected
// with HTTP 429. In particular, uncertain create outcomes are never replayed.
func (c *Client) Call(ctx context.Context, method string, args map[string]string) (Response, error) {
	switch method {
	case "create", "update", "delete", "export", "validate":
	default:
		return Response{}, errors.New("unsupported Slack API method")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	payload, err := json.Marshal(args)
	if err != nil {
		return Response{}, errors.New("cannot encode Slack request")
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"apps.manifest."+method, strings.NewReader(string(payload)))
		if err != nil {
			return Response{}, errors.New("cannot construct Slack request")
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "terraform-provider-slack")
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return Response{}, ctx.Err()
			}
			return Response{}, errors.New("Slack request failed; outcome may be unknown. Inspect the app dashboard before retrying a create and import any created app")
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Second * 60
			if n, e := strconv.ParseInt(resp.Header.Get("Retry-After"), 10, 64); e == nil && n >= 0 && n <= 300 {
				delay = time.Duration(n) * time.Second
			} else if resp.Header.Get("Retry-After") != "" {
				return Response{}, errors.New("Slack rate limit requires a later retry")
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Response{}, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return Response{}, fmt.Errorf("Slack API returned HTTP %d; response body omitted", resp.StatusCode)
		}
		if readErr != nil || len(body) > 4*1024*1024 {
			return Response{}, errors.New("cannot read Slack API response")
		}
		var result Response
		if json.Unmarshal(body, &result) != nil {
			return Response{}, errors.New("Slack API returned invalid JSON; response body omitted")
		}
		if !result.OK {
			code := result.Error
			// Never echo arbitrary server strings or validation messages containing input.
			if !errorCode.MatchString(code) || strings.Contains(code, c.token) {
				code = "unknown_error"
			}
			return Response{}, &APIError{Code: code}
		}
		return result, nil
	}
	return Response{}, errors.New("Slack rate limit retry budget exhausted")
}

func (c *Client) Export(ctx context.Context, id string) (string, error) {
	result, err := c.Call(ctx, "export", map[string]string{"app_id": id})
	if err != nil {
		return "", err
	}
	var obj map[string]any
	if json.Unmarshal(result.Manifest, &obj) != nil || obj == nil {
		return "", errors.New("Slack export response does not contain a manifest object")
	}
	return string(result.Manifest), nil
}
