// Package linker shares entries of the default claude config dir (sessions,
// settings, skills, ...) with profile directories through links.
//
// A link is only ever created where nothing exists or where an empty directory
// or a dangling link sits. Real content is never overwritten or deleted.
package linker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Outcome describes what happened to one shared entry.
type Outcome string

// Possible outcomes of linking one entry.
const (
	Linked        Outcome = "linked"
	Unchanged     Outcome = "unchanged"
	Relinked      Outcome = "relinked"
	SourceMissing Outcome = "source missing"
	Conflict      Outcome = "conflict"
	// Missing and Broken are only reported by Check.
	Missing Outcome = "missing"
	Broken  Outcome = "broken"
)

// ErrConflict is returned when an entry exists in a profile and is not the shared one.
var ErrConflict = errors.New("link conflict")

// Result reports the outcome for one entry.
type Result struct {
	Name    string
	Outcome Outcome
	Detail  string
}

// Link shares each of names from source into profileDir. It processes every
// entry and returns all results together with the joined errors, if any.
// Conflicts are reported as results and wrapped ErrConflict.
func Link(source, profileDir string, names []string) ([]Result, error) {
	var (
		results []Result
		errs    []error
	)
	for _, name := range names {
		res, err := linkOne(source, profileDir, name)
		results = append(results, res)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	return results, errors.Join(errs...)
}

func linkOne(source, profileDir, name string) (Result, error) {
	if !validEntry(name) {
		return Result{Name: name, Outcome: Conflict}, errors.New("invalid entry name")
	}
	src := filepath.Join(source, name)
	dst := filepath.Join(profileDir, name)

	srcInfo, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{Name: name, Outcome: SourceMissing}, nil
	}
	if err != nil {
		return Result{Name: name}, fmt.Errorf("inspect source: %w", err)
	}

	outcome := Linked
	if dstLinfo, err := os.Lstat(dst); err == nil {
		switch {
		case sameEntry(dst, srcInfo):
			return Result{Name: name, Outcome: Unchanged}, nil
		case isDangling(dst, dstLinfo):
			if err := os.Remove(dst); err != nil {
				return Result{Name: name}, fmt.Errorf("remove dangling link: %w", err)
			}
			outcome = Relinked
		case isEmptyDir(dst, dstLinfo):
			if err := os.Remove(dst); err != nil {
				return Result{Name: name}, fmt.Errorf("remove empty directory: %w", err)
			}
		default:
			return Result{Name: name, Outcome: Conflict, Detail: "exists and is not the shared entry"}, ErrConflict
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Result{Name: name}, fmt.Errorf("inspect target: %w", err)
	}

	if err := createLink(src, dst, srcInfo.IsDir()); err != nil {
		return Result{Name: name}, fmt.Errorf("create link: %w", err)
	}
	return Result{Name: name, Outcome: outcome}, nil
}

// validEntry accepts plain names only, so a config value cannot escape the profile dir.
func validEntry(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// sameEntry reports whether dst resolves to the same file or directory as the source.
func sameEntry(dst string, srcInfo fs.FileInfo) bool {
	info, err := os.Stat(dst)
	return err == nil && os.SameFile(info, srcInfo)
}

// isDangling reports whether dst is a link whose target no longer exists.
func isDangling(dst string, linfo fs.FileInfo) bool {
	if linfo.Mode()&(fs.ModeSymlink|fs.ModeIrregular) == 0 {
		return false
	}
	_, err := os.Stat(dst)
	return errors.Is(err, fs.ErrNotExist)
}

func isEmptyDir(dst string, linfo fs.FileInfo) bool {
	if !linfo.IsDir() {
		return false
	}
	entries, err := os.ReadDir(dst)
	return err == nil && len(entries) == 0
}

// Check reports the state of each shared entry without changing anything.
func Check(source, profileDir string, names []string) []Result {
	results := make([]Result, 0, len(names))
	for _, name := range names {
		results = append(results, checkOne(source, profileDir, name))
	}
	return results
}

func checkOne(source, profileDir, name string) Result {
	if !validEntry(name) {
		return Result{Name: name, Outcome: Conflict, Detail: "invalid entry name"}
	}
	srcInfo, err := os.Stat(filepath.Join(source, name))
	if err != nil {
		return Result{Name: name, Outcome: SourceMissing}
	}
	dst := filepath.Join(profileDir, name)
	linfo, err := os.Lstat(dst)
	switch {
	case err != nil:
		return Result{Name: name, Outcome: Missing}
	case sameEntry(dst, srcInfo):
		return Result{Name: name, Outcome: Unchanged}
	case isDangling(dst, linfo):
		return Result{Name: name, Outcome: Broken, Detail: "link target is gone"}
	}
	return Result{Name: name, Outcome: Conflict, Detail: "exists and is not the shared entry"}
}
