package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func discardLog(string, ...any) {}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// markExcluded sets the xattr tmutil would set, without calling tmutil.
func markExcluded(t *testing.T, path string) {
	t.Helper()
	mkdirAll(t, path)
	if err := unix.Setxattr(path, excludeAttr, []byte("com.apple.backupd"), 0); err != nil {
		t.Fatal(err)
	}
}

func TestScanRootClassifiesDirectories(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "Customers", "acme", "app", ".git"))
	writeFile(t, filepath.Join(root, "Projects", "worktree", ".git"))
	mkdirAll(t, filepath.Join(root, "Projects", "worktree", "vendor", "nested", ".git"))
	mkdirAll(t, filepath.Join(root, "Projects", "plain-folder", "src"))
	writeFile(t, filepath.Join(root, "notes.md"))
	markExcluded(t, filepath.Join(root, "Customers", "acme", "app"))
	markExcluded(t, filepath.Join(root, "Customers", "gone", "ex-clone"))
	markExcluded(t, filepath.Join(root, "DevOps", "ex-monorepo"))
	mkdirAll(t, filepath.Join(root, "DevOps", "ex-monorepo", "service", ".git"))

	got, err := scanRoot(root, discardLog)
	if err != nil {
		t.Fatal(err)
	}

	wantClones := []string{
		filepath.Join(root, "Customers", "acme", "app"),
		filepath.Join(root, "DevOps", "ex-monorepo", "service"),
		filepath.Join(root, "Projects", "worktree"),
	}
	if !slices.Equal(got.clones, wantClones) {
		t.Errorf("clones = %v, want %v", got.clones, wantClones)
	}
	wantStale := []string{
		filepath.Join(root, "Customers", "gone", "ex-clone"),
		filepath.Join(root, "DevOps", "ex-monorepo"),
	}
	if !slices.Equal(got.stale, wantStale) {
		t.Errorf("stale = %v, want %v", got.stale, wantStale)
	}
}

func TestScanRootTreatsRootItselfAsClone(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, ".git"))
	mkdirAll(t, filepath.Join(root, "sub", ".git"))

	got, err := scanRoot(root, discardLog)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{root}; !slices.Equal(got.clones, want) {
		t.Errorf("clones = %v, want %v", got.clones, want)
	}
}

func TestScanRootNeverReportsRootAsStale(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	markExcluded(t, root)
	mkdirAll(t, filepath.Join(root, "plain"))

	got, err := scanRoot(root, discardLog)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.stale) != 0 {
		t.Errorf("stale = %v, want none", got.stale)
	}
}

func TestScanRootFailsWhenRootIsMissing(t *testing.T) {
	_, err := scanRoot(filepath.Join(t.TempDir(), "missing"), discardLog)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("scanRoot() error = %v, want ErrNotExist", err)
	}
}

func TestScanRootSkipsUnreadableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	mkdirAll(t, filepath.Join(locked, "repo", ".git"))
	mkdirAll(t, filepath.Join(root, "open", ".git"))
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	var logged []string
	got, err := scanRoot(root, func(format string, args ...any) {
		logged = append(logged, fmt.Sprintf(format, args...))
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(root, "open")}; !slices.Equal(got.clones, want) {
		t.Errorf("clones = %v, want %v", got.clones, want)
	}
	// The xattr read and the directory listing fail separately.
	if len(logged) != 2 {
		t.Fatalf("logged = %v, want two skipping lines", logged)
	}
	for _, line := range logged {
		if !strings.HasPrefix(line, "skipping "+locked) {
			t.Errorf("logged %q, want it to skip %s", line, locked)
		}
	}
}

// fakeTmutil answers isexcluded from state and records every change.
type fakeTmutil struct {
	excluded      map[string]bool
	failing       map[string]bool
	statusFailing map[string]bool
	added         []string
	removed       []string
}

func (f *fakeTmutil) run(args ...string) ([]byte, error) {
	cmd, path := args[0], args[1]
	switch cmd {
	case "isexcluded":
		if f.statusFailing[path] {
			return []byte("tmutil: unexpected failure\n"), errors.New("exit status 1")
		}
		if f.excluded[path] {
			return []byte("[Excluded]  " + path + "\n"), nil
		}
		return []byte("[Included]  " + path + "\n"), nil
	case "addexclusion", "removeexclusion":
		if f.failing[path] {
			return []byte(path + ": Error (100002) while attempting to change exclusion setting.\n"), errors.New("exit status 162")
		}
		if cmd == "addexclusion" {
			f.added = append(f.added, path)
		} else {
			f.removed = append(f.removed, path)
		}
		return nil, nil
	}
	return nil, errors.New("unexpected tmutil verb " + cmd)
}

func collectLog(logged *[]string) logger {
	return func(format string, args ...any) {
		*logged = append(*logged, args[0].(string))
	}
}

func TestExcludeClonesAddsOnlyMissingExclusions(t *testing.T) {
	fake := &fakeTmutil{excluded: map[string]bool{"/r/done": true}}

	var logged []string
	err := excludeClones([]string{"/r/done", "/r/new"}, fake.run, collectLog(&logged))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/r/new"}; !slices.Equal(fake.added, want) {
		t.Errorf("addexclusion calls = %v, want %v", fake.added, want)
	}
	if want := []string{"/r/new"}; !slices.Equal(logged, want) {
		t.Errorf("logged = %v, want %v", logged, want)
	}
}

func TestExcludeClonesContinuesAfterFailure(t *testing.T) {
	fake := &fakeTmutil{
		failing:       map[string]bool{"/r/bad": true},
		statusFailing: map[string]bool{"/r/unknown": true},
	}

	err := excludeClones([]string{"/r/bad", "/r/unknown", "/r/good"}, fake.run, discardLog)
	for _, want := range []string{"tmutil addexclusion /r/bad", "tmutil isexcluded /r/unknown"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	if want := []string{"/r/good"}; !slices.Equal(fake.added, want) {
		t.Errorf("addexclusion calls = %v, want %v", fake.added, want)
	}
}

func TestRemoveStaleContinuesAfterFailure(t *testing.T) {
	fake := &fakeTmutil{failing: map[string]bool{"/r/bad": true}}

	var logged []string
	err := removeStale([]string{"/r/bad", "/r/old"}, fake.run, collectLog(&logged))
	if err == nil || !strings.Contains(err.Error(), "tmutil removeexclusion /r/bad") {
		t.Errorf("error = %v, want removeexclusion failure for /r/bad", err)
	}
	if want := []string{"/r/old"}; !slices.Equal(fake.removed, want) {
		t.Errorf("removeexclusion calls = %v, want %v", fake.removed, want)
	}
	if want := []string{"/r/old"}; !slices.Equal(logged, want) {
		t.Errorf("logged = %v, want %v", logged, want)
	}
}

func TestStatusesReportsExcludedPendingAndStale(t *testing.T) {
	fake := &fakeTmutil{excluded: map[string]bool{"/r/done": true}}

	got, err := statuses(scan{clones: []string{"/r/done", "/r/new"}, stale: []string{"/r/old"}}, fake.run)
	if err != nil {
		t.Fatal(err)
	}
	want := []status{{"excluded", "/r/done"}, {"pending", "/r/new"}, {"stale", "/r/old"}}
	if !slices.Equal(got, want) {
		t.Errorf("statuses() = %v, want %v", got, want)
	}
	if len(fake.added)+len(fake.removed) != 0 {
		t.Errorf("statuses() changed exclusions: added %v, removed %v", fake.added, fake.removed)
	}
}

func TestStatusesKeepsResultsAfterStatusFailure(t *testing.T) {
	fake := &fakeTmutil{excluded: map[string]bool{"/r/done": true}, statusFailing: map[string]bool{"/r/bad": true}}

	got, err := statuses(scan{clones: []string{"/r/bad", "/r/done"}}, fake.run)
	if err == nil || !strings.Contains(err.Error(), "tmutil isexcluded /r/bad") {
		t.Errorf("error = %v, want isexcluded failure for /r/bad", err)
	}
	if want := []status{{"excluded", "/r/done"}}; !slices.Equal(got, want) {
		t.Errorf("statuses() = %v, want %v", got, want)
	}
}
