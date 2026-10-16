package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/sdkproxy"
	"github.com/LuGG000/switchyard/internal/state"
)

// wrapperInvocation recognizes switchyard started by the Claude Code extension for VS Code as its
// claudeCode.claudeProcessWrapper: the extension passes the path of its own claude executable
// first and the claude arguments after it. Without a bundled executable the arguments come alone,
// starting with the SDK's --output-format; claude is then the one on PATH (claude is empty).
func wrapperInvocation(args []string) (claude string, rest []string, ok bool) {
	if len(args) == 0 {
		return "", nil, false
	}
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(args[0])), ".exe")
	switch {
	case filepath.IsAbs(args[0]) && name == "claude":
		return args[0], args[1:], true
	case args[0] == "--output-format":
		return "", args, true
	}
	return "", nil, false
}

// runWrapper runs claude for the extension in the active profile. A conversation moves to the next
// profile at a limit; other calls of claude only get the profile's login. Without a profile, claude
// runs as if switchyard were not there.
func runWrapper(ctx context.Context, claude string, args []string) error {
	if claude == "" {
		path, err := exec.LookPath("claude")
		if err != nil {
			return errors.New("claude executable not found in PATH")
		}
		claude = path
	}
	root, err := profilesRoot()
	if err != nil {
		return err
	}
	m := &profiles.Manager{Root: root, Claude: claude, Environ: os.Environ()}
	store, err := newStateStore()
	if err != nil {
		return err
	}
	p, err := resolveProfile(m, store, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "switchyard: %v; running claude without a profile\n", err)
		return runUnchanged(ctx, claude, args)
	}
	l := launcher.Launcher{Claude: claude, Environ: os.Environ(), Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, StopGrace: stopGrace}
	var code int
	if sdkproxy.SpeaksSDK(args) {
		code, err = proxySDK(ctx, m, store, l, p, args)
	} else {
		code, err = l.Run(ctx, p, args)
	}
	if err != nil {
		return err
	}
	if code != 0 {
		return exitError{code: code}
	}
	return nil
}

func proxySDK(ctx context.Context, m *profiles.Manager, store *state.Store, l launcher.Launcher, p profiles.Profile, args []string) (int, error) {
	list, err := m.List()
	if err != nil {
		return 0, err
	}
	proxy := &sdkproxy.Proxy{
		Launcher: l,
		Store:    store,
		Profiles: list,
		Config:   loadConfig,
		Prepare: func(p profiles.Profile) error {
			if err := markUsed(store, p); err != nil {
				return err
			}
			relinkShared(p)
			return nil
		},
		Now:     time.Now,
		History: recordSwitch(),
		Notify:  sendNotification,
		Notice:  os.Stderr,
	}
	return proxy.Run(ctx, p, args)
}

// runUnchanged runs claude with the environment switchyard was given.
func runUnchanged(ctx context.Context, claude string, args []string) error {
	cmd := exec.CommandContext(ctx, claude, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exitError{code: exit.ExitCode()}
	}
	return err
}
