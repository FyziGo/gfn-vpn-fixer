# GFN-Net-Wrapper

Windows utility that automatically disables specified network adapters and VPN services while GeForce NOW is running, then re-enables them when GFN exits. This prevents VPN/tunnel software (Tailscale, ZeroTier, WireGuard, etc.) from interfering with GeForce NOW's streaming.

## Features

- **Automatic network management** — monitors `GeForceNOW.exe` process and toggles adapters/VPNs on the fly
- **Bandwidth limiting** — caps network speed visible to GFN via Windows QoS policy to stabilize bitrate
- **System tray** — runs silently in the background with a tray icon (green = GFN active, grey = idle)
- **GUI Setup** — Fyne-based settings window for selecting adapters, VPN services, and bandwidth limit
- **Autostart** — Task Scheduler integration with elevated privileges (no UAC prompt on login)
- **Zero console flash** — all PowerShell commands run with hidden windows

## Requirements

- Windows 10/11 (Pro or higher recommended for QoS bandwidth limiting)
- Administrator privileges (UAC prompt on first launch)

## Usage

```powershell
# Open the setup GUI (first run, or to change settings)
.\gfn-net-wrapper.exe --setup

# Headless mode (normal operation, runs in system tray)
.\gfn-net-wrapper.exe
```

## Building from source

**Requirements:**
- Go 1.22+
- MinGW-w64 GCC (`CGO_ENABLED=1`, required by Fyne)

```powershell
# Quick build
.\build.ps1

# Release build (stripped debug symbols)
.\build.ps1 -Release
```

## Configuration

Settings are stored in `config.json` next to the executable:

```json
{
    "blacklist": ["Adapter Name 1", "Adapter Name 2"],
    "gfn_path": "C:\\...\\GeForceNOW.exe",
    "launch_gfn": false,
    "vpn_services": ["Tailscale", "ZeroTierOneService"],
    "bandwidth_limit_mbps": 100
}
```

| Field | Description |
|-------|-------------|
| `blacklist` | Network adapter friendly names to disable |
| `gfn_path` | Path to GeForceNOW.exe |
| `launch_gfn` | Auto-launch GFN when wrapper starts |
| `vpn_services` | Windows service names to stop |
| `bandwidth_limit_mbps` | Bandwidth cap in Mbps (0 = unlimited) |

## How it works

1. On launch, checks for admin rights (re-launches with UAC if needed)
2. If `--setup` or no config exists → opens GUI
3. Otherwise → headless mode with system tray icon
4. Monitors `GeForceNOW.exe` every 2 seconds via Windows API (`CreateToolhelp32Snapshot`)
5. When GFN detected:
   - Stops configured VPN services
   - Disables blacklisted network adapters
   - Applies QoS bandwidth limit (if configured)
6. When GFN exits → reverses all changes

## License

MIT
