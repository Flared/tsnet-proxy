package main

import (
	"context"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"tailscale.com/client/tailscale/apitype"
)

const (
	headerUserLogin      = "Tailscale-User-Login"
	headerUserName       = "Tailscale-User-Name"
	headerUserProfilePic = "Tailscale-User-Profile-Pic"
	headerNodeName       = "Tailscale-Node-Name"
	headerNodeTags       = "Tailscale-Node-Tags"
	headerPrefix         = "Tailscale-"
)

type whoIser interface {
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
}

func newReverseProxy(target *url.URL, wc whoIser) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded()
			stripTailscaleHeaders(r.Out.Header)
			who, err := wc.WhoIs(r.In.Context(), r.In.RemoteAddr)
			if err != nil {
				log.Printf("whois %s: %v", r.In.RemoteAddr, err)
				return
			}
			addIdentityHeaders(r.Out.Header, who)
			log.Printf("%s %s from %s -> %s %s", r.In.Method, r.In.URL, r.In.RemoteAddr, r.Out.URL, formatTailscaleHeaders(r.Out.Header))
		},
	}
}

func stripTailscaleHeaders(h http.Header) {
	for k := range h {
		if strings.HasPrefix(k, headerPrefix) {
			h.Del(k)
		}
	}
}

func formatTailscaleHeaders(h http.Header) string {
	var parts []string
	for k, v := range h {
		if strings.HasPrefix(k, headerPrefix) {
			parts = append(parts, k+"="+strings.Join(v, ","))
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, " ")
}

func addIdentityHeaders(h http.Header, who *apitype.WhoIsResponse) {
	if who == nil || who.Node == nil {
		return
	}

	h.Set(headerNodeName, encHeader(strings.TrimSuffix(who.Node.Name, ".")))

	if who.Node.IsTagged() {
		h.Set(headerNodeTags, encHeader(strings.Join(who.Node.Tags, ",")))
		return
	}

	if who.UserProfile == nil {
		return
	}
	h.Set(headerUserLogin, encHeader(who.UserProfile.LoginName))
	h.Set(headerUserName, encHeader(who.UserProfile.DisplayName))
	h.Set(headerUserProfilePic, who.UserProfile.ProfilePicURL)
}

func encHeader(v string) string {
	if !utf8.ValidString(v) {
		return ""
	}
	return mime.QEncoding.Encode("utf-8", v)
}

func serveTCP(ctx context.Context, ln net.Listener, target string) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go proxyTCP(ctx, c, target)
	}
}

func proxyTCP(ctx context.Context, c net.Conn, target string) {
	defer c.Close()
	var d net.Dialer
	upstream, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		log.Printf("dial %s: %v", target, err)
		return
	}
	defer upstream.Close()

	go io.Copy(upstream, c)
	io.Copy(c, upstream)
}
