package main

import (
	"io"
	"reflect"
	"testing"
)

func envMap(m map[string]string) lookupEnvFunc {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		env        map[string]string
		wantTarget string
		wantMode   string
		wantPort   int
		wantTags   []string
	}{
		{
			name:       "tcp port defaults to localhost",
			args:       []string{"--hostname=app", "--target=8080"},
			wantTarget: "localhost:8080",
			wantMode:   ModeTCP,
			wantPort:   8080,
		},
		{
			name:       "tcp remote host with listen port",
			args:       []string{"--hostname=db", "--target=postgres:5432", "--listen-port=15432"},
			wantTarget: "postgres:5432",
			wantMode:   ModeTCP,
			wantPort:   15432,
		},
		{
			name:       "http host:port",
			args:       []string{"--hostname=app", "--mode=http", "--target=backend:3000"},
			wantTarget: "http://backend:3000",
			wantMode:   ModeHTTP,
			wantPort:   80,
		},
		{
			name:       "https with https target url",
			args:       []string{"--hostname=app", "--mode=https", "--target=https://internal.example.com/base"},
			wantTarget: "https://internal.example.com/base",
			wantMode:   ModeHTTPS,
			wantPort:   443,
		},
		{
			name: "env vars",
			env: map[string]string{
				"TSNET_PROXY_HOSTNAME":       "app",
				"TSNET_PROXY_MODE":           "https",
				"TSNET_PROXY_TARGET":         "9000",
				"TSNET_PROXY_ADVERTISE_TAGS": "tag:a, tag:b",
			},
			wantTarget: "http://localhost:9000",
			wantMode:   ModeHTTPS,
			wantPort:   443,
			wantTags:   []string{"tag:a", "tag:b"},
		},
		{
			name:       "flags override env",
			args:       []string{"--target=backend:1234"},
			env:        map[string]string{"TSNET_PROXY_HOSTNAME": "app", "TSNET_PROXY_TARGET": "9000"},
			wantTarget: "backend:1234",
			wantMode:   ModeTCP,
			wantPort:   1234,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseConfig(tt.args, envMap(tt.env), io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Target != tt.wantTarget || cfg.Mode != tt.wantMode || cfg.ListenPort != tt.wantPort {
				t.Errorf("got target=%q mode=%q port=%d", cfg.Target, cfg.Mode, cfg.ListenPort)
			}
			if !reflect.DeepEqual(cfg.AdvertiseTags, tt.wantTags) {
				t.Errorf("got tags %v, want %v", cfg.AdvertiseTags, tt.wantTags)
			}
		})
	}
}

func TestParseConfigErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--target=8080"},
		{"--hostname=app"},
		{"--hostname=app", "--target=8080", "--mode=udp"},
		{"--hostname=app", "--target=http://backend:80"},
		{"--hostname=app", "--target=backend"},
		{"--hostname=app", "--target=backend:99999"},
		{"--hostname=app", "--mode=http", "--target=ftp://backend"},
		{"--hostname=app", "--target=8080", "--listen-port=0"},
		{"--hostname=app", "--target=8080", "extra"},
	} {
		if _, err := parseConfig(args, envMap(nil), io.Discard); err == nil {
			t.Errorf("parseConfig(%v): expected error", args)
		}
	}
}
