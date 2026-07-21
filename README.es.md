# Nvidia Geforce Now VPN FIXER

[English](README.md) | [Русский](README.ru.md) | **Español**

Utilidad para Windows que desactiva automáticamente los adaptadores de red y servicios VPN especificados mientras GeForce NOW está en ejecución, y los reactiva cuando GFN se cierra. Esto evita que el software VPN/túnel (Tailscale, ZeroTier, WireGuard, etc.) interfiera con el streaming de GeForce NOW.

## Características

- **Gestión automática de red** — monitoriza el proceso `GeForceNOW.exe` y activa/desactiva adaptadores y VPNs en tiempo real
- **Límite de ancho de banda** — limita la velocidad de red visible para GFN mediante la política QoS de Windows para estabilizar el bitrate
- **Bandeja del sistema** — se ejecuta silenciosamente en segundo plano con un icono en la bandeja (verde = GFN activo, gris = en espera)
- **Interfaz de configuración** — ventana de configuración basada en Fyne para seleccionar adaptadores, servicios VPN y límite de ancho de banda
- **Inicio automático** — integración con el Programador de tareas de Windows con privilegios elevados (sin solicitud UAC al iniciar sesión)
- **Sin parpadeo de consola** — todos los comandos PowerShell se ejecutan en ventanas ocultas

## Requisitos

- Windows 10/11 (se recomienda Pro o superior para el límite de ancho de banda QoS)
- Privilegios de administrador (solicitud UAC en el primer inicio)

## Uso

```powershell
# Abrir la interfaz de configuración (primer inicio o para cambiar ajustes)
.\gfn-vpn-fixer.exe --setup

# Modo en segundo plano (operación normal, se ejecuta en la bandeja del sistema)
.\gfn-vpn-fixer.exe
```

## Compilación desde el código fuente

**Requisitos:**
- Go 1.22+
- MinGW-w64 GCC (`CGO_ENABLED=1`, requerido por Fyne)

```powershell
# Compilación rápida
.\build.ps1

# Compilación de lanzamiento (sin símbolos de depuración)
.\build.ps1 -Release
```

## Configuración

Los ajustes se almacenan en `config.json` junto al ejecutable:

```json
{
    "blacklist": ["Nombre del adaptador 1", "Nombre del adaptador 2"],
    "gfn_path": "C:\\...\\GeForceNOW.exe",
    "launch_gfn": false,
    "vpn_services": ["Tailscale", "ZeroTierOneService"],
    "bandwidth_limit_mbps": 100
}
```

| Campo | Descripción |
|-------|-------------|
| `blacklist` | Nombres de los adaptadores de red a desactivar |
| `gfn_path` | Ruta al ejecutable GeForceNOW.exe |
| `launch_gfn` | Iniciar GFN automáticamente junto con el programa |
| `vpn_services` | Nombres de los servicios de Windows a detener |
| `bandwidth_limit_mbps` | Límite de ancho de banda en Mbps (0 = sin límite) |

## Cómo funciona

1. Al iniciarse, verifica los privilegios de administrador (se relanza con UAC si es necesario)
2. Si se usa `--setup` o no existe configuración → se abre la interfaz gráfica
3. De lo contrario → modo en segundo plano con icono en la bandeja del sistema
4. Monitoriza `GeForceNOW.exe` cada 2 segundos mediante la API de Windows (`CreateToolhelp32Snapshot`)
5. Cuando se detecta GFN:
   - Detiene los servicios VPN configurados
   - Desactiva los adaptadores de red en la lista negra
   - Aplica el límite de ancho de banda QoS (si está configurado)
6. Cuando GFN se cierra → revierte todos los cambios

## Licencia

MIT
