package notify

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

const hostile = `it's "quoted" $(rm -rf /) ` + "`x`" + ` & <b>`

func TestCommandKeepsTheTextOutOfTheScripts(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			inv, ok := command(goos, "title "+hostile, "body "+hostile)
			if !ok {
				t.Fatal("no command")
			}
			for _, arg := range inv.args {
				if goos != "linux" && strings.Contains(arg, "quoted") {
					t.Errorf("the text is part of the script: %q", arg)
				}
			}
			switch goos {
			case "linux":
				if !slices.Contains(inv.args, "--") || inv.args[len(inv.args)-2] != "title "+hostile || inv.args[len(inv.args)-1] != "body "+hostile {
					t.Errorf("args = %q, want the text after --", inv.args)
				}
			default:
				if !slices.Contains(inv.env, titleVar+"=title "+hostile) || !slices.Contains(inv.env, bodyVar+"=body "+hostile) {
					t.Errorf("env = %q, want the text in the environment", inv.env)
				}
			}
		})
	}
}

func TestCommandUsesTheSystemsOwnTools(t *testing.T) {
	want := map[string]string{"windows": "powershell.exe", "linux": "notify-send", "darwin": "osascript"}
	for goos, name := range want {
		if inv, ok := command(goos, "t", "b"); !ok || inv.name != name {
			t.Errorf("%s: %q, %v, want %q", goos, inv.name, ok, name)
		}
	}
	if _, ok := command("plan9", "t", "b"); ok {
		t.Error("an unknown system got a command")
	}
}

// TestShowARealNotification shows a real notification; it is off unless asked for, because it
// pops up on the screen: SWITCHYARD_NOTIFY_REAL=1 go test ./internal/notify -run Real -v
func TestShowARealNotification(t *testing.T) {
	if os.Getenv("SWITCHYARD_NOTIFY_REAL") == "" {
		t.Skip("set SWITCHYARD_NOTIFY_REAL=1 to show a notification")
	}
	if err := Send(context.Background(), "switchyard", "A test notification: every profile is at its limit; waiting until 14:00 (it's \"quoted\" & <fine>)"); err != nil {
		t.Fatal(err)
	}
}
