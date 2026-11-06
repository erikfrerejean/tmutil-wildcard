package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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

func TestFindClones(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "Customers", "acme", "app", ".git"))
	writeFile(t, filepath.Join(root, "Projects", "worktree", ".git"))
	mkdirAll(t, filepath.Join(root, "Projects", "worktree", "vendor", "nested", ".git"))
	mkdirAll(t, filepath.Join(root, "Projects", "plain-folder", "src"))
	writeFile(t, filepath.Join(root, "notes.md"))

	got, err := findClones(root, discardLog)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(root, "Customers", "acme", "app"),
		filepath.Join(root, "Projects", "worktree"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("findClones() = %v, want %v", got, want)
	}
}

func TestFindClonesTreatsRootItselfAsClone(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, ".git"))
	mkdirAll(t, filepath.Join(root, "sub", ".git"))

	got, err := findClones(root, discardLog)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{root}; !slices.Equal(got, want) {
		t.Errorf("findClones() = %v, want %v", got, want)
	}
}

func TestFindClonesFailsWhenRootIsMissing(t *testing.T) {
	_, err := findClones(filepath.Join(t.TempDir(), "missing"), discardLog)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("findClones() error = %v, want ErrNotExist", err)
	}
}

func TestFindClonesSkipsUnreadableDirectory(t *testing.T) {
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
	got, err := findClones(root, func(format string, args ...any) {
		logged = append(logged, format)
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(root, "open")}; !slices.Equal(got, want) {
		t.Errorf("findClones() = %v, want %v", got, want)
	}
	if len(logged) != 1 || !strings.HasPrefix(logged[0], "skipping") {
		t.Errorf("logged = %v, want one skipping line", logged)
	}
}

// fakeTmutil answers isexcluded from state and records every addexclusion.
type fakeTmutil struct {
	excluded map[string]bool
	failing  map[string]bool
	added    []string
}

func (f *fakeTmutil) run(args ...string) ([]byte, error) {
	cmd, path := args[0], args[1]
	switch cmd {
	case "isexcluded":
		if f.excluded[path] {
			return []byte("[Excluded]  " + path + "\n"), nil
		}
		return []byte("[Included]  " + path + "\n"), nil
	case "addexclusion":
		if f.failing[path] {
			return []byte(path + ": Error (100002) while attempting to change exclusion setting.\n"), errors.New("exit status 162")
		}
		f.added = append(f.added, path)
		return nil, nil
	}
	return nil, errors.New("unexpected tmutil verb " + cmd)
}

func TestExcludeClonesAddsOnlyMissingExclusions(t *testing.T) {
	fake := &fakeTmutil{excluded: map[string]bool{"/r/done": true}}

	var logged []string
	err := excludeClones([]string{"/r/done", "/r/new"}, fake.run, func(format string, args ...any) {
		logged = append(logged, args[0].(string))
	})
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
	fake := &fakeTmutil{failing: map[string]bool{"/r/bad": true}}

	err := excludeClones([]string{"/r/bad", "/r/good"}, fake.run, discardLog)
	if err == nil || !strings.Contains(err.Error(), "tmutil addexclusion /r/bad") {
		t.Errorf("error = %v, want addexclusion failure for /r/bad", err)
	}
	if want := []string{"/r/good"}; !slices.Equal(fake.added, want) {
		t.Errorf("addexclusion calls = %v, want %v", fake.added, want)
	}
}
