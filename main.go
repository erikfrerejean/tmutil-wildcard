// Command tmutil-wildcard excludes every git clone below the given roots from
// Time Machine by calling `tmutil addexclusion` on each clone root.
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
  exclude   add a Time Machine exclusion for every clone that lacks one
  list      print every clone root found below the roots
  excluded  print the clone roots Time Machine currently excludes
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
		clones, err := findClones(root, log.Printf)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if err := action(clones); err != nil {
			failures = append(failures, err)
		}
	}
	if err := errors.Join(failures...); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

var commands = map[string]func(clones []string) error{
	"exclude": func(clones []string) error {
		return excludeClones(clones, runTmutil, log.Printf)
	},
	"list": func(clones []string) error {
		printLines(clones)
		return nil
	},
	"excluded": func(clones []string) error {
		excluded, err := excludedClones(clones, runTmutil)
		printLines(excluded)
		return err
	},
}

func printLines(paths []string) {
	for _, path := range paths {
		fmt.Println(path)
	}
}

func runTmutil(args ...string) ([]byte, error) {
	return exec.Command("tmutil", args...).CombinedOutput()
}
