//go:build !windows

package main

import "os"

// enableANSI is a no-op: Unix terminals understand escape sequences.
func enableANSI(*os.File) func() { return func() {} }
