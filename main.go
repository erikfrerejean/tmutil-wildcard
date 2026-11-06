// Command tmutil-wildcard keeps Time Machine exclusions in step with the git
// clones below the given roots: every clone root gets a sticky exclusion, and
// excluded folders that are no longer clones lose theirs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `usage: %s COMMAND ROOT [ROOT...]

commands:
  exclude   remove stale exclusions, then exclude every clone that lacks one
  list      print every clone root found below the roots
  excluded  print each clone as excluded or pending, and each excluded
            folder that is no longer a clone as stale
`, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}

	action, ok := commands[flag.Arg(0)]
	if !ok {
		flag.Usage()
		os.Exit(2)
	}

	var failures []error
	for _, root := range flag.Args()[1:] {
		s, err := scanRoot(root, log.Printf)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if err := action(s); err != nil {
			failures = append(failures, err)
		}
	}
	if err := errors.Join(failures...); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

var commands = map[string]func(s scan) error{
	"exclude": func(s scan) error {
		return errors.Join(
			removeStale(s.stale, runTmutil, log.Printf),
			excludeClones(s.clones, runTmutil, log.Printf),
		)
	},
	"list": func(s scan) error {
		for _, clone := range s.clones {
			fmt.Println(clone)
		}
		return nil
	},
	"excluded": func(s scan) error {
		result, err := statuses(s, runTmutil)
		for _, st := range result {
			fmt.Printf("%-9s %s\n", st.state, st.path)
		}
		return err
	},
}

func runTmutil(args ...string) ([]byte, error) {
	return exec.Command("tmutil", args...).CombinedOutput()
}
