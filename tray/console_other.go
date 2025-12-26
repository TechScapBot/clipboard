//go:build !windows

package tray

// HideConsole hides the console window (no-op on non-Windows)
func HideConsole() {}

// ShowConsole shows the console window (no-op on non-Windows)
func ShowConsole() {}

// IsConsoleVisible returns true on non-Windows
func IsConsoleVisible() bool {
	return true
}

// SetConsoleTitle is a no-op on non-Windows
func SetConsoleTitle(title string) {}

// DisableCloseButton is a no-op on non-Windows
func DisableCloseButton() {}

// InitConsole is a no-op on non-Windows
func InitConsole() {}
