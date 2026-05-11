package main

import (
	"fmt"
	"net/http"
	"runtime"
)

func handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeJSON(w, http.StatusOK, map[string]any{
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"cpus":         runtime.NumCPU(),
		"go_version":   runtime.Version(),
		"goroutines":   runtime.NumGoroutine(),
		"memory_alloc": fmt.Sprintf("%.2f MB", float64(mem.Alloc)/1048576),
		"memory_sys":   fmt.Sprintf("%.2f MB", float64(mem.Sys)/1048576),
	})
}
