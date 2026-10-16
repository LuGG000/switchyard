package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// sdkSession answers an SDK client (--input-format stream-json) the way claude does, until the
// input ends: every control request gets a success response, every user message one turn. The
// result of a turn names the config dir and the arguments, so tests see which profile answered. In
// a profile that matches FAKE_LIMIT_DIR every turn ends at a simulated limit.
func sdkSession(args []string) int {
	session := sessionOf(args)
	out := json.NewEncoder(os.Stdout)
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for in.Scan() {
		var msg struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
			Request   struct {
				Subtype string `json:"subtype"`
			} `json:"request"`
		}
		if json.Unmarshal(in.Bytes(), &msg) != nil {
			continue
		}
		switch msg.Type {
		case "control_request":
			_ = out.Encode(map[string]any{"type": "control_response", "response": map[string]any{
				"subtype": "success", "request_id": msg.RequestID,
				"response": map[string]any{"handled": msg.Request.Subtype, "configDir": os.Getenv("CLAUDE_CONFIG_DIR")},
			}})
		case "user":
			_ = out.Encode(map[string]any{"type": "system", "subtype": "init", "session_id": session})
			resets := time.Now().Add(time.Hour).Unix()
			if matchesDir("FAKE_LIMIT_DIR") {
				fmt.Printf(`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","unifiedWindows":{"five_hour":{"utilization":1,"resetsAt":%d}}},"session_id":%q}`+"\n", resets, session)
				_ = out.Encode(map[string]any{"type": "result", "subtype": "success", "is_error": true, "session_id": session, "result": "You've hit your session limit · resets 3:45pm"})
				continue
			}
			fmt.Printf(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","unifiedWindows":{"five_hour":{"utilization":0.25,"resetsAt":%d}}},"session_id":%q}`+"\n", resets, session)
			_ = out.Encode(map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": session,
				"result": Report{Args: args, ConfigDir: os.Getenv("CLAUDE_CONFIG_DIR")}})
		}
	}
	return 0
}

// sessionOf returns the conversation the arguments resume or name, else a fixed one.
func sessionOf(args []string) string {
	for i, a := range args {
		switch {
		case (a == "--resume" || a == "--session-id") && i+1 < len(args):
			return args[i+1]
		case strings.HasPrefix(a, "--resume="):
			return strings.TrimPrefix(a, "--resume=")
		case strings.HasPrefix(a, "--session-id="):
			return strings.TrimPrefix(a, "--session-id=")
		}
	}
	return "fake-session"
}
