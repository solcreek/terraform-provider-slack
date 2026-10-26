// SPDX-License-Identifier: MPL-2.0
package slack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func clientFor(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c := NewClient("secret-token")
	c.endpoint = s.URL + "/"
	return c
}
func TestCallWireFormat(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/apps.manifest.create" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"manifest":"{\"display_information\":{\"name\":\"Example\"}}"}` {
			t.Errorf("wrong encoding: %s", b)
		}
		fmt.Fprint(w, `{"ok":true,"app_id":"A123"}`)
	})
	r, err := c.Call(context.Background(), "create", map[string]string{"manifest": `{"display_information":{"name":"Example"}}`})
	if err != nil || r.AppID != "A123" {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		missing    bool
	}{
		{"missing", `{"ok":false,"error":"app_not_found"}`, 200, true},
		{"denied", `{"ok":false,"error":"access_denied"}`, 200, false},
		{"expired", `{"ok":false,"error":"token_expired"}`, 200, false},
		{"echo", `{"ok":false,"error":"secret-token"}`, 200, false},
		{"bad-json", `secret-token`, 200, false},
		{"server", `secret-token`, 503, false},
		{"empty", `{}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			_, err := c.Call(context.Background(), "create", nil)
			if err == nil || IsNotFound(err) != tc.missing || strings.Contains(err.Error(), "secret-token") || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}
func TestRateLimit(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})
	if _, err := c.Call(context.Background(), "create", nil); err != nil || calls != 3 {
		t.Fatalf("%v calls=%d", err, calls)
	}
}
func TestRateLimitCancellation(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "60"); w.WriteHeader(429) })
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := c.Call(ctx, "export", nil); err == nil {
		t.Fatal("expected cancellation")
	}
}
func TestRateLimitBudget(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	})
	if _, err := c.Call(context.Background(), "create", nil); err == nil || calls != 4 {
		t.Fatalf("%v calls=%d", err, calls)
	}
}
func TestRedirectNotFollowed(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://example.com/leak", 307) })
	if _, err := c.Call(context.Background(), "export", nil); err == nil {
		t.Fatal("expected redirect rejection")
	}
}
func TestExportRejectsMissingManifest(t *testing.T) {
	for _, body := range []string{`{"ok":true}`, `{"ok":true,"manifest":null}`, `{"ok":true,"manifest":[]}`} {
		c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		if _, err := c.Export(context.Background(), "A123"); err == nil {
			t.Fatal("expected invalid export")
		}
	}
}
