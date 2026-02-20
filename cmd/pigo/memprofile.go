//go:build memprofile

package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
)

func setupMemProfile() func() {
	memProfile := os.Getenv("PIGO_MEMPROFILE")
	if memProfile == "" {
		return func() {}
	}
	return func() {
		runtime.GC()
		f, err := os.Create(memProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not create memory profile: %v\n", err)
			return
		}
		defer f.Close()
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "could not write memory profile: %v\n", err)
		}
	}
}
