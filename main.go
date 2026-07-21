//go:build windows

package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
)

func main() {
	// ── Determine executable directory ─────────────────────────────────────
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("Could not determine executable path: %v", err)
	}
	exeDir := filepath.Dir(exe)

	// ── Configure logging ──────────────────────────────────────────────────
	logPath := filepath.Join(exeDir, "gfn-wrapper.log")
	// Rotate log if it exceeds 1 MB to prevent unbounded growth.
	const maxLogSize = 1 << 20 // 1 MB
	if info, statErr := os.Stat(logPath); statErr == nil && info.Size() > maxLogSize {
		_ = os.Rename(logPath, logPath+".old")
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(logFile)
	}
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.SetPrefix("[GFN-VPN-FIXER] ")

	// ── Parse flags ────────────────────────────────────────────────────────
	setupFlag := flag.Bool("setup", false, "Open the GUI setup window")
	flag.Parse()

	// ── Initialize locale (must be before any UI strings are used) ────────
	initLocale()

	// ── Admin check ────────────────────────────────────────────────────────
	// Both GUI and Launcher modes require administrator privileges.
	requireAdmin()

	// ── Determine config path ──────────────────────────────────────────────
	cfgPath, err := configPath()
	if err != nil {
		log.Fatalf("Could not resolve config path: %v", err)
	}

	// ── Load existing config (may not exist yet) ───────────────────────────
	cfg, cfgErr := loadConfig(cfgPath)

	// ── Mode dispatch ──────────────────────────────────────────────────────
	//   GUI mode  : --setup flag  OR  config.json is missing / unreadable
	//   Launcher  : config.json exists and no --setup flag
	if *setupFlag || cfgErr != nil {
		if cfgErr != nil && !os.IsNotExist(cfgErr) {
			// Config exists but is malformed — warn, then open GUI.
			log.Printf("WARNING: could not load config (%v). Opening setup.", cfgErr)
		}
		log.Println("Mode: GUI Setup")
		runGUI(cfg, cfgPath)
	} else {
		log.Println("Mode: Headless Launcher")
		// Moderate garbage collection to keep background memory footprint small
		debug.SetGCPercent(50)
		runLauncher(cfg, cfgPath)
	}
}
