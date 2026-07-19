package main

import (
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// unicodeTheme wraps the built-in dark theme but substitutes the default
// font with a system font that has full Unicode (Cyrillic, CJK, etc.) coverage.
// On Windows we try Segoe UI (ships with every Windows since Vista).
type unicodeTheme struct{ fyne.Theme }

func newUnicodeTheme() fyne.Theme {
	base := theme.DarkTheme()
	fontData := loadSystemFont()
	if fontData == nil {
		// No system font found — fall back to built-in (garbled non-Latin, but functional)
		return base
	}
	return &unicodeTheme{base}
}

// fontResource caches the loaded system font bytes so we only read the file once.
var fontResource fyne.Resource

func loadSystemFont() []byte {
	// Ordered preference list — first match wins.
	candidates := []string{
		`C:\Windows\Fonts\segoeui.ttf`,  // Segoe UI — ships with every modern Windows
		`C:\Windows\Fonts\arial.ttf`,    // Arial — universal fallback
		`C:\Windows\Fonts\tahoma.ttf`,   // Tahoma — older systems
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			fontResource = fyne.NewStaticResource("unicode-font", data)
			return data
		}
	}
	return nil
}

// Font overrides the theme font so Fyne uses our Unicode-capable resource.
func (u *unicodeTheme) Font(style fyne.TextStyle) fyne.Resource {
	if fontResource != nil {
		return fontResource
	}
	return u.Theme.Font(style)
}

// Color, Icon, Size delegate to the wrapped dark theme unchanged.
func (u *unicodeTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return u.Theme.Color(n, v)
}
func (u *unicodeTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return u.Theme.Icon(n)
}
func (u *unicodeTheme) Size(n fyne.ThemeSizeName) float32 {
	return u.Theme.Size(n)
}
