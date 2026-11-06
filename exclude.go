package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// runner executes tmutil with the given arguments and returns its combined output.
type runner func(args ...string) ([]byte, error)

type logger func(format string, args ...any)

// findClones walks root and returns every directory that holds a `.git` entry.
// A `.git` file (worktree or submodule checkout) counts as well as a `.git`
// directory. Clone contents are not descended into, so nested repositories
// inside a clone are covered by their parent's exclusion. Symlinks are not
// followed. Unreadable directories are logged and skipped.
func findClones(root string, logf logger) ([]string, error) {
	var clones []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			logf("skipping %s: %v", path, err)
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
			clones = append(clones, path)
			return fs.SkipDir
		}
		return nil
	})
	return clones, err
}

// excludeClones adds a Time Machine exclusion for every clone that does not
// have one yet. Failures are collected so one bad path does not stop the run.
func excludeClones(clones []string, run runner, logf logger) error {
	var failures []error
	for _, clone := range clones {
		excluded, err := isExcluded(clone, run)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if excluded {
			continue
		}
		if out, err := run("addexclusion", clone); err != nil {
			failures = append(failures, fmt.Errorf("tmutil addexclusion %s: %w: %s", clone, err, bytes.TrimSpace(out)))
			continue
		}
		logf("excluded %s", clone)
	}
	return errors.Join(failures...)
}

// isExcluded parses `tmutil isexcluded`, which exits 0 for every outcome and
// reports the state as a bracketed prefix: [Excluded], [Included] or [UNKNOWN].
func isExcluded(path string, run runner) (bool, error) {
	out, err := run("isexcluded", path)
	if err != nil {
		return false, fmt.Errorf("tmutil isexcluded %s: %w: %s", path, err, bytes.TrimSpace(out))
	}
	return bytes.HasPrefix(out, []byte("[Excluded]")), nil
}
