package main

import (
	"errors"
	"slices"
	"testing"
)

func TestInstallModAddsMarketplaceThenInstalls(t *testing.T) {
	var calls [][]string
	run := func(args ...string) error {
		calls = append(calls, slices.Clone(args))
		return nil
	}
	if err := installMod(run, "owner/repo", "user"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"plugin", "marketplace", "add", "owner/repo", "--scope", "user"},
		{"plugin", "install", "switchyard-mod@switchyard", "--scope", "user"},
	}
	if !slices.EqualFunc(calls, want, slices.Equal) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

func TestInstallModStopsWhenTheMarketplaceCannotBeAdded(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	run := func(args ...string) error {
		calls++
		return boom
	}
	if err := installMod(run, "owner/repo", "user"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap boom", err)
	}
	if calls != 1 {
		t.Errorf("%d calls, want 1", calls)
	}
}
