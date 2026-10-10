// Package notify shows a desktop notification with what the system already has: a toast
// through Windows PowerShell, notify-send on Linux and osascript on macOS. It adds no
// dependency and is best effort: a missing tool is an error the caller may ignore.
package notify

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

// ErrUnsupported is returned when the system has no tool to show a notification.
var ErrUnsupported = errors.New("no way to show a desktop notification here")

const (
	titleVar = "SWITCHYARD_NOTIFY_TITLE"
	bodyVar  = "SWITCHYARD_NOTIFY_BODY"
	timeout  = 10 * time.Second
)

// windowsScript shows a toast under PowerShell's own application id, which every Windows
// has registered, so no shortcut or package is needed. The text comes from the environment
// and is never part of the script.
const windowsScript = `$ErrorActionPreference = 'Stop'
[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$text = $xml.GetElementsByTagName('text')
[void]$text.Item(0).AppendChild($xml.CreateTextNode($env:` + titleVar + `))
[void]$text.Item(1).AppendChild($xml.CreateTextNode($env:` + bodyVar + `))
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show($toast)`

// macScript reads the text from the environment, so quotes in it cannot break the script.
const macScript = `display notification (system attribute "` + bodyVar + `") with title (system attribute "` + titleVar + `")`

// invocation is the command that shows a notification.
type invocation struct {
	name string
	args []string
	env  []string
}

// command builds the invocation for goos, or reports that there is none.
func command(goos, title, body string) (invocation, bool) {
	env := []string{titleVar + "=" + title, bodyVar + "=" + body}
	switch goos {
	case "windows":
		return invocation{"powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", windowsScript}, env}, true
	case "linux":
		return invocation{"notify-send", []string{"--app-name=switchyard", "--", title, body}, nil}, true
	case "darwin":
		return invocation{"osascript", []string{"-e", macScript}, env}, true
	}
	return invocation{}, false
}

// Send shows a notification and waits for the tool, for a few seconds at most.
func Send(ctx context.Context, title, body string) error {
	inv, ok := command(runtime.GOOS, title, body)
	if !ok {
		return ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, inv.name, inv.args...)
	cmd.Env = append(cmd.Environ(), inv.env...)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
