# Claude.md - Project Knowledge

## System Tray Implementation (Windows)

### Build Command
```bash
go build -ldflags "-H windowsgui" -o clipboard-controller.exe .
```

**Important**: Must use `-H windowsgui` flag to:
- Prevent default console window from appearing
- Allow app to run as a GUI application with system tray
- Console is created manually via `AllocConsole` when needed

### Key Technical Details

#### 1. Icon Format
- Windows system tray requires **ICO format** (not PNG)
- ICO structure:
  - 6-byte header (reserved, type=1, count)
  - 16-byte directory entry per image
  - BITMAPINFOHEADER (40 bytes)
  - Pixel data in BGRA format, bottom-to-top order
  - AND mask for transparency

#### 2. Console Management
Since `-H windowsgui` hides the console, we manually create one:

```go
// AllocConsole creates a new console window
allocConsoleProc := kernel32.NewProc("AllocConsole")
allocConsoleProc.Call()

// Redirect stdout/stderr to the new console
conout, _ := syscall.Open("CONOUT$", syscall.O_RDWR, 0)
os.Stdout = os.NewFile(uintptr(conout), "/dev/stdout")
os.Stderr = os.NewFile(uintptr(conout), "/dev/stderr")
```

#### 3. Disable Close Button
To prevent users from accidentally closing the app via console X button:

```go
getSystemMenu := user32.NewProc("GetSystemMenu")
enableMenuItem := user32.NewProc("EnableMenuItem")

hMenu, _, _ := getSystemMenu.Call(consoleWindow, 0)
enableMenuItem.Call(hMenu, SC_CLOSE, MF_BYCOMMAND|MF_DISABLED|MF_GRAYED)
```

**Note**: This only works on console windows created by `AllocConsole`, not Windows Terminal.

#### 4. Console Visibility Control
```go
// Hide console
showWindow.Call(consoleWindow, SW_HIDE)

// Show console
showWindow.Call(consoleWindow, SW_SHOW)

// Check if visible
isVisible := user32.NewProc("IsWindowVisible")
ret, _, _ := isVisible.Call(consoleWindow)
visible := ret != 0
```

### Behavior Summary
1. **App starts**: Console window appears + Tray icon appears
2. **X button disabled**: User cannot close via console X button
3. **Tray menu "Hide Console"**: Hides console, app continues running
4. **Tray menu "Show Console"**: Shows console again
5. **Tray menu "Exit"**: Properly shuts down the application

### Libraries Used
- `github.com/getlantern/systray` - Cross-platform system tray

### Files
- `tray/tray.go` - Main tray logic
- `tray/icon.go` - ICO icon generation
- `tray/console_windows.go` - Windows-specific console APIs
- `tray/console_other.go` - Non-Windows stubs
- `tray/utils.go` - Utility functions (open logs folder)
