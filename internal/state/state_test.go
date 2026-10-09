package state

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	helperEnv   = "SWITCHYARD_STATE_HELPER"
	helperCount = 5
	helperProcs = 4
)

func TestMain(m *testing.M) {
	if path := os.Getenv(helperEnv); path != "" {
		os.Exit(runHelper(path))
	}
	os.Exit(m.Run())
}

// runHelper adds helperCount unique profiles; it runs in child processes.
func runHelper(path string) int {
	store := NewStore(path)
	for i := range helperCount {
		key := fmt.Sprintf("p%d-%d", os.Getpid(), i)
		err := store.Update(func(st *State) error {
			st.Profiles[key] = Profile{}
			return nil
		})
		if err != nil {
			return 1
		}
	}
	return 0
}

func TestReadMissingFileIsEmpty(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "state.json")).Read()
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != SchemaVersion || len(st.Profiles) != 0 {
		t.Errorf("unexpected state: %+v", st)
	}
}

func TestUpdatePersistsChanges(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sub", "state.json"))
	until := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	err := store.Update(func(st *State) error {
		st.Active = "acc1"
		st.Profiles["acc1"] = Profile{CooldownUntil: until}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Active != "acc1" || !got.Profiles["acc1"].CooldownUntil.Equal(until) {
		t.Errorf("state not persisted: %+v", got)
	}
}

func TestUpdateErrorLeavesFileUntouched(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	boom := errors.New("boom")
	err := store.Update(func(st *State) error {
		st.Active = "changed"
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
	if got, _ := store.Read(); got.Active != "" {
		t.Errorf("failed update was saved: %+v", got)
	}
}

func TestReadRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(path).Read(); err == nil {
		t.Error("expected an error for a newer schema version")
	}
}

func TestUpdateIsSafeAcrossGoroutines(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	const workers = 8
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.Update(func(st *State) error {
				st.Profiles[fmt.Sprintf("g%d", i)] = Profile{}
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	assertProfileCount(t, store, workers)
}

func TestUpdateIsSafeAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmds := make([]*exec.Cmd, helperProcs)
	for i := range cmds {
		cmds[i] = exec.Command(exe)
		cmds[i].Env = append(os.Environ(), helperEnv+"="+path)
		if err := cmds[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper failed: %v", err)
		}
	}
	assertProfileCount(t, NewStore(path), helperProcs*helperCount)
}

// assertProfileCount fails if updates were lost, i.e. fewer profiles exist than were added.
func assertProfileCount(t *testing.T, store *Store, want int) {
	t.Helper()
	st, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(st.Profiles); got != want {
		t.Errorf("lost updates: got %d profiles, want %d", got, want)
	}
}
