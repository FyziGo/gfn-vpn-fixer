package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"sync"
	"time"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
)

// isGFNName performs a zero-allocation case-insensitive check for "geforcenow.exe"
func isGFNName(name []uint16) bool {
	const target = "geforcenow.exe"
	for i := 0; i < len(target); i++ {
		c := name[i]
		if c == 0 {
			return false
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != uint16(target[i]) {
			return false
		}
	}
	return name[len(target)] == 0
}

// isGFNRunning quickly checks if GeForceNOW.exe is running using Windows API.
func isGFNRunning() bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)

	var pe32 windows.ProcessEntry32
	pe32.Size = uint32(unsafe.Sizeof(pe32))

	if err := windows.Process32First(snapshot, &pe32); err != nil {
		return false
	}

	for {
		if isGFNName(pe32.ExeFile[:]) {
			return true
		}
		if err := windows.Process32Next(snapshot, &pe32); err != nil {
			break
		}
	}
	return false
}

// runLauncher executes the headless launcher flow and shows a system-tray icon
// so the user can access Settings or exit without a command line.
func runLauncher(cfg Config, cfgPath string) {
	log.Println("[Launcher] Starting headless monitoring mode.")

	if cfg.GFNPath == "" {
		cfg.GFNPath = defaultGFNPath()
	}

	var (
		mu               sync.Mutex
		adaptersDisabled bool
		ctx, cancel      = context.WithCancel(context.Background())
	)

	disableNetworks := func() {
		mu.Lock()
		defer mu.Unlock()
		if adaptersDisabled {
			return
		}
		log.Printf("[Launcher] Batch disabling %d VPN(s) and %d adapter(s)...", len(cfg.VPNServices), len(cfg.Blacklist))
		if err := DisableNetworksBatch(cfg.VPNServices, cfg.Blacklist); err != nil {
			log.Printf("[Launcher] WARNING disable batch: %v", err)
		}
		adaptersDisabled = true
	}

	enableNetworks := func() {
		mu.Lock()
		defer mu.Unlock()
		if !adaptersDisabled {
			return
		}
		log.Printf("[Launcher] Batch re-enabling %d adapter(s) and %d VPN(s)...", len(cfg.Blacklist), len(cfg.VPNServices))
		if err := EnableNetworksBatch(cfg.VPNServices, cfg.Blacklist); err != nil {
			log.Printf("[Launcher] WARNING enable batch: %v", err)
		}
		adaptersDisabled = false
	}

	if cfg.LaunchGFN {
		log.Printf("[Launcher] LaunchGFN is true. Launching GFN immediately: %s", cfg.GFNPath)
		_ = exec.Command(cfg.GFNPath).Start()
	}

	// ── Safety re-enable: recover from crash / forced kill ───────────────
	// If GFN is NOT running but some blacklisted adapters are disabled,
	// a previous instance likely crashed without cleanup.
	if !isGFNRunning() && len(cfg.Blacklist) > 0 {
		stuck := findDisabledAdapters(cfg.Blacklist)
		if len(stuck) > 0 {
			log.Printf("[Launcher] Safety re-enable: found %d adapter(s) left disabled from previous crash", len(stuck))
			if err := EnableNetworksBatch(cfg.VPNServices, stuck); err != nil {
				log.Printf("[Launcher] WARNING safety re-enable: %v", err)
			}
		}
	}

	// ── Pre-generate tray icons (avoids re-rendering on every state change) ──
	iconIdle := makeTrayIcon(false)
	iconActive := makeTrayIcon(true)

	// ── System tray — must run on the main goroutine ───────────────────────
	systray.Run(func() {
		// Delay initial icon to avoid "unable to set icon" race with Shell_TrayWnd.
		go func() {
			time.Sleep(150 * time.Millisecond)
			systray.SetIcon(iconIdle)
		}()
		systray.SetTooltip(L.TrayTooltip)

		mStatus := systray.AddMenuItem(L.StatusWaiting, "")
		mStatus.Disable()

		systray.AddSeparator()
		mLaunch := systray.AddMenuItem(L.MenuLaunch, L.MenuLaunchTip)
		systray.AddSeparator()
		mSettings := systray.AddMenuItem(L.MenuSettings, L.MenuSettingsTip)
		systray.AddSeparator()
		mReload := systray.AddMenuItem(L.MenuReload, L.MenuReloadTip)
		systray.AddSeparator()
		mExit := systray.AddMenuItem(L.MenuExit, L.MenuExitTip)

		// Free initial startup memory after a short delay (one-shot).
		go func() {
			time.Sleep(2 * time.Second)
			debug.FreeOSMemory()
		}()

		// OS signal handler: re-enable adapters on SIGINT, SIGTERM, or console close.
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, os.Kill)
			<-sigCh
			log.Println("[Launcher] OS signal received, cleaning up...")
			cancel()
			mu.Lock()
			bw := cfg.BandwidthLimitMbps
			mu.Unlock()
			if bw > 0 {
				_ = RemoveBandwidthLimit()
			}
			enableNetworks()
			systray.Quit()
		}()

		// Monitoring loop
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				running := isGFNRunning()

				mu.Lock()
				disabled := adaptersDisabled
				bwLimit := cfg.BandwidthLimitMbps
				mu.Unlock()

				if running && !disabled {
					log.Println("[Monitor] GFN started. Disabling networks.")
					disableNetworks()
					if bwLimit > 0 {
						log.Printf("[Monitor] Applying bandwidth limit: %d Mbps", bwLimit)
						if err := ApplyBandwidthLimit(bwLimit); err != nil {
							log.Printf("[Monitor] WARNING bandwidth limit: %v", err)
						}
						mStatus.SetTitle(fmt.Sprintf(L.StatusBandwidth, bwLimit))
					} else {
						mStatus.SetTitle(L.StatusRunning)
					}
					mLaunch.Disable()
					systray.SetIcon(iconActive)
				} else if !running && disabled {
					log.Println("[Monitor] GFN stopped. Re-enabling networks.")
					if bwLimit > 0 {
						_ = RemoveBandwidthLimit()
					}
					enableNetworks()
					mStatus.SetTitle(L.StatusWaiting)
					mLaunch.Enable()
					systray.SetIcon(iconIdle)
				}

				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()

		// UI event loop
		go func() {
			for {
				select {
				case <-mLaunch.ClickedCh:
					mu.Lock()
					gfnPath := cfg.GFNPath
					mu.Unlock()
					log.Printf("[Launcher] Manually launching: %s", gfnPath)
					_ = exec.Command(gfnPath).Start()

				case <-mSettings.ClickedCh:
					// Launch a new instance of ourselves with --setup.
					exe, err := os.Executable()
					if err == nil {
						_ = exec.Command(exe, "--setup").Start()
					}

				case <-mReload.ClickedCh:
					newCfg, err := loadConfig(cfgPath)
					if err != nil {
						log.Printf("[Launcher] Config reload failed: %v", err)
						mStatus.SetTitle(L.ReloadFail)
					} else {
						mu.Lock()
						cfg = newCfg
						if cfg.GFNPath == "" {
							cfg.GFNPath = defaultGFNPath()
						}
						mu.Unlock()
						log.Printf("[Launcher] Config reloaded from %s", cfgPath)
						mStatus.SetTitle(L.ReloadOK)
					}
					// Restore normal status after 3s.
					go func() {
						time.Sleep(3 * time.Second)
						mu.Lock()
						d := adaptersDisabled
						bw := cfg.BandwidthLimitMbps
						mu.Unlock()
						if d {
							if bw > 0 {
								mStatus.SetTitle(fmt.Sprintf(L.StatusBandwidth, bw))
							} else {
								mStatus.SetTitle(L.StatusRunning)
							}
						} else {
							mStatus.SetTitle(L.StatusWaiting)
						}
					}()

				case <-mExit.ClickedCh:
					// User exits wrapper manually — stop monitor, re-enable adapters.
					log.Println("[Launcher] Exiting...")
					mExit.Disable()
					mExit.SetTitle(L.Exiting)
					cancel() // stop monitoring loop to prevent concurrent enable

					go func() {
						mu.Lock()
						bw := cfg.BandwidthLimitMbps
						mu.Unlock()
						if bw > 0 {
							_ = RemoveBandwidthLimit()
						}
						enableNetworks()
						systray.Quit()
					}()
					return
				}
			}
		}()
	}, func() {
		// onExit callback
		os.Exit(0)
	})
}

// makeTrayIcon returns a small circle ICO used as the tray icon.
// If active is true, it returns a green circle; otherwise, a grey circle.
func makeTrayIcon(active bool) []byte {
	const size = 32
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	cx, cy := float64(size)/2-0.5, float64(size)/2-0.5
	r := float64(size)/2 - 1.5
	
	var c color.NRGBA
	if active {
		c = color.NRGBA{R: 0x76, G: 0xb9, B: 0x00, A: 0xff} // NVIDIA green
	} else {
		c = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff} // Grey
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, c)
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	pngData := buf.Bytes()

	// Windows requires an ICO format for the system tray icon.
	// We wrap the PNG data inside a valid ICO container.
	ico := new(bytes.Buffer)
	
	// ICONDIR (6 bytes)
	ico.Write([]byte{0, 0}) // Reserved
	ico.Write([]byte{1, 0}) // Type = ICO
	ico.Write([]byte{1, 0}) // Count = 1

	// ICONDIRENTRY (16 bytes)
	ico.WriteByte(32) // Width
	ico.WriteByte(32) // Height
	ico.WriteByte(0)  // Color count
	ico.WriteByte(0)  // Reserved
	ico.Write([]byte{1, 0}) // Color planes
	ico.Write([]byte{32, 0}) // Bits per pixel

	// Size of PNG data (4 bytes)
	binary.Write(ico, binary.LittleEndian, uint32(len(pngData)))
	
	// Offset of PNG data (4 bytes)
	binary.Write(ico, binary.LittleEndian, uint32(22))

	ico.Write(pngData)
	return ico.Bytes()
}
