// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program builds all u-root commands provided to it as arguments.
// It does not build the u-root initramfs.
// All build errors will be saved into a file to be reviewed later.
// It is loosely based on https://gist.github.com/leongross/3ae8517cfae68d1795ad7fe243efb701

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

type buildJob struct {
	cmd       string        // the build command executes by the worker.
	goPkgPath string        // path to go package built, aka the cmdlet.
	err       string        // error message of failed build. for golang builds this should always be empty.
	time      time.Duration // time it took to build the command.
	size      uint64        // size in bytes
}

func newBuildJob() (*buildJob, error) {
	b := new(buildJob)

	return b, nil
}

func (b *buildJob) build() {

}

// options for the building process
type builder struct {
	threads    uint   // number of threads to used for parallel building
	pathTingo  string // path to the tinygo binary
	pathGolang string // path to the go binary
	compare    bool   // compare output size of tinygo and go. If not set, only tinygo will be used
	verbose    bool   // print verbose output, aka. build errors
	outDirBin  string // output directory for the built binaries, default somewhere in tmp
	outDirCmp  string // output directory for the results
	logger     log.Logger
}

func (b *builder) parseCmdline() error {
	//  check if threads is valid

	// check if tinygo path is valid or if TINYGO env var is set

	// check if tinygo path is valid or if TINYGO env var is set

	// if compare is set, check if the outDirBin and outDirCmp were set.
	// if not, create temporary directories and set them in struct

	// if outDirCmp is not set, set it the cwd

	// if verbose is set, initialize the logger

	return nil
}

func run(args []string) error {
	b := &builder{}

	flag.UintVar(&b.threads, "threads", 1, "number of threads to used for parallel building")
	flag.StringVar(&b.pathTingo, "tinygo", "tinygo", "path to the tinygo binary")
	flag.StringVar(&b.pathGolang, "go", "go", "path to the go binary")
	flag.BoolVar(&b.compare, "compare", false, "compare output size of tinygo and go")
	flag.BoolVar(&b.verbose, "verbose", false, "print verbose output, aka. build errors")
	flag.StringVar(&b.outDirBin, "outbin", "", "output directory for the built binaries, default somewhere in tmp")
	flag.StringVar(&b.outDirCmp, "outcmp", "", "output directory for the results")
	flag.Parse()

	if err := b.parseCmdline(); err != nil {
		return fmt.Errorf("invalid command line arguments: %w", err)
	}

	for _, dir := range args {
		b.logger.Printf("building commands for paths %v\n", args)
		cmdlets, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("error reading cmdlet root dir: %w", err)
		}

		for _, cmd := range cmdlets {
			if cmd.IsDir() {
				// enter the cmdlet and build it
				b.logger.Printf("\tbuilding cmdlet '%v'\n", cmd.Name())

				// TODO: make this run in parallel

			}
		}
	}

	return nil
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error running builder: %v\n", err)
		os.Exit(1)
	}
}
