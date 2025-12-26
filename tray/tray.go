package tray

import (
	"sync"

	"github.com/getlantern/systray"
	"github.com/rs/zerolog/log"
)

// TrayApp manages the system tray functionality
type TrayApp struct {
	port       int
	onExit     func()
	ready      chan struct{}
	menuStatus *systray.MenuItem
	menuShow   *systray.MenuItem
	isRunning  bool
	mu         sync.Mutex
}

// New creates a new TrayApp
func New(port int, onExit func()) *TrayApp {
	return &TrayApp{
		port:   port,
		onExit: onExit,
		ready:  make(chan struct{}),
	}
}

// Run starts the system tray (blocking)
func (t *TrayApp) Run() {
	t.mu.Lock()
	t.isRunning = true
	t.mu.Unlock()

	systray.Run(t.onReady, t.onQuit)
}

// Ready returns a channel that closes when tray is ready
func (t *TrayApp) Ready() <-chan struct{} {
	return t.ready
}

// UpdateStatus updates the status display in menu
func (t *TrayApp) UpdateStatus(toolCount, queueLength int) {
	if t.menuStatus != nil {
		t.menuStatus.SetTitle(formatStatus(toolCount, queueLength))
	}
}

// IsRunning returns true if tray is running
func (t *TrayApp) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.isRunning
}

func (t *TrayApp) onReady() {
	// Set console title and disable X button (user must use tray menu)
	SetConsoleTitle("Clipboard Controller")
	DisableCloseButton()

	// Set icon and tooltip
	systray.SetIcon(getIcon())
	systray.SetTitle("Clipboard Controller")
	systray.SetTooltip("Clipboard Controller - Running on port " + itoa(t.port))

	// Add menu items
	t.menuStatus = systray.AddMenuItem("Tools: 0 | Queue: 0", "Current status")
	t.menuStatus.Disable()

	systray.AddSeparator()

	t.menuShow = systray.AddMenuItem("Hide Console", "Show/hide console window")
	menuOpen := systray.AddMenuItem("Open Logs Folder", "Open logs folder")

	systray.AddSeparator()

	menuExit := systray.AddMenuItem("Exit", "Quit the application completely")

	// Signal ready
	close(t.ready)

	log.Debug().Int("port", t.port).Msg("System tray initialized")

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-t.menuShow.ClickedCh:
				if IsConsoleVisible() {
					HideConsole()
					t.menuShow.SetTitle("Show Console")
				} else {
					ShowConsole()
					t.menuShow.SetTitle("Hide Console")
				}
			case <-menuOpen.ClickedCh:
				openLogsFolder()
			case <-menuExit.ClickedCh:
				log.Info().Msg("Exit requested from tray")
				systray.Quit()
			}
		}
	}()
}

func (t *TrayApp) onQuit() {
	t.mu.Lock()
	t.isRunning = false
	t.mu.Unlock()

	log.Info().Msg("System tray quitting")
	if t.onExit != nil {
		t.onExit()
	}
}

// Quit exits the systray
func (t *TrayApp) Quit() {
	systray.Quit()
}

// SetMenuShowTitle updates the show/hide menu item title
func (t *TrayApp) SetMenuShowTitle(title string) {
	if t.menuShow != nil {
		t.menuShow.SetTitle(title)
	}
}

func formatStatus(toolCount, queueLength int) string {
	return "Tools: " + itoa(toolCount) + " | Queue: " + itoa(queueLength)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
