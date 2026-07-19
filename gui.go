package main

import (
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// runGUI initialises and runs the Fyne setup window.
// It blocks until the user closes the window or clicks "Save & Exit".
func runGUI(cfg Config, cfgPath string) {
	a := app.NewWithID("com.gfnwrapper.setup")
	a.Settings().SetTheme(newUnicodeTheme())

	w := a.NewWindow("GFN Wrapper — Setup")
	w.Resize(fyne.NewSize(520, 780))
	w.CenterOnScreen()

	// ── Loading Screen ─────────────────────────────────────────────────────
	loadingSpinner := widget.NewProgressBarInfinite()
	loadingLabel := widget.NewLabel("Поиск сетевых адаптеров и служб...\n(Это может занять пару секунд)")
	loadingLabel.Alignment = fyne.TextAlignCenter

	loadingContainer := container.NewCenter(
		container.NewVBox(
			loadingSpinner,
			loadingLabel,
		),
	)
	w.SetContent(loadingContainer)

	// Start data loading in a goroutine
	go func() {
		// ── Fetch adapter list ─────────────────────────────────────────────────
		adapters, err := listAdapters()
		if err != nil {
			log.Printf("[GUI] Could not list adapters: %v", err)
			adapters = []AdapterInfo{}
		}

		// ── Zero Trust VPN ────────────────────────────────────────────────────
		vpnServices, vpnErr := detectVPNServices()
		if vpnErr != nil {
			log.Printf("[GUI] Could not detect VPN services: %v", vpnErr)
			vpnServices = []VPNServiceInfo{}
		}

		// Track which adapters are currently checked.
		checked := make(map[string]bool)
		for _, name := range cfg.Blacklist {
			checked[name] = true
		}

		// ── Build adapter list with descriptions ──────────────────────────────
		var checkItems []fyne.CanvasObject
		for _, adapter := range adapters {
			adapter := adapter // capture for closure
			cb := widget.NewCheck("", func(selected bool) {
				checked[adapter.Name] = selected
			})
			cb.SetChecked(checked[adapter.Name])

			descText := adapter.Description
			if descText == "" {
				descText = adapter.Name
			}
			descLabel := widget.NewRichText(&widget.TextSegment{
				Text: descText,
				Style: widget.RichTextStyle{
					TextStyle: fyne.TextStyle{Bold: true},
					SizeName:  theme.SizeNameSubHeadingText,
				},
			})
			nameLabel := widget.NewRichText(&widget.TextSegment{
				Text: adapter.Name,
				Style: widget.RichTextStyle{
					SizeName:  theme.SizeNameCaptionText,
					ColorName: theme.ColorNameDisabled,
				},
			})

			row := container.NewHBox(
				cb,
				container.NewVBox(descLabel, nameLabel),
			)
			checkItems = append(checkItems, row)
		}
		if len(checkItems) == 0 {
			checkItems = append(checkItems, widget.NewLabel("(No adapters found — ensure PowerShell access)"))
		}
		
		adapterBox := container.NewVBox(checkItems...)
		adapterGroup := widget.NewCard("Network Adapters to Disable", "Check adapters that should be disabled while GFN runs.", adapterBox)

		// ── Zero Trust VPN ────────────────────────────────────────────────────
		vpnChecked := make(map[string]bool)
		for _, svc := range cfg.VPNServices {
			vpnChecked[svc] = true
		}

		var vpnItems []fyne.CanvasObject
		for _, svc := range vpnServices {
			svc := svc // capture
			cb := widget.NewCheck("", func(selected bool) {
				vpnChecked[svc.ServiceName] = selected
			})
			cb.SetChecked(vpnChecked[svc.ServiceName])

			displayLabel := widget.NewRichText(&widget.TextSegment{
				Text: svc.DisplayName,
				Style: widget.RichTextStyle{
					TextStyle: fyne.TextStyle{Bold: true},
					SizeName:  theme.SizeNameSubHeadingText,
				},
			})
			serviceLabel := widget.NewRichText(&widget.TextSegment{
				Text: svc.ServiceName,
				Style: widget.RichTextStyle{
					SizeName:  theme.SizeNameCaptionText,
					ColorName: theme.ColorNameDisabled,
				},
			})

			row := container.NewHBox(cb, container.NewVBox(displayLabel, serviceLabel))
			vpnItems = append(vpnItems, row)
		}
		if len(vpnItems) == 0 {
			vpnItems = append(vpnItems, widget.NewLabel("(VPN-сервисы не обнаружены)"))
		}
		
		vpnBox := container.NewVBox(vpnItems...)
		vpnGroup := widget.NewCard("Zero Trust VPN", "Службы VPN, которые будут остановлены при запуске.", vpnBox)

		// ── GFN path entry ─────────────────────────────────────────────────────
		gfnEntry := widget.NewEntry()
		gfnEntry.SetPlaceHolder(defaultGFNPath())
		if cfg.GFNPath != "" {
			gfnEntry.SetText(cfg.GFNPath)
		} else {
			gfnEntry.SetText(defaultGFNPath())
		}

		gfnGroup := widget.NewCard("GeForce NOW Path", "Full path to the GeForceNOW.exe executable.", gfnEntry)

		// ── Bandwidth limit ──────────────────────────────────────────────────
		bandwidthEntry := widget.NewEntry()
		bandwidthEntry.SetPlaceHolder("0")
		if cfg.BandwidthLimitMbps > 0 {
			bandwidthEntry.SetText(fmt.Sprintf("%d", cfg.BandwidthLimitMbps))
		} else {
			bandwidthEntry.SetText("0")
		}
		bandwidthEntry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			v, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("введите целое число")
			}
			if v < 0 {
				return fmt.Errorf("значение не может быть отрицательным")
			}
			return nil
		}

		bandwidthGroup := widget.NewCard(
			"Ограничение скорости (Мбит/с)",
			"Ограничивает пропускную способность для GFN через Windows QoS.\n0 = без ограничений.",
			bandwidthEntry,
		)

		// ── Launch GFN checkbox ───────────────────────────────────────────────
		launchGFNCheck := widget.NewCheck(
			"Запускать GeForce NOW вместе с программой",
			nil,
		)
		launchGFNCheck.SetChecked(cfg.LaunchGFN)

		// ── Autostart checkbox ─────────────────────────────────────────────────
		autostartCheck := widget.NewCheck(
			"Запускать автоматически при старте Windows (без интерфейса)",
			nil,
		)
		autostartCheck.SetChecked(isAutostartEnabled())

		autostartGroup := widget.NewCard(
			"Автозапуск",
			"Создаёт задачу в Планировщике Windows с правами администратора. Запрос UAC не появляется.",
			autostartCheck,
		)

		// ── Status label ───────────────────────────────────────────────────────
		statusLabel := widget.NewLabel("")
		statusLabel.Wrapping = fyne.TextWrapWord

		// ── Save & Exit button ─────────────────────────────────────────────────
		saveBtn := widget.NewButton("💾  Save & Exit", func() {
			// Collect checked adapter names.
			var blacklist []string
			for _, adapter := range adapters {
				if checked[adapter.Name] {
					blacklist = append(blacklist, adapter.Name)
				}
			}

			// Collect checked VPN services.
			var vpnServiceList []string
			for _, svc := range vpnServices {
				if vpnChecked[svc.ServiceName] {
					vpnServiceList = append(vpnServiceList, svc.ServiceName)
				}
			}

			bwLimit := 0
			if v, err := strconv.Atoi(bandwidthEntry.Text); err == nil && v > 0 {
				bwLimit = v
			}

			newCfg := Config{
				Blacklist:          blacklist,
				GFNPath:            gfnEntry.Text,
				LaunchGFN:          launchGFNCheck.Checked,
				VPNServices:        vpnServiceList,
				BandwidthLimitMbps: bwLimit,
			}

			if err := saveConfig(cfgPath, newCfg); err != nil {
				statusLabel.SetText("❌ Error saving config: " + err.Error())
				log.Printf("[GUI] Save error: %v", err)
				return
			}

			// Apply autostart registry setting.
			if err := setAutostart(autostartCheck.Checked); err != nil {
				statusLabel.SetText("❌ Ошибка автозапуска: " + err.Error())
				log.Printf("[GUI] Autostart error: %v", err)
				return
			}

			statusLabel.SetText("✅ Config saved! You can close this window.")
			log.Printf("[GUI] Config saved to %s", cfgPath)
			a.Quit()
		})
		saveBtn.Importance = widget.HighImportance

		// ── Layout ─────────────────────────────────────────────────────────────
		allContent := container.NewVBox(
			adapterGroup,
			vpnGroup,
			gfnGroup,
			bandwidthGroup,
			launchGFNCheck,
			autostartGroup,
			statusLabel,
			saveBtn,
		)
		scrollableContent := container.NewVScroll(container.NewPadded(allContent))

		// Replace the loading screen with the final UI
		w.SetContent(scrollableContent)

		// Free memory after UI is built
		go func() {
			time.Sleep(500 * time.Millisecond)
			debug.FreeOSMemory()
		}()
	}()

	w.ShowAndRun()
}
