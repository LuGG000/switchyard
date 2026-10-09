package main

import "testing"

func TestIsHeadless(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"-p", "hello"}, true},
		{[]string{"--model", "haiku", "--print", "hello"}, true},
		{[]string{"--model", "haiku"}, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := isHeadless(tt.args); got != tt.want {
			t.Errorf("isHeadless(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}
