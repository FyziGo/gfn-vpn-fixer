package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
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
	a := app.NewWithID("com.gfnvpnfixer.setup")
	a.Settings().SetTheme(newUnicodeTheme())

	w := a.NewWindow("GFN VPN FIXER — Setup")
	w.Resize(fyne.NewSize(520, 620))
	w.SetFixedSize(false)
	w.CenterOnScreen()

	// ── Loading Screen ─────────────────────────────────────────────────────
	loadingSpinner := widget.NewProgressBarInfinite()
	loadingLabel := widget.NewLabel(L.LoadingText)
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

		// Split adapters into regular, VPN, and built-in VPN; hide excluded.
		var regularAdapters, vpnAdapterList, builtinVPNAdapters []AdapterInfo
		for _, a := range adapters {
			if isExcludedAdapter(a.Description) {
				continue
			}
			if isBuiltinVPNAdapter(a.Description) {
				builtinVPNAdapters = append(builtinVPNAdapters, a)
			} else if matchesVPNAdapter(a.Description) {
				vpnAdapterList = append(vpnAdapterList, a)
			} else {
				regularAdapters = append(regularAdapters, a)
			}
		}

		// On first run (no config file), default built-in VPN adapters to checked.
		_, statErr := os.Stat(cfgPath)
		if statErr != nil {
			for _, a := range builtinVPNAdapters {
				checked[a.Name] = true
			}
		}

		// ── Build adapter list with descriptions ──────────────────────────────
		var checkItems []fyne.CanvasObject
		for _, adapter := range regularAdapters {
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
			checkItems = append(checkItems, widget.NewLabel(L.NoAdapters))
		}
		
		adapterBox := container.NewVBox(checkItems...)
		adapterSub := widget.NewLabel(L.AdaptersSubtitle)
		adapterSub.Wrapping = fyne.TextWrapWord
		adapterGroup := widget.NewCard(L.AdaptersTitle, "", container.NewVBox(adapterSub, adapterBox))

		// ── Unified VPN Section ────────────────────────────────────────────────
		vpnChecked := make(map[string]bool)
		for _, svc := range cfg.VPNServices {
			vpnChecked[svc] = true
		}

		// Group VPN services and adapters by provider.
		type unifiedVPN struct {
			DisplayName string
			Service     *VPNServiceInfo
			Adapters    []AdapterInfo
		}
		vpnMap := make(map[string]*unifiedVPN)
		var vpnOrder []string

		for i := range vpnServices {
			svc := &vpnServices[i]
			key := matchedVPNPattern(svc.ServiceName)
			if key == "" {
				key = strings.ToLower(svc.ServiceName)
			}
			vpnMap[key] = &unifiedVPN{
				DisplayName: svc.DisplayName,
				Service:     svc,
			}
			vpnOrder = append(vpnOrder, key)
		}

		for _, adapter := range vpnAdapterList {
			key := matchedVPNPattern(adapter.Description)
			if key == "" {
				key = matchedVPNAdapterPattern(adapter.Description)
			}
			if key == "" {
				key = strings.ToLower(adapter.Description)
			}
			if entry, ok := vpnMap[key]; ok {
				entry.Adapters = append(entry.Adapters, adapter)
			} else {
				vpnMap[key] = &unifiedVPN{
					DisplayName: adapter.Description,
					Adapters:    []AdapterInfo{adapter},
				}
				vpnOrder = append(vpnOrder, key)
			}
		}

		// Add Windows built-in VPN adapters as a single grouped entry.
		if len(builtinVPNAdapters) > 0 {
			const builtinKey = "_windows_builtin_vpn"
			vpnMap[builtinKey] = &unifiedVPN{
				DisplayName: L.BuiltinVPNName,
				Adapters:    builtinVPNAdapters,
			}
			vpnOrder = append(vpnOrder, builtinKey)
		}

		var vpnItems []fyne.CanvasObject
		for _, key := range vpnOrder {
			entry := vpnMap[key]
			svc := entry.Service
			entryAdapters := entry.Adapters

			// Build per-adapter checkboxes for the expandable section.
			adapterCbs := make(map[string]*widget.Check)
			var adapterRows []fyne.CanvasObject
			for _, a := range entryAdapters {
				a := a
				acb := widget.NewCheck(a.Description+" — "+a.Name, nil)
				acb.SetChecked(checked[a.Name])
				acb.OnChanged = func(sel bool) {
					checked[a.Name] = sel
				}
				adapterCbs[a.Name] = acb
				adapterRows = append(adapterRows, acb)
			}

			// Main checkbox: controls service + all adapters at once.
			mainCb := widget.NewCheck("", nil)
			mainCb.OnChanged = func(sel bool) {
				if svc != nil {
					vpnChecked[svc.ServiceName] = sel
				}
				for _, a := range entryAdapters {
					checked[a.Name] = sel
					if cb, ok := adapterCbs[a.Name]; ok {
						cb.SetChecked(sel)
					}
				}
			}

			// Determine initial checked state.
			allOn := true
			hasItems := false
			if svc != nil {
				hasItems = true
				if !vpnChecked[svc.ServiceName] {
					allOn = false
				}
			}
			for _, a := range entryAdapters {
				hasItems = true
				if !checked[a.Name] {
					allOn = false
					break
				}
			}
			if !hasItems {
				allOn = false
			}
			mainCb.SetChecked(allOn)

			// Display name.
			descLabel := widget.NewRichText(&widget.TextSegment{
				Text: entry.DisplayName,
				Style: widget.RichTextStyle{
					TextStyle: fyne.TextStyle{Bold: true},
					SizeName:  theme.SizeNameSubHeadingText,
				},
			})

			// Subtitle.
			subtitle := ""
			if svc != nil {
				subtitle = svc.ServiceName
			} else if len(entryAdapters) > 0 {
				names := make([]string, len(entryAdapters))
				for i, a := range entryAdapters {
					names[i] = a.Name
				}
				subtitle = strings.Join(names, ", ")
			}
			subtitleLabel := widget.NewRichText(&widget.TextSegment{
				Text: subtitle,
				Style: widget.RichTextStyle{
					SizeName:  theme.SizeNameCaptionText,
					ColorName: theme.ColorNameDisabled,
				},
			})

			mainRow := container.NewHBox(mainCb, container.NewVBox(descLabel, subtitleLabel))
			entryBox := container.NewVBox(mainRow)

			// Expandable adapter list (hidden by default).
			if len(adapterRows) > 0 {
				adapterContainer := container.NewVBox(adapterRows...)
				adapterContainer.Hide()

				expandText := fmt.Sprintf(L.VPNAdaptersExpand, len(entryAdapters))
				collapseText := fmt.Sprintf(L.VPNAdaptersCollapse, len(entryAdapters))

				var expandBtn *widget.Button
				expandBtn = widget.NewButton(expandText, func() {
					if adapterContainer.Visible() {
						adapterContainer.Hide()
						expandBtn.SetText(expandText)
					} else {
						adapterContainer.Show()
						expandBtn.SetText(collapseText)
					}
				})
				expandBtn.Importance = widget.LowImportance

				entryBox.Add(expandBtn)
				entryBox.Add(adapterContainer)
			}

			vpnItems = append(vpnItems, entryBox)
		}

		if len(vpnItems) == 0 {
			vpnItems = append(vpnItems, widget.NewLabel(L.NoVPN))
		}

		vpnBox := container.NewVBox(vpnItems...)
		vpnSub := widget.NewLabel(L.VPNSubtitle)
		vpnSub.Wrapping = fyne.TextWrapWord
		vpnGroup := widget.NewCard(L.VPNTitle, "", container.NewVBox(vpnSub, vpnBox))

		// ── GFN path entry ─────────────────────────────────────────────────────
		gfnEntry := widget.NewEntry()
		gfnEntry.SetPlaceHolder(defaultGFNPath())
		if cfg.GFNPath != "" {
			gfnEntry.SetText(cfg.GFNPath)
		} else {
			gfnEntry.SetText(defaultGFNPath())
		}

		gfnGroup := widget.NewCard(L.GFNPathTitle, L.GFNPathSubtitle, gfnEntry)

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
				return fmt.Errorf("%s", L.BandwidthErrInt)
			}
			if v < 0 {
				return fmt.Errorf("%s", L.BandwidthErrNeg)
			}
			return nil
		}

		bandwidthSub := widget.NewLabel(L.BandwidthSubtitle)
		bandwidthSub.Wrapping = fyne.TextWrapWord
		bandwidthGroup := widget.NewCard(
			L.BandwidthTitle,
			"",
			container.NewVBox(bandwidthSub, bandwidthEntry),
		)

		// ── Launch GFN checkbox ───────────────────────────────────────────────
		launchGFNCheck := widget.NewCheck(
			L.LaunchGFNLabel,
			nil,
		)
		launchGFNCheck.SetChecked(cfg.LaunchGFN)

		// ── Autostart checkbox ─────────────────────────────────────────────────
		autostartCheck := widget.NewCheck(
			L.AutostartLabel,
			nil,
		)
		autostartCheck.SetChecked(isAutostartEnabled())

		autostartSub := widget.NewLabel(L.AutostartSubtitle)
		autostartSub.Wrapping = fyne.TextWrapWord
		autostartGroup := widget.NewCard(
			L.AutostartTitle,
			"",
			container.NewVBox(autostartSub, autostartCheck),
		)

		// ── Status label ───────────────────────────────────────────────────────
		statusLabel := widget.NewLabel("")
		statusLabel.Wrapping = fyne.TextWrapWord

		// ── Save & Exit button ─────────────────────────────────────────────────
		saveBtn := widget.NewButton(L.SaveButton, func() {
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
				statusLabel.SetText(L.SaveError + err.Error())
				log.Printf("[GUI] Save error: %v", err)
				return
			}

			// Apply autostart registry setting.
			if err := setAutostart(autostartCheck.Checked); err != nil {
				statusLabel.SetText(L.AutostartError + err.Error())
				log.Printf("[GUI] Autostart error: %v", err)
				return
			}

			statusLabel.SetText(L.SaveSuccess)
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
		scrollableContent := container.NewScroll(container.NewPadded(allContent))

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
