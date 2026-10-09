package detector

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func scan(t *testing.T, chunks ...string) Result {
	t.Helper()
	var d Detector
	for _, c := range chunks {
		if _, err := d.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	return d.Result()
}

func TestLimitSignals(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"rejected rate limit event", `{"type":"rate_limit_event","rate_limit_info":{"status":"rejected"}}`, true},
		{"allowed rate limit event", `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`, false},
		{"allowed warning", `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning"}}`, false},
		{"api retry rate limit", `{"type":"system","subtype":"api_retry","error":"rate_limit"}`, true},
		{"api retry other error", `{"type":"system","subtype":"api_retry","error":"server_error"}`, false},
		{"error result with limit text", `{"type":"result","is_error":true,"result":"You've hit your session limit · resets 3:45pm"}`, true},
		{"successful result mentioning a limit", `{"type":"result","is_error":false,"result":"A usage limit reached message looks like this"}`, false},
		{"assistant text mentioning a limit", `{"type":"assistant","message":"you hit your limit"}`, false},
		{"plain text", "You've hit your weekly limit · resets Mon 9:00am", true},
		{"plain text, unrelated", "done", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scan(t, tt.output+"\n").Limited; got != tt.want {
				t.Errorf("Limited = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLineSplitAcrossWrites(t *testing.T) {
	r := scan(t, `{"type":"system","subtype":"api_`, `retry","error":"rate_limit"}`)
	if !r.Limited {
		t.Error("unterminated last line assembled from two writes was not inspected")
	}
}

func TestSessionAndWindows(t *testing.T) {
	reset := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	r := scan(t,
		`{"type":"system","subtype":"init","session_id":"abc"}`+"\n",
		`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","unifiedWindows":{`+
			`"five_hour":{"utilization":0.42,"resetsAt":`+strconv.FormatInt(reset.Unix(), 10)+`},`+
			`"seven_day":{"utilization":1,"resetsAt":`+strconv.FormatInt(reset.Unix(), 10)+`}}}}`+"\n")
	if r.SessionID != "abc" {
		t.Errorf("SessionID = %q", r.SessionID)
	}
	if r.FiveHour == nil || r.FiveHour.UsedPercent != 42 || !r.FiveHour.ResetsAt.Equal(reset) {
		t.Errorf("FiveHour = %+v", r.FiveHour)
	}
	if r.SevenDay == nil || r.SevenDay.UsedPercent != 100 {
		t.Errorf("SevenDay = %+v", r.SevenDay)
	}
	if r.Limited {
		t.Error("an allowed event must not count as a limit")
	}
}

func TestOverlongLineIsSkipped(t *testing.T) {
	long := strings.Repeat("x", maxLine+10)
	r := scan(t, long+"\n", "You've hit your session limit\n")
	if !r.Limited {
		t.Error("the line after an overlong line was not inspected")
	}
}
