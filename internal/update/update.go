// Package update finds out whether a newer switchyard release exists. It only
// reads the public release list of the repository and never installs anything.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// Repo is the GitHub repository whose releases are checked.
	Repo = "LuGG000/switchyard"
	// ReleasesPage lists the releases for humans.
	ReleasesPage = "https://github.com/" + Repo + "/releases"

	cacheFile = "update-check.json"
	// TTL is how long a check result is reused before GitHub is asked again.
	TTL = 24 * time.Hour
)

// Release is a published version.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Checker looks up the latest release and caches the answer in Dir.
type Checker struct {
	// Dir holds the cache file, normally the data dir.
	Dir string
	// APIURL is the "latest release" endpoint; empty means GitHub's.
	APIURL string
	Client *http.Client
	Now    func() time.Time
}

type cache struct {
	CheckedAt time.Time `json:"checked_at"`
	Release
}

// Latest returns the newest release. A cached answer younger than TTL is used
// unless force is set. A failed lookup is cached as well, so a machine without
// network access does not retry on every command; the previous answer, if any,
// is returned together with the error.
func (c *Checker) Latest(ctx context.Context, force bool) (Release, error) {
	now := c.now()
	stored, _ := c.read()
	if !force && !stored.CheckedAt.IsZero() && now.Sub(stored.CheckedAt) < TTL {
		return stored.Release, nil
	}
	release, err := c.fetch(ctx)
	if err != nil {
		stored.CheckedAt = now
		_ = c.write(stored)
		return stored.Release, err
	}
	_ = c.write(cache{CheckedAt: now, Release: release})
	return release, nil
}

func (c *Checker) fetch(ctx context.Context) (Release, error) {
	url := c.APIURL
	if url == "" {
		url = "https://api.github.com/repos/" + Repo + "/releases/latest"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("release lookup: %s", resp.Status)
	}
	var body struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("release lookup: %w", err)
	}
	if body.Tag == "" {
		return Release{}, errors.New("release lookup: no tag")
	}
	return Release{Version: strings.TrimPrefix(body.Tag, "v"), URL: body.URL}, nil
}

func (c *Checker) read() (cache, error) {
	var stored cache
	data, err := os.ReadFile(filepath.Join(c.Dir, cacheFile))
	if errors.Is(err, fs.ErrNotExist) {
		return stored, nil
	}
	if err != nil {
		return stored, err
	}
	err = json.Unmarshal(data, &stored)
	return stored, err
}

func (c *Checker) write(stored cache) error {
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.Dir, cacheFile), data, 0o600)
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Newer reports whether latest is a higher release than current. Versions are
// compared as major.minor.patch; a pre-release suffix is ignored, and a version
// that does not parse (a development build) is never considered outdated.
func Newer(current, latest string) bool {
	cur, ok := parse(current)
	if !ok {
		return false
	}
	next, ok := parse(latest)
	if !ok {
		return false
	}
	for i := range cur {
		if next[i] != cur[i] {
			return next[i] > cur[i]
		}
	}
	return false
}

// Released reports whether version looks like a published release (not the
// "0.0.0-dev" placeholder of a local build).
func Released(version string) bool {
	parts, ok := parse(version)
	return ok && parts != [3]int{} && !strings.Contains(version, "dev")
}

func parse(version string) ([3]int, bool) {
	var out [3]int
	version = strings.TrimPrefix(version, "v")
	if i := strings.IndexAny(version, "-+"); i >= 0 {
		version = version[:i]
	}
	fields := strings.Split(version, ".")
	if len(fields) != 3 {
		return out, false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Cached returns the answer of the last lookup without asking GitHub, so it is
// cheap enough for hooks and `status --json`. ok is false if nothing is cached.
func (c *Checker) Cached() (Release, bool) {
	stored, err := c.read()
	return stored.Release, err == nil && stored.Version != ""
}
