// Command tsnet-proxy exposes a target service on the tailnet under a chosen
// hostname. It proxies raw TCP, or reverse proxies HTTP/HTTPS and injects
// Tailscale identity headers from WhoIs, including for tagged nodes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tailscale.com/tsnet"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], os.LookupEnv, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg *config) error {
	s := &tsnet.Server{
		Hostname:      cfg.Hostname,
		Dir:           cfg.StateDir,
		Ephemeral:     cfg.Ephemeral,
		AdvertiseTags: cfg.AdvertiseTags,
	}
	if cfg.Verbose {
		s.Logf = log.Printf
	}
	defer s.Close()

	if _, err := s.Up(ctx); err != nil {
		return fmt.Errorf("tsnet up: %w", err)
	}

	var ln net.Listener
	var err error
	if cfg.Mode == ModeHTTPS {
		ln, err = s.ListenTLS("tcp", cfg.listenAddr())
	} else {
		ln, err = s.Listen("tcp", cfg.listenAddr())
	}
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	log.Printf("proxying %s (%s) -> %s%s on tailnet", cfg.Target, cfg.Mode, cfg.Hostname, cfg.listenAddr())

	if cfg.Mode == ModeTCP {
		return serveTCP(ctx, ln, cfg.Target)
	}

	lc, err := s.LocalClient()
	if err != nil {
		return fmt.Errorf("local client: %w", err)
	}
	return serveHTTP(ctx, ln, newReverseProxy(cfg.targetURL, lc))
}

func serveHTTP(ctx context.Context, ln net.Listener, rp *httputil.ReverseProxy) error {
	srv := &http.Server{Handler: rp}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	err := srv.Serve(ln)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
