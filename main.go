// Command tmutil-wildcard keeps Time Machine exclusions in step with the git
// clones below the given roots: every clone root gets a sticky exclusion, and
// excluded folders that are no longer clones lose theirs.
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

const usage = `usage: tmutil-wildcard COMMAND ROOT [ROOT...]

commands:
  exclude   remove stale exclusions, then exclude every clone that lacks one
  list      print every clone root found below the roots
  excluded  print each clone as excluded or pending, and each excluded
            folder that is no longer a clone as stale
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, runTmutil))
}

// run executes one command over the roots and returns the process exit code:
// 0 on success, 1 when a root or a tmutil call failed, 2 on usage errors.
func run(args []string, stdout, stderr io.Writer, tm runner) int {
	if len(args) < 2 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	action, ok := commands[args[0]]
	if !ok {
		fmt.Fprint(stderr, usage)
		return 2
	}
	logf := log.New(stderr, "", log.LstdFlags).Printf

	var failures []error
	for _, root := range args[1:] {
		s, err := scanRoot(root, logf)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if err := action(s, stdout, tm, logf); err != nil {
			failures = append(failures, err)
		}
	}
	if err := errors.Join(failures...); err != nil {
		logf("%v", err)
		return 1
	}
	return 0
}

var commands = map[string]func(s scan, out io.Writer, tm runner, logf logger) error{
	"exclude": func(s scan, _ io.Writer, tm runner, logf logger) error {
		return errors.Join(
			removeStale(s.stale, tm, logf),
			excludeClones(s.clones, tm, logf),
		)
	},
	"list": func(s scan, out io.Writer, _ runner, _ logger) error {
		for _, clone := range s.clones {
			fmt.Fprintln(out, clone)
		}
		return nil
	},
	"excluded": func(s scan, out io.Writer, tm runner, _ logger) error {
		result, err := statuses(s, tm)
		for _, st := range result {
			fmt.Fprintf(out, "%-9s %s\n", st.state, st.path)
		}
		return err
	},
}

func runTmutil(args ...string) ([]byte, error) {
	return exec.Command("tmutil", args...).CombinedOutput()
}
