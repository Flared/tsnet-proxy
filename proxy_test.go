package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

type fakeWhoIs struct {
	resp *apitype.WhoIsResponse
	err  error
}

func (f fakeWhoIs) WhoIs(context.Context, string) (*apitype.WhoIsResponse, error) {
	return f.resp, f.err
}

func TestReverseProxyHeaders(t *testing.T) {
	user := &apitype.WhoIsResponse{
		Node: &tailcfg.Node{Name: "laptop.tailnet.ts.net."},
		UserProfile: &tailcfg.UserProfile{
			LoginName:     "alice@example.com",
			DisplayName:   "Alice Ünicode",
			ProfilePicURL: "https://example.com/pic.png",
		},
	}
	tagged := &apitype.WhoIsResponse{
		Node:        &tailcfg.Node{Name: "ci-runner.tailnet.ts.net.", Tags: []string{"tag:ci", "tag:prod"}},
		UserProfile: &tailcfg.UserProfile{LoginName: "tagged-devices", DisplayName: "tagged-devices"},
	}

	tests := []struct {
		name string
		who  fakeWhoIs
		want map[string]string
	}{
		{
			name: "user node",
			who:  fakeWhoIs{resp: user},
			want: map[string]string{
				headerNodeName:       "laptop.tailnet.ts.net",
				headerNodeTags:       "",
				headerUserLogin:      "alice@example.com",
				headerUserName:       "=?utf-8?q?Alice_=C3=9Cnicode?=",
				headerUserProfilePic: "https://example.com/pic.png",
				"Tailscale-Spoofed":  "",
				"X-Forwarded-Host":   "app.tailnet.ts.net",
			},
		},
		{
			name: "tagged node",
			who:  fakeWhoIs{resp: tagged},
			want: map[string]string{
				headerNodeName:       "ci-runner.tailnet.ts.net",
				headerNodeTags:       "tag:ci,tag:prod",
				headerUserLogin:      "",
				headerUserName:       "",
				headerUserProfilePic: "",
				"Tailscale-Spoofed":  "",
			},
		},
		{
			name: "whois error",
			who:  fakeWhoIs{err: errors.New("boom")},
			want: map[string]string{
				headerNodeName:      "",
				headerUserLogin:     "",
				"Tailscale-Spoofed": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got http.Header
			var gotPath string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				gotPath = r.URL.Path
			}))
			defer backend.Close()

			target, _ := url.Parse(backend.URL + "/base")
			rp := newReverseProxy(target, tt.who)

			req := httptest.NewRequest(http.MethodGet, "http://app.tailnet.ts.net/hello", nil)
			req.Header.Set(headerUserLogin, "mallory@example.com")
			req.Header.Set(headerNodeTags, "tag:admin")
			req.Header.Set("Tailscale-Spoofed", "1")
			rec := httptest.NewRecorder()
			rp.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			if gotPath != "/base/hello" {
				t.Errorf("path = %q", gotPath)
			}
			for k, v := range tt.want {
				if g := got.Get(k); g != v {
					t.Errorf("%s = %q, want %q", k, g, v)
				}
			}
		})
	}
}
