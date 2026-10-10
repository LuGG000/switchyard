// Package breaker limits automatic profile switches: a minimum time between two of
// them and a maximum per 24 hours. It keeps two nearly empty accounts from
// alternating every few minutes and limits what runs unattended. It reduces a
// switching pattern that looks like working around limits; it guarantees nothing.
//
// Only switches switchyard decides alone count. A switch the user chose (an answer to
// the question, a button in the mod, handoff) is not recorded and never blocked.
package breaker

import (
	"fmt"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/state"
)

// Window is the period the daily maximum is counted over.
const Window = 24 * time.Hour

// Limits are the two limits; zero turns the matching one off.
type Limits struct {
	// MinInterval is the time that must pass after an automatic switch before the next.
	MinInterval time.Duration
	// MaxPerDay is the most automatic switches in Window.
	MaxPerDay int
}

// TrippedError is returned when an automatic switch is not allowed right now.
type TrippedError struct {
	// Reason says which limit applies.
	Reason string
	// Until is when an automatic switch is allowed again.
	Until time.Time
}

func (e *TrippedError) Error() string {
	return fmt.Sprintf("%s; allowed again at %s", e.Reason, e.Until.Local().Format("15:04"))
}

// Check returns a *TrippedError if another automatic switch at now would break a
// limit, judging by the switches recorded in st.
func (l Limits) Check(st state.State, now time.Time) error {
	var recent []time.Time
	for _, at := range st.AutoSwitches {
		if now.Sub(at) < Window {
			recent = append(recent, at)
		}
	}
	var tripped *TrippedError
	if l.MaxPerDay > 0 && len(recent) >= l.MaxPerDay {
		// The oldest switch that has to leave the window makes room for one more.
		oldest := recent[len(recent)-l.MaxPerDay]
		tripped = &TrippedError{
			Reason: fmt.Sprintf("%d automatic switches in the last 24 hours (maximum %d)", len(recent), l.MaxPerDay),
			Until:  oldest.Add(Window),
		}
	}
	if l.MinInterval > 0 && len(recent) > 0 {
		last := recent[len(recent)-1]
		if until := last.Add(l.MinInterval); until.After(now) && (tripped == nil || until.After(tripped.Until)) {
			tripped = &TrippedError{
				Reason: fmt.Sprintf("the last automatic switch was %d minutes ago (minimum %d)", int(now.Sub(last).Minutes()), int(l.MinInterval.Minutes())),
				Until:  until,
			}
		}
	}
	if tripped == nil {
		return nil
	}
	return tripped
}

// Record notes an automatic switch at now and forgets those older than Window.
func Record(store *state.Store, now time.Time) error {
	return store.Update(func(st *state.State) error {
		kept := st.AutoSwitches[:0]
		for _, at := range st.AutoSwitches {
			if now.Sub(at) < Window {
				kept = append(kept, at)
			}
		}
		st.AutoSwitches = append(kept, now.UTC())
		return nil
	})
}

// Allowed is Check on the switches stored in store.
func Allowed(store *state.Store, l Limits, now time.Time) error {
	st, err := store.Read()
	if err != nil {
		return err
	}
	return l.Check(st, now)
}

// FromConfig takes the limits from the config.
func FromConfig(c config.Config) Limits {
	return Limits{MinInterval: time.Duration(c.MinSwitchInterval) * time.Minute, MaxPerDay: c.MaxAutoSwitches}
}
