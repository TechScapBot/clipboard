package service

import (
	"sync"
	"time"

	"clipboard-controller/config"

	"github.com/rs/zerolog/log"
)

// BackgroundJobs manages all background goroutines
type BackgroundJobs struct {
	config       *config.Config
	toolRegistry *ToolRegistry
	lockManager  *LockManager

	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewBackgroundJobs creates a new BackgroundJobs manager
func NewBackgroundJobs(cfg *config.Config, tr *ToolRegistry, lm *LockManager) *BackgroundJobs {
	return &BackgroundJobs{
		config:       cfg,
		toolRegistry: tr,
		lockManager:  lm,
		stopChan:     make(chan struct{}),
	}
}

// Start starts all background jobs
func (bg *BackgroundJobs) Start() {
	log.Info().Msg("Starting background jobs")

	// Heartbeat checker - every 30 seconds
	bg.wg.Add(1)
	go bg.runHeartbeatChecker()

	// Lock expiry checker - every 1 second
	bg.wg.Add(1)
	go bg.runLockExpiryChecker()

	// Ticket TTL checker - every 5 seconds
	bg.wg.Add(1)
	go bg.runTicketTTLChecker()

	// Grace period checker - every 1 second
	bg.wg.Add(1)
	go bg.runGracePeriodChecker()

	log.Info().Msg("Background jobs started")
}

// Stop stops all background jobs gracefully
func (bg *BackgroundJobs) Stop() {
	log.Info().Msg("Stopping background jobs")

	close(bg.stopChan)
	bg.wg.Wait()

	log.Info().Msg("Background jobs stopped")
}

// runHeartbeatChecker checks for expired heartbeats every 30 seconds
func (bg *BackgroundJobs) runHeartbeatChecker() {
	defer bg.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Debug().Msg("Heartbeat checker started")

	for {
		select {
		case <-bg.stopChan:
			log.Debug().Msg("Heartbeat checker stopped")
			return
		case <-ticker.C:
			bg.checkHeartbeats()
		}
	}
}

// checkHeartbeats marks tools as offline if heartbeat expired
func (bg *BackgroundJobs) checkHeartbeats() {
	offlineTools := bg.toolRegistry.CheckAndMarkOffline()

	if len(offlineTools) > 0 {
		log.Warn().
			Int("count", len(offlineTools)).
			Strs("tools", offlineTools).
			Msg("Tools marked offline due to heartbeat timeout")

		// Remove tickets for offline tools
		for _, toolID := range offlineTools {
			removed := bg.lockManager.RemoveToolTickets(toolID)
			if len(removed) > 0 {
				log.Info().
					Str("tool_id", toolID).
					Int("count", len(removed)).
					Msg("Removed tickets for offline tool")
			}
		}
	}
}

// runLockExpiryChecker checks for expired locks every 1 second
func (bg *BackgroundJobs) runLockExpiryChecker() {
	defer bg.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	log.Debug().Msg("Lock expiry checker started")

	for {
		select {
		case <-bg.stopChan:
			log.Debug().Msg("Lock expiry checker stopped")
			return
		case <-ticker.C:
			bg.checkLockExpiry()
		}
	}
}

// checkLockExpiry expires locks that exceeded max duration
func (bg *BackgroundJobs) checkLockExpiry() {
	expired := bg.lockManager.CheckLockExpiry()

	if expired != nil {
		log.Warn().
			Str("ticket_id", expired.TicketID).
			Str("tool_id", expired.ToolID).
			Msg("Lock force expired due to max duration")
	}
}

// runTicketTTLChecker checks for expired tickets every 5 seconds
func (bg *BackgroundJobs) runTicketTTLChecker() {
	defer bg.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Debug().Msg("Ticket TTL checker started")

	for {
		select {
		case <-bg.stopChan:
			log.Debug().Msg("Ticket TTL checker stopped")
			return
		case <-ticker.C:
			bg.checkTicketTTL()
		}
	}
}

// checkTicketTTL expires waiting tickets that exceeded TTL
func (bg *BackgroundJobs) checkTicketTTL() {
	expired := bg.lockManager.ExpireWaitingTickets()

	if len(expired) > 0 {
		log.Warn().
			Int("count", len(expired)).
			Msg("Tickets expired due to TTL")
	}
}

// runGracePeriodChecker checks grace period every 1 second
func (bg *BackgroundJobs) runGracePeriodChecker() {
	defer bg.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	log.Debug().Msg("Grace period checker started")

	for {
		select {
		case <-bg.stopChan:
			log.Debug().Msg("Grace period checker stopped")
			return
		case <-ticker.C:
			bg.checkGracePeriod()
		}
	}
}

// checkGracePeriod expires locks where holder didn't poll in time
func (bg *BackgroundJobs) checkGracePeriod() {
	expired := bg.lockManager.CheckGracePeriod()

	if expired != nil {
		log.Warn().
			Str("ticket_id", expired.TicketID).
			Str("tool_id", expired.ToolID).
			Msg("Lock expired due to grace period")
	}
}
