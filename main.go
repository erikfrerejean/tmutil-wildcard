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
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s ROOT [ROOT...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	var failures []error
	for _, root := range flag.Args() {
		clones, err := findClones(root, log.Printf)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if err := excludeClones(clones, runTmutil, log.Printf); err != nil {
			failures = append(failures, err)
		}
	}
	if err := errors.Join(failures...); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func runTmutil(args ...string) ([]byte, error) {
	return exec.Command("tmutil", args...).CombinedOutput()
}
