package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "0.1.1", true},
		{"0.9.0", "0.10.0", true},
		{"1.0.0", "0.9.9", false},
		{"0.1.0", "0.1.0", false},
		{"v0.1.0", "0.2.0", true},
		{"0.1.0-rc1", "0.1.1", true},
		{"0.0.0-dev", "9.9.9", true},
		{"dev", "9.9.9", false},
		{"0.1.0", "latest", false},
	}
	for _, tc := range cases {
		if got := Newer(tc.current, tc.latest); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestReleased(t *testing.T) {
	for version, want := range map[string]bool{"0.1.0": true, "v1.2.3": true, "0.0.0-dev": false, "(devel)": false, "": false} {
		if got := Released(version); got != want {
			t.Errorf("Released(%q) = %v, want %v", version, got, want)
		}
	}
}

func server(t *testing.T, hits *atomic.Int32, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://example.test/r/v0.3.0"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatestCachesTheAnswer(t *testing.T) {
	var hits atomic.Int32
	srv := server(t, &hits, http.StatusOK)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	c := &Checker{Dir: t.TempDir(), APIURL: srv.URL, Now: func() time.Time { return now }}

	release, err := c.Latest(context.Background(), false)
	if err != nil || release.Version != "0.3.0" || release.URL != "https://example.test/r/v0.3.0" {
		t.Fatalf("Latest = %+v, %v", release, err)
	}
	now = now.Add(time.Hour)
	if _, err := c.Latest(context.Background(), false); err != nil || hits.Load() != 1 {
		t.Fatalf("second call: err %v, hits %d, want a cached answer", err, hits.Load())
	}
	if _, err := c.Latest(context.Background(), true); err != nil || hits.Load() != 2 {
		t.Fatalf("forced call: err %v, hits %d", err, hits.Load())
	}
	now = now.Add(TTL + time.Minute)
	if _, err := c.Latest(context.Background(), false); err != nil || hits.Load() != 3 {
		t.Fatalf("expired cache: err %v, hits %d", err, hits.Load())
	}
}

func TestLatestRemembersAFailure(t *testing.T) {
	var hits atomic.Int32
	srv := server(t, &hits, http.StatusNotFound)
	c := &Checker{Dir: t.TempDir(), APIURL: srv.URL}

	if _, err := c.Latest(context.Background(), false); err == nil {
		t.Fatal("want an error for a failed lookup")
	}
	if _, err := c.Latest(context.Background(), false); err != nil || hits.Load() != 1 {
		t.Fatalf("a failed lookup must not be retried within the TTL: err %v, hits %d", err, hits.Load())
	}
}
