//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// ═══════════════════════════════════════════════════════════════════════
//  SetupDi DLL procedures (loaded once from setupapi.dll)
// ═══════════════════════════════════════════════════════════════════════

var (
	setupapi                              = windows.NewLazySystemDLL("setupapi.dll")
	procSetupDiGetClassDevsW              = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiDestroyDeviceInfoList      = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procSetupDiEnumDeviceInfo             = setupapi.NewProc("SetupDiEnumDeviceInfo")
	procSetupDiOpenDevRegKey              = setupapi.NewProc("SetupDiOpenDevRegKey")
	procSetupDiSetClassInstallParamsW     = setupapi.NewProc("SetupDiSetClassInstallParamsW")
	procSetupDiCallClassInstaller         = setupapi.NewProc("SetupDiCallClassInstaller")
	procSetupDiGetDeviceRegistryPropertyW = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
)

// ── Network Adapter device class GUID ────────────────────────────────
// {4D36E972-E325-11CE-BFC1-08002BE10318}
var guidDevclassNet = windows.GUID{
	Data1: 0x4d36e972,
	Data2: 0xe325,
	Data3: 0x11ce,
	Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18},
}

// ── SetupDi constants ────────────────────────────────────────────────
const (
	digcfPresent       = 0x02 // DIGCF_PRESENT
	spdrpDeviceDesc    = 0x00 // SPDRP_DEVICEDESC
	diregDrv           = 0x02 // DIREG_DRV
	dicsFlagGlobal     = 0x01 // DICS_FLAG_GLOBAL
	dicsEnable         = 0x01 // DICS_ENABLE
	dicsDisable        = 0x02 // DICS_DISABLE
	difPropertyChange  = 0x12 // DIF_PROPERTYCHANGE
	invalidHandleValue = ^uintptr(0)
)

// spDevInfoData mirrors the Windows SP_DEVINFO_DATA structure.
type spDevInfoData struct {
	Size      uint32
	ClassGUID windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

// spClassInstallHeader mirrors SP_CLASSINSTALL_HEADER.
type spClassInstallHeader struct {
	Size            uint32
	InstallFunction uint32
}

// spPropChangeParams mirrors SP_PROPCHANGE_PARAMS.
type spPropChangeParams struct {
	ClassInstallHeader spClassInstallHeader
	StateChange        uint32
	Scope              uint32
	HwProfile          uint32
}

// ═══════════════════════════════════════════════════════════════════════
//  SetupDi low-level helpers
// ═══════════════════════════════════════════════════════════════════════

func diGetClassDevs(classGUID *windows.GUID, flags uint32) (uintptr, error) {
	r1, _, err := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(classGUID)),
		0, 0, uintptr(flags),
	)
	if r1 == invalidHandleValue {
		return 0, fmt.Errorf("SetupDiGetClassDevsW: %w", err)
	}
	return r1, nil
}

func diDestroyDeviceInfoList(h uintptr) {
	procSetupDiDestroyDeviceInfoList.Call(h)
}

func diEnumDeviceInfo(h uintptr, idx uint32, data *spDevInfoData) bool {
	r1, _, _ := procSetupDiEnumDeviceInfo.Call(h, uintptr(idx), uintptr(unsafe.Pointer(data)))
	return r1 != 0
}

func diOpenDevRegKey(h uintptr, data *spDevInfoData) (syscall.Handle, error) {
	r1, _, err := procSetupDiOpenDevRegKey.Call(
		h, uintptr(unsafe.Pointer(data)),
		dicsFlagGlobal, 0, diregDrv, windows.KEY_READ,
	)
	hk := syscall.Handle(r1)
	if hk == syscall.InvalidHandle {
		return 0, err
	}
	return hk, nil
}

func diGetDeviceRegistryProperty(h uintptr, data *spDevInfoData, prop uint32) string {
	var buf [512]byte
	var size uint32
	r1, _, _ := procSetupDiGetDeviceRegistryPropertyW.Call(
		h, uintptr(unsafe.Pointer(data)),
		uintptr(prop), 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 || size < 2 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), size/2))
}

func diSetClassInstallParams(h uintptr, data *spDevInfoData, params *spPropChangeParams) error {
	r1, _, err := procSetupDiSetClassInstallParamsW.Call(
		h, uintptr(unsafe.Pointer(data)),
		uintptr(unsafe.Pointer(params)), unsafe.Sizeof(*params),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiSetClassInstallParamsW: %w", err)
	}
	return nil
}

func diCallClassInstaller(fn uint32, h uintptr, data *spDevInfoData) error {
	r1, _, err := procSetupDiCallClassInstaller.Call(
		uintptr(fn), h, uintptr(unsafe.Pointer(data)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiCallClassInstaller: %w", err)
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════
//  Adapter connection-name resolution (registry-based)
// ═══════════════════════════════════════════════════════════════════════

// getConnectionName maps a SetupDi device entry to its human-readable
// network connection name (e.g. "Ethernet", "Подключение по локальной сети").
func getConnectionName(devInfoSet uintptr, devData *spDevInfoData) (string, error) {
	hkey, err := diOpenDevRegKey(devInfoSet, devData)
	if err != nil {
		return "", err
	}
	key := registry.Key(hkey)
	defer key.Close()

	instanceID, _, err := key.GetStringValue("NetCfgInstanceId")
	if err != nil {
		return "", err
	}

	netPath := fmt.Sprintf(
		`SYSTEM\CurrentControlSet\Control\Network\{4D36E972-E325-11CE-BFC1-08002BE10318}\%s\Connection`,
		instanceID)
	netKey, err := registry.OpenKey(registry.LOCAL_MACHINE, netPath, registry.READ)
	if err != nil {
		return "", err
	}
	defer netKey.Close()

	name, _, err := netKey.GetStringValue("Name")
	return name, err
}

// ═══════════════════════════════════════════════════════════════════════
//  Adapter listing via SetupDi (replaces Get-NetAdapter)
// ═══════════════════════════════════════════════════════════════════════

func listAdaptersAPI() ([]AdapterInfo, error) {
	h, err := diGetClassDevs(&guidDevclassNet, digcfPresent)
	if err != nil {
		return nil, err
	}
	defer diDestroyDeviceInfoList(h)

	var adapters []AdapterInfo
	var data spDevInfoData
	data.Size = uint32(unsafe.Sizeof(data))

	for i := uint32(0); diEnumDeviceInfo(h, i, &data); i++ {
		connName, err := getConnectionName(h, &data)
		if err != nil || connName == "" {
			continue
		}
		desc := diGetDeviceRegistryProperty(h, &data, spdrpDeviceDesc)
		adapters = append(adapters, AdapterInfo{Name: connName, Description: desc})
	}
	return adapters, nil
}

// ═══════════════════════════════════════════════════════════════════════
//  Adapter status check via SetupDi (crash recovery)
// ═══════════════════════════════════════════════════════════════════════

// isAdapterDisabledAPI checks if any of the named adapters are currently
// disabled. Returns the names of adapters that are disabled.
// Uses SetupDi DN_HAS_PROBLEM / DN_DISABLEABLE device status flags.
func findDisabledAdapters(adapterNames []string) []string {
	if len(adapterNames) == 0 {
		return nil
	}

	h, err := diGetClassDevs(&guidDevclassNet, digcfPresent|0x20) // DIGCF_PRESENT | DIGCF_ALLCLASSES
	if err != nil {
		// Fallback: try without ALLCLASSES flag
		h, err = diGetClassDevs(&guidDevclassNet, digcfPresent)
		if err != nil {
			return nil
		}
	}
	defer diDestroyDeviceInfoList(h)

	nameSet := make(map[string]bool, len(adapterNames))
	for _, n := range adapterNames {
		nameSet[n] = true
	}

	var data spDevInfoData
	data.Size = uint32(unsafe.Sizeof(data))

	// We only need to check if the adapter exists — if it's in DIGCF_PRESENT
	// but was disabled by us, it won't appear. So we check which adapters
	// from the blacklist are NOT present.
	foundPresent := make(map[string]bool)
	for i := uint32(0); diEnumDeviceInfo(h, i, &data); i++ {
		connName, err := getConnectionName(h, &data)
		if err != nil || !nameSet[connName] {
			continue
		}
		foundPresent[connName] = true
	}

	// Adapters not found in PRESENT list are likely disabled.
	var disabled []string
	for _, name := range adapterNames {
		if !foundPresent[name] {
			disabled = append(disabled, name)
		}
	}
	return disabled
}

// ═══════════════════════════════════════════════════════════════════════
//  Adapter disable / enable via SetupDi (replaces Disable/Enable-NetAdapter)
// ═══════════════════════════════════════════════════════════════════════

func setAdapterState(adapterName string, enable bool) error {
	h, err := diGetClassDevs(&guidDevclassNet, digcfPresent)
	if err != nil {
		return err
	}
	defer diDestroyDeviceInfoList(h)

	var data spDevInfoData
	data.Size = uint32(unsafe.Sizeof(data))

	for i := uint32(0); diEnumDeviceInfo(h, i, &data); i++ {
		connName, err := getConnectionName(h, &data)
		if err != nil || connName != adapterName {
			continue
		}

		// Matching adapter found — apply state change.
		var params spPropChangeParams
		params.ClassInstallHeader.Size = uint32(unsafe.Sizeof(params.ClassInstallHeader))
		params.ClassInstallHeader.InstallFunction = difPropertyChange
		params.Scope = dicsFlagGlobal
		if enable {
			params.StateChange = dicsEnable
		} else {
			params.StateChange = dicsDisable
		}

		if err := diSetClassInstallParams(h, &data, &params); err != nil {
			return fmt.Errorf("adapter %q: %w", adapterName, err)
		}
		if err := diCallClassInstaller(difPropertyChange, h, &data); err != nil {
			return fmt.Errorf("adapter %q: %w", adapterName, err)
		}
		return nil
	}

	return fmt.Errorf("adapter %q not found", adapterName)
}

// setAdapterStateBatch enables or disables multiple adapters in a single
// SetupDi enumeration pass, avoiding the overhead of opening a new device
// info set for each adapter.
func setAdapterStateBatch(adapterNames []string, enable bool) error {
	if len(adapterNames) == 0 {
		return nil
	}

	h, err := diGetClassDevs(&guidDevclassNet, digcfPresent)
	if err != nil {
		return err
	}
	defer diDestroyDeviceInfoList(h)

	nameSet := make(map[string]bool, len(adapterNames))
	for _, n := range adapterNames {
		nameSet[n] = true
	}

	var data spDevInfoData
	data.Size = uint32(unsafe.Sizeof(data))

	var errs []error
	remaining := len(nameSet)

	for i := uint32(0); remaining > 0 && diEnumDeviceInfo(h, i, &data); i++ {
		connName, err := getConnectionName(h, &data)
		if err != nil || !nameSet[connName] {
			continue
		}

		var params spPropChangeParams
		params.ClassInstallHeader.Size = uint32(unsafe.Sizeof(params.ClassInstallHeader))
		params.ClassInstallHeader.InstallFunction = difPropertyChange
		params.Scope = dicsFlagGlobal
		if enable {
			params.StateChange = dicsEnable
		} else {
			params.StateChange = dicsDisable
		}

		if err := diSetClassInstallParams(h, &data, &params); err != nil {
			errs = append(errs, fmt.Errorf("adapter %q: %w", connName, err))
		} else if err := diCallClassInstaller(difPropertyChange, h, &data); err != nil {
			errs = append(errs, fmt.Errorf("adapter %q: %w", connName, err))
		} else {
			action := "disabled"
			if enable {
				action = "enabled"
			}
			log.Printf("[WinAPI] %s %q", action, connName)
		}

		delete(nameSet, connName)
		remaining--
	}

	for name := range nameSet {
		errs = append(errs, fmt.Errorf("adapter %q not found", name))
	}

	return errors.Join(errs...)
}

// ═══════════════════════════════════════════════════════════════════════
//  VPN service detection via Service Manager (replaces Get-Service)
// ═══════════════════════════════════════════════════════════════════════

// vpnPatterns contains lowercase substrings to match against service DisplayName
// and ServiceName.
var vpnPatterns = []string{
	"tailscale", "wireguard", "openvpn", "nordvpn", "expressvpn",
	"surfshark", "cloudflare", "zerotier", "ipvanish",
	"protonvpn", "mullvad", "cyberghost", "windscribe", "outline",
}

func matchesVPN(s string) bool {
	lower := strings.ToLower(s)
	for _, p := range vpnPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// matchedVPNPattern returns the first vpnPatterns entry that matches s,
// or "" if none match. Used to group VPN services and adapters by provider.
func matchedVPNPattern(s string) string {
	lower := strings.ToLower(s)
	for _, p := range vpnPatterns {
		if strings.Contains(lower, p) {
			return p
		}
	}
	return ""
}

// vpnAdapterPatterns contains lowercase substrings to match against network
// adapter Description to identify VPN/tunnel virtual adapters.
var vpnAdapterPatterns = []string{
	"tap-windows", "tap adapter", "tun adapter", "tun/tap",
	"ipvanish", "wireguard tunnel", "wintun",
	"nordlynx", "surfshark", "expressvpn",
	"protonvpn", "mullvad", "cyberghost", "windscribe",
	"cloudflare warp", "zerotier", "tailscale",
	"openvpn", "fortinet", "cisco anyconnect",
}

// matchesVPNAdapter returns true if the adapter description matches a known
// VPN/tunnel virtual adapter pattern.
func matchesVPNAdapter(description string) bool {
	lower := strings.ToLower(description)
	for _, p := range vpnAdapterPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// matchedVPNAdapterPattern returns the first vpnAdapterPatterns entry that
// matches the adapter description, or "" if none match. Used for grouping
// multiple adapters from the same VPN provider under one checkbox.
func matchedVPNAdapterPattern(description string) string {
	lower := strings.ToLower(description)
	for _, p := range vpnAdapterPatterns {
		if strings.Contains(lower, p) {
			return p
		}
	}
	return ""
}

// excludedAdapterPatterns lists adapter descriptions that should never
// appear in the GUI. Includes both English and Russian (localized) variants.
var excludedAdapterPatterns = []string{
	// English
	"wi-fi direct", "wifi direct", "microsoft wi-fi direct",
	"microsoft kernel debug", "bluetooth device",
	// Russian (Windows localised descriptions)
	"отладк",      // «Сетевой адаптер с отладкой ядра»
	"bluetooth",   // Bluetooth PAN
}

// isExcludedAdapter returns true if the adapter should be hidden from the GUI.
func isExcludedAdapter(description string) bool {
	lower := strings.ToLower(description)
	for _, p := range excludedAdapterPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// builtinVPNAdapterPatterns matches Windows built-in VPN virtual adapters
// (WAN Miniport / Мини-порт). Both English and Russian variants.
var builtinVPNAdapterPatterns = []string{
	"wan miniport",
	"мини-порт",
}

// isBuiltinVPNAdapter returns true if the adapter is a Windows built-in
// VPN miniport (SSTP, L2TP, PPTP, IKEv2, PPPOE, etc.).
func isBuiltinVPNAdapter(description string) bool {
	lower := strings.ToLower(description)
	for _, p := range builtinVPNAdapterPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func detectVPNServicesAPI() ([]VPNServiceInfo, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("SCM connect: %w", err)
	}
	defer m.Disconnect()

	names, err := m.ListServices()
	if err != nil {
		return nil, fmt.Errorf("ListServices: %w", err)
	}

	var services []VPNServiceInfo
	for _, name := range names {
		// Fast path: check service name before opening the handle.
		// This skips ~95% of OpenService+Config calls.
		if !matchesVPN(name) {
			continue
		}

		s, err := m.OpenService(name)
		if err != nil {
			continue
		}
		cfg, err := s.Config()
		s.Close()
		if err != nil {
			continue
		}
		display := cfg.DisplayName
		if display == "" {
			display = name
		}
		services = append(services, VPNServiceInfo{
			ServiceName: name,
			DisplayName: display,
		})
	}
	return services, nil
}

// ═══════════════════════════════════════════════════════════════════════
//  Service stop / start via Service Manager (replaces Stop/Start-Service)
// ═══════════════════════════════════════════════════════════════════════

func stopServicesAPI(names []string) {
	if len(names) == 0 {
		return
	}
	m, err := mgr.Connect()
	if err != nil {
		log.Printf("[WinAPI] SCM connect: %v", err)
		return
	}
	defer m.Disconnect()

	for _, name := range names {
		s, err := m.OpenService(name)
		if err != nil {
			log.Printf("[WinAPI] open service %q: %v", name, err)
			continue
		}
		if _, err := s.Control(svc.Stop); err != nil {
			log.Printf("[WinAPI] stop %q: %v", name, err)
		}
		s.Close()
	}
}

func startServicesAPI(names []string) {
	if len(names) == 0 {
		return
	}
	m, err := mgr.Connect()
	if err != nil {
		log.Printf("[WinAPI] SCM connect: %v", err)
		return
	}
	defer m.Disconnect()

	for _, name := range names {
		s, err := m.OpenService(name)
		if err != nil {
			log.Printf("[WinAPI] open service %q: %v", name, err)
			continue
		}
		if err := s.Start(); err != nil {
			log.Printf("[WinAPI] start %q: %v", name, err)
		}
		s.Close()
	}
}
