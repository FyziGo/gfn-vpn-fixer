package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// psEscape safely wraps a network adapter name for use inside a PowerShell
// single-quoted string by escaping embedded single-quote characters.
func psEscape(name string) string {
	// In PowerShell single-quoted strings, a literal ' is represented as ''
	return strings.ReplaceAll(name, "'", "''")
}

// runPS runs a PowerShell command and returns its stdout output as a UTF-8 string.
// Forces [Console]::OutputEncoding to UTF-8 so non-ASCII characters (Cyrillic adapter
// names, etc.) come through correctly instead of being mangled by the OEM codepage.
// CREATE_NO_WINDOW (0x08000000) + HideWindow prevent any console flashing on screen.
func runPS(command string) (string, error) {
	// Prefix the actual command with a UTF-8 encoding override.
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
func listAdapters() ([]AdapterInfo, error) {
	out, err := runPS("Get-NetAdapter | ForEach-Object { \"$($_.Name)\t$($_.InterfaceDescription)\" }")
	if err != nil {
		return nil, fmt.Errorf("listAdapters: %w", err)
	}

	var adapters []AdapterInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		info := AdapterInfo{Name: parts[0]}
		if len(parts) > 1 {
			info.Description = strings.TrimSpace(parts[1])
		}
		adapters = append(adapters, info)
	}
	return adapters, nil
}

// VPNServiceInfo holds the Windows service name and display name of a VPN service.
type VPNServiceInfo struct {
	ServiceName string // e.g. "Tailscale"
	DisplayName string // e.g. "Tailscale"
}

// detectVPNServices returns a list of installed Windows services that match
// known VPN/tunnel providers.
func detectVPNServices() ([]VPNServiceInfo, error) {
	cmd := "Get-Service | Where-Object { $_.DisplayName -match " +
		"'Tailscale|WireGuard|OpenVPN|NordVPN|ExpressVPN|Surfshark|Cloudflare.WARP|ZeroTier|IPVanish|ProtonVPN|Mullvad|CyberGhost|Windscribe|Outline' " +
		"} | ForEach-Object { \"$($_.Name)\t$($_.DisplayName)\" }"
	out, err := runPS(cmd)
	if err != nil {
		return nil, fmt.Errorf("detectVPNServices: %w", err)
	}

	var services []VPNServiceInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		info := VPNServiceInfo{ServiceName: parts[0]}
		if len(parts) > 1 {
			info.DisplayName = strings.TrimSpace(parts[1])
		} else {
			info.DisplayName = parts[0]
		}
		services = append(services, info)
	}
	return services, nil
}

// DisableNetworksBatch disables all specified VPN services and network adapters
// using a single PowerShell invocation to drastically reduce execution time.
func DisableNetworksBatch(vpnServices []string, adapters []string) error {
	var sb strings.Builder
	sb.WriteString("$ErrorActionPreference = 'SilentlyContinue';\n")

	if len(vpnServices) > 0 {
		for _, svc := range vpnServices {
			sb.WriteString(fmt.Sprintf("Stop-Service -Name '%s' -Force;\n", psEscape(svc)))
		}
	}

	if len(adapters) > 0 {
		for _, adapter := range adapters {
			escaped := psEscape(adapter)
			sb.WriteString(fmt.Sprintf(`$svc = (Get-NetAdapter -Name '%s').ServiceName; `, escaped))
			sb.WriteString(`if ($svc) { Stop-Service -Name $svc -Force; }; `)
			sb.WriteString(fmt.Sprintf(`Disable-NetAdapter -Name '%s' -Confirm:$false;`+"\n", escaped))
		}
	}

	_, err := runPS(sb.String())
	if err != nil {
		return fmt.Errorf("DisableNetworksBatch: %w", err)
	}
	return nil
}

// EnableNetworksBatch enables all specified network adapters and VPN services
// using a single PowerShell invocation.
func EnableNetworksBatch(vpnServices []string, adapters []string) error {
	var sb strings.Builder
	sb.WriteString("$ErrorActionPreference = 'SilentlyContinue';\n")

	if len(adapters) > 0 {
		for _, adapter := range adapters {
			escaped := psEscape(adapter)
			sb.WriteString(fmt.Sprintf(`Enable-NetAdapter -Name '%s' -Confirm:$false; `, escaped))
			sb.WriteString(fmt.Sprintf(`$svc = (Get-NetAdapter -Name '%s').ServiceName; `, escaped))
			sb.WriteString(`if ($svc) { Start-Service -Name $svc; };`+"\n")
		}
	}

	if len(vpnServices) > 0 {
		for _, svc := range vpnServices {
			sb.WriteString(fmt.Sprintf("Start-Service -Name '%s';\n", psEscape(svc)))
		}
	}

	_, err := runPS(sb.String())
	if err != nil {
		return fmt.Errorf("EnableNetworksBatch: %w", err)
	}
	return nil
}

// ApplyBandwidthLimit creates a Windows QoS policy that throttles the
// GeForceNOW.exe process to the specified bandwidth in Mbps.
// This makes GFN's internal speed test see a stable, capped value.
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
