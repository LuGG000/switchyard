package main

import (
	"bytes"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "switchyard "+version+"\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
