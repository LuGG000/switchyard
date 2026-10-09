package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipOf(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// release serves a latest-release answer with the given archive and checksum line.
func release(t *testing.T, archiveName string, archive []byte, checksum string) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(archive)
	if checksum == "" {
		checksum = hex.EncodeToString(sum[:])
	}
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"tag_name":"v0.4.0","assets":[{"name":%q,"browser_download_url":"%s/dl/archive"},{"name":"checksums.txt","browser_download_url":"%s/dl/sums"}]}`, archiveName, base, base)
	})
	mux.HandleFunc("/dl/archive", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n%s  other.zip\n", checksum, archiveName, "00")
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func target(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "switchyard")
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstallReplacesTheBinaryFromATarball(t *testing.T) {
	srv := release(t, "switchyard_0.4.0_linux_amd64.tar.gz", tarGz(t, "switchyard", []byte("new binary")), "")
	path := target(t)
	in := &Installer{APIURL: srv.URL + "/latest", GOOS: "linux", GOARCH: "amd64"}

	version, err := in.Install(context.Background(), path)
	if err != nil || version != "0.4.0" {
		t.Fatalf("Install = %q, %v", version, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new binary" {
		t.Errorf("binary = %q, want the new one", got)
	}
}

func TestInstallReadsAZipForWindows(t *testing.T) {
	srv := release(t, "switchyard_0.4.0_windows_arm64.zip", zipOf(t, "switchyard.exe", []byte("new exe")), "")
	path := target(t)
	in := &Installer{APIURL: srv.URL + "/latest", GOOS: "windows", GOARCH: "arm64"}

	if _, err := in.Install(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new exe" {
		t.Errorf("binary = %q, want the new one", got)
	}
}

func TestInstallRefusesAChecksumMismatch(t *testing.T) {
	srv := release(t, "switchyard_0.4.0_linux_amd64.tar.gz", tarGz(t, "switchyard", []byte("new binary")), "deadbeef")
	path := target(t)
	in := &Installer{APIURL: srv.URL + "/latest", GOOS: "linux", GOARCH: "amd64"}

	if _, err := in.Install(context.Background(), path); err == nil {
		t.Fatal("want an error for a checksum mismatch")
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary = %q, must stay untouched", got)
	}
}

func TestInstallNeedsAnArchiveForThePlatform(t *testing.T) {
	srv := release(t, "switchyard_0.4.0_linux_amd64.tar.gz", tarGz(t, "switchyard", []byte("x")), "")
	in := &Installer{APIURL: srv.URL + "/latest", GOOS: "freebsd", GOARCH: "amd64"}

	if _, err := in.Install(context.Background(), target(t)); err == nil {
		t.Fatal("want an error when the release has no archive for this system")
	}
}
