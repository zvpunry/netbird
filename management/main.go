package main

import (
	"os"
	"runtime/pprof"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/management/cmd"
)

func main() {
	f, err := os.Create("cpu_profile.prof")
	if err != nil {
		log.Fatal("could not create CPU profile: ", err)
	}
	defer f.Close()

	// Start CPU profiling.
	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatal("could not start CPU profile: ", err)
	}
	log.Info("Starting CPU profiler")

	time.AfterFunc(5*time.Minute,
		func() {
			pprof.StopCPUProfile()
			log.Info("CPU profile stopped")
		})

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
