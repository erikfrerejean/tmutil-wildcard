package main

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixture builds a root with one excluded clone, one pending clone and one
// stale folder, and a fake tmutil that knows which clone is excluded.
func fixture(t *testing.T) (root string, fake *fakeTmutil) {
	t.Helper()
	root = t.TempDir()
	mkdirAll(t, filepath.Join(root, "done", ".git"))
	mkdirAll(t, filepath.Join(root, "new", ".git"))
	markExcluded(t, filepath.Join(root, "old"))
	fake = &fakeTmutil{excluded: map[string]bool{filepath.Join(root, "done"): true}}
	return root, fake
}

func TestRunRejectsBadUsage(t *testing.T) {
	for name, args := range map[string][]string{
		"no arguments":    nil,
		"no root":         {"list"},
		"unknown command": {"bogus", "/r"},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(args, &bytes.Buffer{}, &stderr, nil); code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if !strings.HasPrefix(stderr.String(), "usage:") {
				t.Errorf("stderr = %q, want usage", stderr.String())
			}
		})
	}
}

func TestRunListPrintsCloneRoots(t *testing.T) {
	root, fake := fixture(t)

	var stdout bytes.Buffer
	if code := run([]string{"list", root}, &stdout, &bytes.Buffer{}, fake.run); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	want := filepath.Join(root, "done") + "\n" + filepath.Join(root, "new") + "\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunExcludedPrintsStatusPerFolder(t *testing.T) {
	root, fake := fixture(t)

	var stdout bytes.Buffer
	if code := run([]string{"excluded", root}, &stdout, &bytes.Buffer{}, fake.run); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	want := "excluded  " + filepath.Join(root, "done") + "\n" +
		"pending   " + filepath.Join(root, "new") + "\n" +
		"stale     " + filepath.Join(root, "old") + "\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunExcludeReconciles(t *testing.T) {
	root, fake := fixture(t)

	var stderr bytes.Buffer
	if code := run([]string{"exclude", root}, &bytes.Buffer{}, &stderr, fake.run); code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, stderr.String())
	}
	if want := []string{filepath.Join(root, "new")}; !slices.Equal(fake.added, want) {
		t.Errorf("addexclusion calls = %v, want %v", fake.added, want)
	}
	if want := []string{filepath.Join(root, "old")}; !slices.Equal(fake.removed, want) {
		t.Errorf("removeexclusion calls = %v, want %v", fake.removed, want)
	}
}

func TestRunReportsFailuresAndContinues(t *testing.T) {
	root, fake := fixture(t)
	fake.failing = map[string]bool{filepath.Join(root, "old"): true}
	missing := filepath.Join(root, "missing")

	var stderr bytes.Buffer
	if code := run([]string{"exclude", missing, root}, &bytes.Buffer{}, &stderr, fake.run); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if want := []string{filepath.Join(root, "new")}; !slices.Equal(fake.added, want) {
		t.Errorf("addexclusion calls = %v, want %v", fake.added, want)
	}
	for _, want := range []string{missing, "tmutil removeexclusion " + filepath.Join(root, "old")} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to mention %q", stderr.String(), want)
		}
	}
}
