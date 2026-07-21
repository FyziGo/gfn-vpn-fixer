package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"syscall"
)

// psEscape safely wraps a value for use inside a PowerShell single-quoted string.
func psEscape(name string) string {
	// In PowerShell single-quoted strings, a literal ' is represented as ''
	return strings.ReplaceAll(name, "'", "''")
}

// runPS runs a PowerShell command and returns its stdout output as a UTF-8 string.
// Retained only for QoS bandwidth management and Task Scheduler operations
// where no practical direct WinAPI equivalent exists.
// CREATE_NO_WINDOW (0x08000000) + HideWindow prevent any console flashing.
func runPS(command string) (string, error) {
	wrapped := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + command
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", wrapped)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	out, err := cmd.Output()
	if err != nil {
		// Include stderr in the error when possible
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("powershell error: %s", string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// AdapterInfo holds the friendly name and hardware description of a network adapter.
type AdapterInfo struct {
	Name        string // e.g. "Ethernet", "Wi-Fi", "Tailscale"
	Description string // e.g. "Realtek Gaming 2.5GbE Family Controller"
}

// listAdapters returns info about all network adapters visible to Windows.
// Uses SetupDi API directly — no PowerShell process is spawned.
func listAdapters() ([]AdapterInfo, error) {
	adapters, err := listAdaptersAPI()
	if err != nil {
		return nil, fmt.Errorf("listAdapters: %w", err)
	}
	return adapters, nil
}

// VPNServiceInfo holds the Windows service name and display name of a VPN service.
type VPNServiceInfo struct {
	ServiceName string // e.g. "Tailscale"
	DisplayName string // e.g. "Tailscale"
}

// detectVPNServices returns a list of installed Windows services that match
// known VPN/tunnel providers. Uses Service Manager — no PowerShell.
func detectVPNServices() ([]VPNServiceInfo, error) {
	services, err := detectVPNServicesAPI()
	if err != nil {
		return nil, fmt.Errorf("detectVPNServices: %w", err)
	}
	return services, nil
}

// DisableNetworksBatch disables all specified VPN services and network adapters
// using direct Windows API calls (Service Manager + SetupDi).
func DisableNetworksBatch(vpnServices []string, adapters []string) error {
	// Stop VPN services first (non-blocking signal).
	stopServicesAPI(vpnServices)

	// Disable network adapters via SetupDi — single handle, one enumeration pass.
	if err := setAdapterStateBatch(adapters, false); err != nil {
		return fmt.Errorf("DisableNetworksBatch: %w", err)
	}
	return nil
}

// EnableNetworksBatch enables all specified network adapters and VPN services
// using direct Windows API calls.
func EnableNetworksBatch(vpnServices []string, adapters []string) error {
	// Enable network adapters via SetupDi — single handle, one enumeration pass.
	if err := setAdapterStateBatch(adapters, true); err != nil {
		log.Printf("[Batch] WARNING enable adapters: %v", err)
	}

	// Start VPN services.
	startServicesAPI(vpnServices)
	return nil
}

// ApplyBandwidthLimit creates a Windows QoS policy that throttles the
// GeForceNOW.exe process to the specified bandwidth in Mbps.
// Uses PowerShell because the Traffic Control API has no practical Go wrapper.
func ApplyBandwidthLimit(mbps int) error {
	if mbps <= 0 {
		return nil
	}
	bps := int64(mbps) * 1_000_000
	cmd := fmt.Sprintf(
		"Remove-NetQosPolicy -Name 'GFN_BandwidthCap' -Confirm:$false -ErrorAction SilentlyContinue; "+
			"New-NetQosPolicy -Name 'GFN_BandwidthCap' -AppPathNameMatchCondition 'GeForceNOW.exe' "+
			"-ThrottleRateActionBitsPerSecond %d -PolicyStore ActiveStore",
		bps,
	)
	_, err := runPS(cmd)
	if err != nil {
		return fmt.Errorf("ApplyBandwidthLimit: %w", err)
	}
	return nil
}

// RemoveBandwidthLimit removes the QoS policy created by ApplyBandwidthLimit.
func RemoveBandwidthLimit() error {
	_, err := runPS("Remove-NetQosPolicy -Name 'GFN_BandwidthCap' -Confirm:$false -ErrorAction SilentlyContinue")
	if err != nil {
		return fmt.Errorf("RemoveBandwidthLimit: %w", err)
	}
	return nil
}
