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
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	tailscaleRe = regexp.MustCompile(`(?m)^\s*(?:require\s+)?tailscale\.com\s+v(\S+)`)
	versionRe   = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-.*)?$`)
)

var errAborted = errors.New("aborted")

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := chdirRepoRoot(); err != nil {
		return err
	}
	if err := checkClean(); err != nil {
		return err
	}
	tsVersion, err := tailscaleVersion()
	if err != nil {
		return err
	}
	latestTag, latest, err := latestRelease()
	if err != nil {
		return err
	}

	fmt.Printf("Current version:   %s\n", latestTag)
	fmt.Printf("Tailscale version: %s\n\n", tsVersion)

	stdin := bufio.NewReader(os.Stdin)

	next, err := askVersion(stdin, latest, latestTag)
	if err != nil {
		return err
	}
	tag := fmt.Sprintf("v%s-%s", next, tsVersion)

	if err := confirmTag(stdin, tag); err != nil {
		return err
	}
	if err := pushTag(tag); err != nil {
		return err
	}

	releaseURL, err := newReleaseURL(tag)
	if err != nil {
		return err
	}
	fmt.Printf("\nCreate the release: %s\n", releaseURL)
	return nil
}

// version is the tsnet-proxy part of a release: major, minor, patch.
type version [3]int

func (v version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

func (v version) compare(o version) int { return slices.Compare(v[:], o[:]) }

func (v version) nextMinor() version { return version{v[0], v[1] + 1, 0} }

// parseVersion parses a version or tag, ignoring the tailscale suffix:
// "v0.1.2-1.102.5" -> 0.1.2.
func parseVersion(s string) (version, bool) {
	match := versionRe.FindStringSubmatch(s)
	if match == nil {
		return version{}, false
	}
	var v version
	for i, part := range match[1:] {
		n, err := strconv.Atoi(part)
		if err != nil {
			return version{}, false
		}
		v[i] = n
	}
	return v, true
}

func chdirRepoRoot() error {
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	return os.Chdir(root)
}

func checkClean() error {
	status, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return errors.New("working tree is not clean")
	}
	return nil
}

func tailscaleVersion() (string, error) {
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	match := tailscaleRe.FindSubmatch(goMod)
	if match == nil {
		return "", errors.New("could not find tailscale.com version in go.mod")
	}
	return string(match[1]), nil
}

// latestRelease returns the tag with the highest tsnet-proxy version on
// origin, or "<none>" and a zero version when there are no releases yet.
func latestRelease() (string, version, error) {
	if _, err := git("fetch", "--tags", "--quiet", "origin"); err != nil {
		return "", version{}, err
	}
	out, err := git("tag", "-l", "v*")
	if err != nil {
		return "", version{}, err
	}
	latestTag, latest := "<none>", version{}
	for _, tag := range strings.Fields(out) {
		if v, ok := parseVersion(tag); ok && v.compare(latest) > 0 {
			latestTag, latest = tag, v
		}
	}
	return latestTag, latest, nil
}

// askVersion prompts for a version greater than latest, suggesting the next
// minor version.
func askVersion(r *bufio.Reader, latest version, latestTag string) (version, error) {
	suggested := latest.nextMinor()
	input, err := prompt(r, fmt.Sprintf("New tsnet-proxy version [%s]: ", suggested))
	if err != nil {
		return version{}, err
	}
	if input == "" {
		return suggested, nil
	}
	v, ok := parseVersion(input)
	if !ok || strings.Contains(input, "-") {
		return version{}, errors.New("version must look like X.Y.Z")
	}
	if v.compare(latest) <= 0 {
		return version{}, fmt.Errorf("version %s must be greater than the current version %s", v, latestTag)
	}
	return v, nil
}

func confirmTag(r *bufio.Reader, tag string) error {
	head, err := git("rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}
	answer, err := prompt(r, fmt.Sprintf("Tag %s as %s and push to origin? [y/N] ", head, tag))
	if err != nil {
		return err
	}
	if !strings.EqualFold(answer, "y") {
		return errAborted
	}
	return nil
}

func pushTag(tag string) error {
	if _, err := git("tag", tag); err != nil {
		return err
	}
	push := exec.Command("git", "push", "origin", tag)
	push.Stdout, push.Stderr = os.Stdout, os.Stderr
	if err := push.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	return nil
}

func newReleaseURL(tag string) (string, error) {
	repoURL, err := git("remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	repoURL = strings.TrimSuffix(repoURL, ".git")
	if rest, ok := strings.CutPrefix(repoURL, "git@github.com:"); ok {
		repoURL = "https://github.com/" + rest
	}
	return fmt.Sprintf("%s/releases/new?tag=%s", repoURL, url.QueryEscape(tag)), nil
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

func prompt(r *bufio.Reader, question string) (string, error) {
	fmt.Print(question)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return "", errAborted
	}
	return strings.TrimSpace(line), nil
}
