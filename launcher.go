package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/exec"
	"runtime/debug"
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

	adaptersDisabled := false

	disableNetworks := func() {
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

	// ── System tray — must run on the main goroutine ───────────────────────
	systray.Run(func() {
		systray.SetIcon(makeTrayIcon(false))
		systray.SetTooltip("GFN Net Wrapper — активен")

		mStatus := systray.AddMenuItem("Состояние: ожидание запуска GFN", "")
		mStatus.Disable()
		
		systray.AddSeparator()
		mLaunch := systray.AddMenuItem("🚀 Запустить GeForce NOW", "Запустить GFN вручную")
		systray.AddSeparator()
		mSettings := systray.AddMenuItem("⚙  Настройки", "Открыть окно настройки адаптеров")
		systray.AddSeparator()
		mExit := systray.AddMenuItem("✖  Выйти из обёртки", "Включить адаптеры и завершить работу")

		// Memory cleanup loop
		go func() {
			// Free initial startup memory
			time.Sleep(2 * time.Second)
			debug.FreeOSMemory()
			
			// Periodically force garbage collection and memory release to OS
			for {
				time.Sleep(1 * time.Minute)
				debug.FreeOSMemory()
			}
		}()

		// Monitoring loop
		go func() {
			for {
				running := isGFNRunning()

				if running && !adaptersDisabled {
					log.Println("[Monitor] GFN started. Disabling networks.")
					disableNetworks()
					if cfg.BandwidthLimitMbps > 0 {
						log.Printf("[Monitor] Applying bandwidth limit: %d Mbps", cfg.BandwidthLimitMbps)
						if err := ApplyBandwidthLimit(cfg.BandwidthLimitMbps); err != nil {
							log.Printf("[Monitor] WARNING bandwidth limit: %v", err)
						}
						mStatus.SetTitle(fmt.Sprintf("Состояние: GFN запущен, лимит %d Мбит/с", cfg.BandwidthLimitMbps))
					} else {
						mStatus.SetTitle("Состояние: GFN запущен, адаптеры отключены")
					}
					mLaunch.Disable()
					systray.SetIcon(makeTrayIcon(true))
				} else if !running && adaptersDisabled {
					log.Println("[Monitor] GFN stopped. Re-enabling networks.")
					if cfg.BandwidthLimitMbps > 0 {
						_ = RemoveBandwidthLimit()
					}
					enableNetworks()
					mStatus.SetTitle("Состояние: ожидание запуска GFN")
					mLaunch.Enable()
					systray.SetIcon(makeTrayIcon(false))
				}

				time.Sleep(2 * time.Second)
			}
		}()

		// UI event loop
		go func() {
			for {
				select {
				case <-mLaunch.ClickedCh:
					log.Printf("[Launcher] Manually launching: %s", cfg.GFNPath)
					_ = exec.Command(cfg.GFNPath).Start()

				case <-mSettings.ClickedCh:
					// Launch a new instance of ourselves with --setup.
					exe, err := os.Executable()
					if err == nil {
						_ = exec.Command(exe, "--setup").Start()
					}

				case <-mExit.ClickedCh:
					// User exits wrapper manually — re-enable adapters.
					log.Println("[Launcher] Exiting...")
					mExit.Disable()
					mExit.SetTitle("Выход...")
					
					go func() {
						if cfg.BandwidthLimitMbps > 0 {
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
