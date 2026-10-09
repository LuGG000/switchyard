package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// maxDownload bounds every downloaded file.
const maxDownload = 100 << 20

// Installer replaces the switchyard binary with the latest release. It does
// not touch running processes: a running binary keeps working (on Windows it is
// renamed aside, elsewhere the new file is renamed over it), and everything
// started afterwards, such as the hook commands of a running session, uses the
// new binary at once.
type Installer struct {
	// APIURL is the "latest release" endpoint; empty means GitHub's.
	APIURL string
	Client *http.Client
	// GOOS and GOARCH select the archive; empty means this build's.
	GOOS, GOARCH string
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Install downloads the latest release, checks it against the published
// checksums and puts the binary at target. It returns the installed version.
func (in *Installer) Install(ctx context.Context, target string) (string, error) {
	version, assets, err := in.release(ctx)
	if err != nil {
		return "", err
	}
	goos, goarch := in.GOOS, in.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	name := fmt.Sprintf("switchyard_%s_%s_%s.%s", version, goos, goarch, ext)
	archive, ok := assets[name]
	if !ok {
		return "", fmt.Errorf("release %s has no %s", version, name)
	}
	sums, ok := assets["checksums.txt"]
	if !ok {
		return "", fmt.Errorf("release %s has no checksums.txt", version)
	}

	sumData, err := in.download(ctx, sums)
	if err != nil {
		return "", err
	}
	want, err := checksumFor(sumData, name)
	if err != nil {
		return "", err
	}
	data, err := in.download(ctx, archive)
	if err != nil {
		return "", err
	}
	if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
		return "", fmt.Errorf("%s does not match its checksum; nothing was installed", name)
	}
	binary := "switchyard"
	if goos == "windows" {
		binary += ".exe"
	}
	exe, err := extract(data, ext, binary)
	if err != nil {
		return "", err
	}
	if err := Replace(target, exe); err != nil {
		return "", err
	}
	return version, nil
}

func (in *Installer) release(ctx context.Context) (string, map[string]string, error) {
	url := in.APIURL
	if url == "" {
		url = "https://api.github.com/repos/" + Repo + "/releases/latest"
	}
	data, err := in.get(ctx, url, "application/vnd.github+json")
	if err != nil {
		return "", nil, err
	}
	var body struct {
		Tag    string  `json:"tag_name"`
		Assets []asset `json:"assets"`
	}
	if err := json.Unmarshal(data, &body); err != nil || body.Tag == "" {
		return "", nil, errors.New("release lookup: unexpected answer")
	}
	assets := make(map[string]string, len(body.Assets))
	for _, a := range body.Assets {
		assets[a.Name] = a.URL
	}
	return strings.TrimPrefix(body.Tag, "v"), assets, nil
}

func (in *Installer) download(ctx context.Context, url string) ([]byte, error) {
	return in.get(ctx, url, "application/octet-stream")
}

func (in *Installer) get(ctx context.Context, url, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	client := in.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDownload {
		return nil, errors.New("download: file is too large")
	}
	return data, nil
}

// checksumFor finds the sha256 of name in a checksums.txt ("<hex>  <name>" per line).
func checksumFor(sums []byte, name string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

// extract returns the file called name from a tar.gz or zip archive.
func extract(data []byte, ext, name string) ([]byte, error) {
	if ext == "zip" {
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range r.File {
			if f.Name != name {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return readLimited(rc)
		}
		return nil, fmt.Errorf("archive has no %s", name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("archive has no %s", name)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && hdr.Name == name {
			return readLimited(tr)
		}
	}
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDownload {
		return nil, errors.New("archive: file is too large")
	}
	return data, nil
}

// OldSuffix marks the previous binary that Replace leaves next to the new one
// on Windows, where a running executable can be renamed but not deleted.
const OldSuffix = ".old"

// Replace puts exe at target. On Windows the running file is renamed aside
// first; CleanOld removes it at a later start.
func Replace(target string, exe []byte) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".new-*")
	if err != nil {
		return fmt.Errorf("%w (is %s writable? install it again from the releases page)", err, dir)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(exe); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		old := target + OldSuffix
		_ = os.Remove(old)
		if err := os.Rename(target, old); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), target); err != nil {
			_ = os.Rename(old, target)
			return err
		}
		return nil
	}
	return os.Rename(tmp.Name(), target)
}

// CleanOld removes the binary left behind by a Windows update. A failure is
// not reported: the file is only in the way of disk space.
func CleanOld(target string) {
	_ = os.Remove(target + OldSuffix)
}
