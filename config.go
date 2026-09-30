package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	ModeTCP   = "tcp"
	ModeHTTP  = "http"
	ModeHTTPS = "https"
)

type config struct {
	Hostname      string
	Target        string
	Mode          string
	ListenPort    int
	StateDir      string
	Ephemeral     bool
	AdvertiseTags []string
	Verbose       bool

	targetURL *url.URL
}

func (c *config) listenAddr() string {
	return ":" + strconv.Itoa(c.ListenPort)
}

type lookupEnvFunc func(string) (string, bool)

func parseConfig(args []string, lookupEnv lookupEnvFunc, output io.Writer) (*config, error) {
	env := func(name, def string) string {
		if v, ok := lookupEnv(name); ok {
			return v
		}
		return def
	}
	envBool := func(name string) bool {
		v, _ := strconv.ParseBool(env(name, "false"))
		return v
	}

	fs := flag.NewFlagSet("tsnet-proxy", flag.ContinueOnError)
	fs.SetOutput(output)

	cfg := &config{}
	var listenPort, tags string
	fs.StringVar(
		&cfg.Hostname,
		"hostname",
		env("TSNET_PROXY_HOSTNAME", ""),
		"tailnet hostname to register (env TSNET_PROXY_HOSTNAME)",
	)
	fs.StringVar(
		&cfg.Target,
		"target",
		env("TSNET_PROXY_TARGET", ""),
		"target to proxy to: a port, host:port, or http(s):// URL (env TSNET_PROXY_TARGET)",
	)
	fs.StringVar(
		&cfg.Mode,
		"mode",
		env("TSNET_PROXY_MODE", ModeTCP),
		"proxy mode: tcp, http or https (env TSNET_PROXY_MODE)",
	)
	fs.StringVar(
		&listenPort,
		"listen-port",
		env("TSNET_PROXY_LISTEN_PORT", ""),
		"tailnet port to listen on; defaults to 443 for https, 80 for http, the target port for tcp (env TSNET_PROXY_LISTEN_PORT)",
	)
	fs.StringVar(
		&cfg.StateDir,
		"state-dir",
		env("TSNET_PROXY_STATE_DIR", ""),
		"directory to persist tsnet state; defaults to a per-user config dir (env TSNET_PROXY_STATE_DIR)",
	)
	fs.BoolVar(
		&cfg.Ephemeral,
		"ephemeral",
		envBool("TSNET_PROXY_EPHEMERAL"),
		"register as an ephemeral node (env TSNET_PROXY_EPHEMERAL)",
	)
	fs.StringVar(
		&tags,
		"advertise-tags",
		env("TSNET_PROXY_ADVERTISE_TAGS", ""),
		"comma-separated tags to advertise, e.g. tag:proxy (env TSNET_PROXY_ADVERTISE_TAGS)",
	)
	fs.BoolVar(
		&cfg.Verbose,
		"v",
		envBool("TSNET_PROXY_VERBOSE"),
		"verbose tsnet backend logs (env TSNET_PROXY_VERBOSE)",
	)
	fs.Usage = func() {
		fmt.Fprintf(output, "usage: tsnet-proxy [flags]\n\nAuthentication uses TS_AUTHKEY or TS_CLIENT_SECRET.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() != 0 {
		return nil, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	if cfg.Hostname == "" {
		return nil, errors.New("--hostname is required")
	}
	if cfg.Target == "" {
		return nil, errors.New("--target is required")
	}
	switch cfg.Mode {
	case ModeTCP, ModeHTTP, ModeHTTPS:
	default:
		return nil, fmt.Errorf("invalid --mode %q: must be tcp, http or https", cfg.Mode)
	}
	for _, t := range strings.Split(tags, ",") {
		if t = strings.TrimSpace(t); t != "" {
			cfg.AdvertiseTags = append(cfg.AdvertiseTags, t)
		}
	}

	targetPort, err := cfg.normalizeTarget()
	if err != nil {
		return nil, fmt.Errorf("invalid --target %q: %w", cfg.Target, err)
	}

	cfg.ListenPort = defaultListenPort(cfg.Mode, targetPort)
	if listenPort != "" {
		cfg.ListenPort, err = parsePort(listenPort)
		if err != nil {
			return nil, fmt.Errorf("invalid --listen-port %q: %w", listenPort, err)
		}
	}

	return cfg, nil
}

func (c *config) normalizeTarget() (int, error) {
	if p, err := parsePort(c.Target); err == nil {
		c.Target = net.JoinHostPort("localhost", strconv.Itoa(p))
	}

	if c.Mode == ModeTCP {
		host, port, err := net.SplitHostPort(c.Target)
		if err != nil {
			return 0, errors.New("tcp mode requires a port or host:port")
		}
		if host == "" {
			return 0, errors.New("missing host")
		}
		return parsePort(port)
	}

	raw := c.Target
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return 0, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return 0, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return 0, errors.New("missing host")
	}
	c.targetURL = u
	c.Target = u.String()
	if u.Port() == "" {
		return 0, nil
	}
	return parsePort(u.Port())
}

func defaultListenPort(mode string, targetPort int) int {
	switch mode {
	case ModeHTTPS:
		return 443
	case ModeHTTP:
		return 80
	}
	return targetPort
}

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 || p > 65535 {
		return 0, errors.New("bad port")
	}
	return p, nil
}
