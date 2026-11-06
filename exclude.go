package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// excludeAttr is the xattr `tmutil addexclusion` (without -p) sets on a folder.
const excludeAttr = "com.apple.metadata:com_apple_backup_excludeItem"

// runner executes tmutil with the given arguments and returns its combined output.
type runner func(args ...string) ([]byte, error)

type logger func(format string, args ...any)

// scan is the classification of the directories below one root.
type scan struct {
	// clones hold a `.git` entry: a directory for a clone, a file for a
	// worktree or submodule checkout.
	clones []string
	// stale carry the exclusion xattr but are not clone roots, typically a
	// folder whose `.git` was removed after it had been excluded.
	stale []string
}

// scanRoot walks root. Clone contents are not descended into, so nested
// repositories inside a clone are covered by their parent's exclusion. Stale
// directories are descended into, since a clone may live inside one. The root
// itself is never reported as stale. Symlinks are not followed. Unreadable
// directories are logged and skipped.
func scanRoot(root string, logf logger) (scan, error) {
	var s scan
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
			s.clones = append(s.clones, path)
			return fs.SkipDir
		}
		if path == root {
			return nil
		}
		excluded, err := hasExcludeAttr(path)
		if err != nil {
			logf("skipping %s: %v", path, err)
			return nil
		}
		if excluded {
			s.stale = append(s.stale, path)
		}
		return nil
	})
	return s, err
}

func hasExcludeAttr(path string) (bool, error) {
	_, err := unix.Getxattr(path, excludeAttr, nil)
	if errors.Is(err, unix.ENOATTR) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("getxattr %s: %w", path, err)
	}
	return true, nil
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
		if _, err := tmutil(run, "addexclusion", clone); err != nil {
			failures = append(failures, err)
			continue
		}
		logf("excluded %s", clone)
	}
	return errors.Join(failures...)
}

// removeStale drops the exclusion from every stale directory.
func removeStale(stale []string, run runner, logf logger) error {
	var failures []error
	for _, path := range stale {
		if _, err := tmutil(run, "removeexclusion", path); err != nil {
			failures = append(failures, err)
			continue
		}
		logf("removed stale exclusion %s", path)
	}
	return errors.Join(failures...)
}

// status describes one directory for the `excluded` command.
type status struct {
	state string // excluded, pending or stale
	path  string
}

// statuses reports every clone as excluded or pending and every stale
// directory as stale. A failed status check is reported but does not hide
// the other results.
func statuses(s scan, run runner) ([]status, error) {
	var result []status
	var failures []error
	for _, clone := range s.clones {
		excluded, err := isExcluded(clone, run)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		state := "pending"
		if excluded {
			state = "excluded"
		}
		result = append(result, status{state, clone})
	}
	for _, path := range s.stale {
		result = append(result, status{"stale", path})
	}
	return result, errors.Join(failures...)
}

// isExcluded parses `tmutil isexcluded`, which exits 0 for every outcome and
// reports the state as a bracketed prefix: [Excluded], [Included] or [UNKNOWN].
func isExcluded(path string, run runner) (bool, error) {
	out, err := tmutil(run, "isexcluded", path)
	if err != nil {
		return false, err
	}
	return bytes.HasPrefix(out, []byte("[Excluded]")), nil
}

func tmutil(run runner, verb, path string) ([]byte, error) {
	out, err := run(verb, path)
	if err != nil {
		return nil, fmt.Errorf("tmutil %s %s: %w: %s", verb, path, err, bytes.TrimSpace(out))
	}
	return out, nil
}
