//usr/bin/env go run "$0" "$@"; exit

// Command release tags and pushes a release as
// v<tsnet-proxy version>-<tailscale version>.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	tailscaleRe = regexp.MustCompile(`(?m)^\s*(?:require\s+)?tailscale\.com\s+v(\S+)`)
	versionRe   = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return err
	}

	status, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return errors.New("working tree is not clean")
	}

	tsVersion, err := tailscaleVersion(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}

	if _, err := git("fetch", "--tags", "--quiet", "origin"); err != nil {
		return err
	}
	out, err := git("tag", "-l", "v*", "--sort=-v:refname")
	if err != nil {
		return err
	}
	tags := strings.Fields(out)
	currentTag := "<none>"
	if len(tags) > 0 {
		currentTag = tags[0]
	}

	fmt.Printf("Current version:   %s\n", currentTag)
	fmt.Printf("Tailscale version: %s\n\n", tsVersion)

	stdin := bufio.NewReader(os.Stdin)

	version, err := prompt(stdin, "New tsnet-proxy version (e.g. 0.1.3): ")
	if err != nil {
		return err
	}
	version = strings.TrimPrefix(version, "v")
	if !versionRe.MatchString(version) {
		return errors.New("version must look like X.Y.Z")
	}

	tag := fmt.Sprintf("v%s-%s", version, tsVersion)
	if slices.Contains(tags, tag) {
		return fmt.Errorf("tag %s already exists", tag)
	}

	head, err := git("rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}
	confirm, err := prompt(stdin, fmt.Sprintf("Tag %s as %s and push to origin? [y/N] ", head, tag))
	if err != nil {
		return err
	}
	if !strings.EqualFold(confirm, "y") {
		return errors.New("aborted")
	}

	if _, err := git("tag", tag); err != nil {
		return err
	}
	push := exec.Command("git", "push", "origin", tag)
	push.Stdout, push.Stderr = os.Stdout, os.Stderr
	if err := push.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	repoURL, err := git("remote", "get-url", "origin")
	if err != nil {
		return err
	}
	repoURL = strings.TrimSuffix(repoURL, ".git")
	if rest, ok := strings.CutPrefix(repoURL, "git@github.com:"); ok {
		repoURL = "https://github.com/" + rest
	}
	fmt.Printf("\nCreate the release: %s/releases/new?tag=%s\n", repoURL, url.QueryEscape(tag))
	return nil
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func tailscaleVersion(goModPath string) (string, error) {
	goMod, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	match := tailscaleRe.FindSubmatch(goMod)
	if match == nil {
		return "", errors.New("could not find tailscale.com version in go.mod")
	}
	return string(match[1]), nil
}

func prompt(r *bufio.Reader, question string) (string, error) {
	fmt.Print(question)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return "", errors.New("aborted")
	}
	return strings.TrimSpace(line), nil
}
