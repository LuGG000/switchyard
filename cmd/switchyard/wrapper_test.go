package main

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestWrapperInvocation(t *testing.T) {
	bundled := "/ext/resources/native-binaries/linux-x64/claude"
	if runtime.GOOS == "windows" {
		bundled = `C:\ext\resources\native-binaries\win32-x64\claude.exe`
	}
	sdk := []string{"--output-format", "stream-json", "--verbose", "--input-format", "stream-json"}
	tests := []struct {
		name   string
		args   []string
		claude string
		rest   []string
		ok     bool
	}{
		{"bundled claude", append([]string{bundled}, sdk...), bundled, sdk, true},
		{"no bundled claude", sdk, "", sdk, true},
		{"a command", []string{"status"}, "", nil, false},
		{"no arguments", nil, "", nil, false},
		{"relative claude", append([]string{filepath.Join("bin", "claude")}, sdk...), "", nil, false},
		{"another program", []string{filepath.Join(filepath.Dir(bundled), "node")}, "", nil, false},
	}
	for _, tt := range tests {
		claude, rest, ok := wrapperInvocation(tt.args)
		if ok != tt.ok || claude != tt.claude || !slices.Equal(rest, tt.rest) {
			t.Errorf("%s: got %q %v %v", tt.name, claude, rest, ok)
		}
	}
}
