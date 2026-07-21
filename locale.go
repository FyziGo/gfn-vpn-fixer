//go:build windows

package main

import "golang.org/x/sys/windows"

// Locale holds all user-facing translatable strings.
type Locale struct {
	// GUI — Loading
	LoadingText string

	// GUI — Adapters card
	AdaptersTitle    string
	AdaptersSubtitle string
	NoAdapters       string

	// GUI — VPN card
	VPNTitle    string
	VPNSubtitle string
	NoVPN       string

	// GUI — VPN unified (expand/collapse adapter list)
	VPNAdaptersExpand   string // fmt with %d
	VPNAdaptersCollapse string // fmt with %d
	BuiltinVPNName      string // display name for Windows built-in VPN group

	// GUI — GFN Path card
	GFNPathTitle    string
	GFNPathSubtitle string

	// GUI — Bandwidth card
	BandwidthTitle    string
	BandwidthSubtitle string
	BandwidthErrInt   string
	BandwidthErrNeg   string

	// GUI — Checkboxes
	LaunchGFNLabel string

	// GUI — Autostart card
	AutostartLabel    string
	AutostartTitle    string
	AutostartSubtitle string

	// GUI — Save
	SaveButton     string
	SaveError      string
	AutostartError string
	SaveSuccess    string

	// Tray menu
	TrayTooltip     string
	StatusWaiting   string
	StatusRunning   string
	StatusBandwidth string // fmt with %d
	MenuLaunch      string
	MenuLaunchTip   string
	MenuSettings    string
	MenuSettingsTip string
	MenuReload      string
	MenuReloadTip   string
	MenuExit        string
	MenuExitTip     string
	Exiting         string
	ReloadOK        string
	ReloadFail      string

	// Privilege dialog
	PrivilegeTitle   string
	PrivilegeMessage string

	// Autostart errors (format strings for fmt.Errorf)
	ErrExePath    string
	ErrTaskCreate string
}

// L is the global locale instance, set once at startup via initLocale.
var L Locale

// initLocale detects the system UI language and populates L.
func initLocale() {
	if isSystemRussian() {
		L = localeRU
	} else {
		L = localeEN
	}
}

// isSystemRussian returns true if the Windows UI language is Russian.
func isSystemRussian() bool {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("GetUserDefaultUILanguage")
	langID, _, _ := proc.Call()
	// LANG_RUSSIAN primary language sub-tag = 0x19
	return langID&0x3FF == 0x19
}

var localeRU = Locale{
	LoadingText:       "Поиск сетевых адаптеров и служб...\n(Это может занять пару секунд)",
	AdaptersTitle:     "Сетевые адаптеры для отключения",
	AdaptersSubtitle:  "Отметьте адаптеры, которые будут отключены при запуске GFN.",
	NoAdapters:        "(Адаптеры не найдены)",
	VPNTitle:            "VPN-сервисы",
	VPNSubtitle:         "Службы и адаптеры VPN, которые будут отключены при запуске GFN.",
	NoVPN:               "(VPN-сервисы не обнаружены)",
	VPNAdaptersExpand:   "▶ Адаптеры (%d)",
	VPNAdaptersCollapse: "▼ Адаптеры (%d)",
	BuiltinVPNName:      "Встроенный VPN Windows (L2TP, PPTP, SSTP, IKEv2)",
	GFNPathTitle:      "Путь к GeForce NOW",
	GFNPathSubtitle:   "Полный путь к исполняемому файлу GeForceNOW.exe.",
	BandwidthTitle:    "Ограничение скорости (Мбит/с)",
	BandwidthSubtitle: "Ограничивает пропускную способность для GFN через Windows QoS.\n0 = без ограничений.",
	BandwidthErrInt:   "введите целое число",
	BandwidthErrNeg:   "значение не может быть отрицательным",
	LaunchGFNLabel:    "Запускать GeForce NOW вместе с программой",
	AutostartLabel:    "Запускать автоматически при старте Windows (без интерфейса)",
	AutostartTitle:    "Автозапуск",
	AutostartSubtitle: "Создаёт задачу в Планировщике Windows с правами администратора.\nЗапрос UAC не появляется.",
	SaveButton:        "💾  Сохранить и выйти",
	SaveError:         "❌ Ошибка сохранения: ",
	AutostartError:    "❌ Ошибка автозапуска: ",
	SaveSuccess:       "✅ Конфигурация сохранена!",

	TrayTooltip:     "GFN VPN FIXER — активен",
	StatusWaiting:   "Состояние: ожидание запуска GFN",
	StatusRunning:   "Состояние: GFN запущен, адаптеры отключены",
	StatusBandwidth: "Состояние: GFN запущен, лимит %d Мбит/с",
	MenuLaunch:      "🚀 Запустить GeForce NOW",
	MenuLaunchTip:   "Запустить GFN вручную",
	MenuSettings:    "⚙  Настройки",
	MenuSettingsTip: "Открыть окно настройки адаптеров",
	MenuReload:      "🔄 Перезагрузить конфиг",
	MenuReloadTip:   "Перечитать config.json без перезапуска",
	MenuExit:        "✖  Выйти",
	MenuExitTip:     "Включить адаптеры и завершить работу",
	Exiting:         "Выход...",
	ReloadOK:        "✔ Конфиг перезагружен",
	ReloadFail:      "✖ Ошибка загрузки конфига",

	PrivilegeTitle:   "GFN VPN FIXER — Требуются права администратора",
	PrivilegeMessage: "Не удалось получить права администратора.\n\nПодтвердите запрос UAC или запустите .exe от имени администратора вручную.",
	ErrExePath:       "не удалось получить путь к exe: %w",
	ErrTaskCreate:    "не удалось создать задачу планировщика: %w",
}

var localeEN = Locale{
	LoadingText:       "Searching for network adapters and services...\n(This may take a few seconds)",
	AdaptersTitle:     "Network Adapters to Disable",
	AdaptersSubtitle:  "Check adapters that should be disabled while GFN is running.",
	NoAdapters:        "(No adapters found)",
	VPNTitle:            "VPN Services",
	VPNSubtitle:         "VPN services and adapters that will be disabled when GFN launches.",
	NoVPN:               "(No VPN services detected)",
	VPNAdaptersExpand:   "▶ Adapters (%d)",
	VPNAdaptersCollapse: "▼ Adapters (%d)",
	BuiltinVPNName:      "Windows Built-in VPN (L2TP, PPTP, SSTP, IKEv2)",
	GFNPathTitle:      "GeForce NOW Path",
	GFNPathSubtitle:   "Full path to the GeForceNOW.exe executable.",
	BandwidthTitle:    "Bandwidth Limit (Mbps)",
	BandwidthSubtitle: "Limits bandwidth for GFN via Windows QoS policy.\n0 = unlimited.",
	BandwidthErrInt:   "enter a whole number",
	BandwidthErrNeg:   "value cannot be negative",
	LaunchGFNLabel:    "Launch GeForce NOW with the program",
	AutostartLabel:    "Start automatically on Windows startup (headless)",
	AutostartTitle:    "Autostart",
	AutostartSubtitle: "Creates a Task Scheduler entry with admin privileges.\nNo UAC prompt on login.",
	SaveButton:        "💾  Save & Exit",
	SaveError:         "❌ Error saving config: ",
	AutostartError:    "❌ Autostart error: ",
	SaveSuccess:       "✅ Config saved!",

	TrayTooltip:     "GFN VPN FIXER — active",
	StatusWaiting:   "Status: waiting for GFN",
	StatusRunning:   "Status: GFN running, adapters disabled",
	StatusBandwidth: "Status: GFN running, limit %d Mbps",
	MenuLaunch:      "🚀 Launch GeForce NOW",
	MenuLaunchTip:   "Launch GFN manually",
	MenuSettings:    "⚙  Settings",
	MenuSettingsTip: "Open adapter settings window",
	MenuReload:      "🔄 Reload Config",
	MenuReloadTip:   "Re-read config.json without restarting",
	MenuExit:        "✖  Exit",
	MenuExitTip:     "Enable adapters and quit",
	Exiting:         "Exiting...",
	ReloadOK:        "✔ Config reloaded",
	ReloadFail:      "✖ Config reload failed",

	PrivilegeTitle:   "GFN VPN FIXER — Administrator Required",
	PrivilegeMessage: "Could not obtain administrator privileges.\n\nPlease accept the UAC prompt or run the .exe as administrator manually.",
	ErrExePath:       "could not get executable path: %w",
	ErrTaskCreate:    "could not create scheduled task: %w",
}
