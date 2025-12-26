package service

import (
	"errors"
	"sync"
	"time"

	"clipboard-controller/config"
	"clipboard-controller/model"

	"github.com/rs/zerolog/log"
)

var (
	ErrTicketNotFound   = errors.New("ticket not found")
	ErrNotLockHolder    = errors.New("ticket is not the current lock holder")
	ErrExtendDisabled   = errors.New("lock extend is disabled")
	ErrMaxExtendReached = errors.New("maximum extend count reached")
)

// EventLogger interface for logging lock events
type EventLogger interface {
	LogLockRequested(ticketID, toolID, threadID string, queuePosition int)
	LogLockGranted(ticketID, toolID, threadID string, waitDurationMs int64)
	LogLockReleased(ticketID, toolID, threadID string, holdDurationMs int64)
	LogLockExpired(ticketID, toolID, threadID, reason string, holdDurationMs int64)
	LogLockExtended(ticketID, toolID, threadID string, extendCount int)
	LogTicketExpired(ticketID, toolID, threadID, reason string)
	LogToolRegistered(toolID string)
	LogToolHeartbeat(toolID string)
	LogToolOffline(toolID, reason string)
	LogToolUnregistered(toolID string)
	IncrementRequests()
	IncrementErrors()
}

// LockManager manages the lock queue and current lock
type LockManager struct {
	mu           sync.Mutex
	queue        []*model.Ticket          // FIFO queue of waiting tickets
	currentLock  *model.Ticket            // Currently granted ticket
	tickets      map[string]*model.Ticket // Quick lookup by ticket_id
	threadKeys   map[string]string        // Map of tool:thread -> ticket_id
	config       *config.Config
	toolRegistry *ToolRegistry
	eventLogger  EventLogger
}

// NewLockManager creates a new LockManager
func NewLockManager(cfg *config.Config, tr *ToolRegistry) *LockManager {
	return &LockManager{
		queue:        make([]*model.Ticket, 0),
		tickets:      make(map[string]*model.Ticket),
		threadKeys:   make(map[string]string),
		config:       cfg,
		toolRegistry: tr,
	}
}

// SetEventLogger sets the event logger for lock events
func (lm *LockManager) SetEventLogger(el EventLogger) {
	lm.eventLogger = el
}

// RequestLock creates a new ticket for a lock request
func (lm *LockManager) RequestLock(toolID, threadID string) (*model.Ticket, int, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Check if tool is online
	if !lm.toolRegistry.IsOnline(toolID) {
		log.Debug().
			Str("tool_id", toolID).
			Msg("Lock request from offline tool")
		return nil, 0, ErrToolOffline
	}

	// Check for existing ticket for this tool+thread
	key := toolID + ":" + threadID
	if existingTicketID, ok := lm.threadKeys[key]; ok {
		if ticket, exists := lm.tickets[existingTicketID]; exists {
			if ticket.IsWaiting() || ticket.IsGranted() {
				// Return existing ticket
				position := lm.getQueuePosition(ticket.TicketID)
				log.Debug().
					Str("ticket_id", ticket.TicketID).
					Str("tool_id", toolID).
					Str("thread_id", threadID).
					Int("position", position).
					Msg("Returning existing ticket")
				return ticket, position, nil
			}
		}
	}

	// Create new ticket
	ticket := model.NewTicket(toolID, threadID)
	lm.tickets[ticket.TicketID] = ticket
	lm.threadKeys[key] = ticket.TicketID
	lm.queue = append(lm.queue, ticket)

	position := len(lm.queue)

	log.Info().
		Str("ticket_id", ticket.TicketID).
		Str("tool_id", toolID).
		Str("thread_id", threadID).
		Int("queue_position", position).
		Msg("Lock requested, ticket created")

	// Log event
	if lm.eventLogger != nil {
		lm.eventLogger.LogLockRequested(ticket.TicketID, toolID, threadID, position)
	}

	// Try to grant immediately if no current lock
	lm.tryGrantNext()

	// Recalculate position after potential grant
	position = lm.getQueuePosition(ticket.TicketID)

	return ticket, position, nil
}

// CheckLock checks the status of a ticket and updates poll time
func (lm *LockManager) CheckLock(ticketID string) (*model.Ticket, int, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	ticket, ok := lm.tickets[ticketID]
	if !ok {
		return nil, 0, ErrTicketNotFound
	}

	// Update poll time (for TTL reset if enabled)
	if lm.config.TicketTTLOnPoll && ticket.IsWaiting() {
		ticket.UpdatePollTime()
	}

	// Also update for granted tickets to track activity
	if ticket.IsGranted() {
		ticket.UpdatePollTime()
	}

	position := lm.getQueuePosition(ticketID)

	log.Debug().
		Str("ticket_id", ticketID).
		Str("status", string(ticket.Status)).
		Int("position", position).
		Msg("Lock check")

	return ticket, position, nil
}

// ReleaseLock releases the current lock
func (lm *LockManager) ReleaseLock(ticketID string) (*model.Ticket, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	ticket, ok := lm.tickets[ticketID]
	if !ok {
		return nil, ErrTicketNotFound
	}

	if lm.currentLock == nil || lm.currentLock.TicketID != ticketID {
		return nil, ErrNotLockHolder
	}

	holdDuration := ticket.HoldDuration()
	ticket.Release()
	lm.currentLock = nil

	// Cleanup
	lm.cleanupTicket(ticket)

	log.Info().
		Str("ticket_id", ticketID).
		Str("tool_id", ticket.ToolID).
		Str("thread_id", ticket.ThreadID).
		Dur("hold_duration", holdDuration).
		Msg("Lock released")

	// Log event
	if lm.eventLogger != nil {
		lm.eventLogger.LogLockReleased(ticketID, ticket.ToolID, ticket.ThreadID, holdDuration.Milliseconds())
	}

	// Try to grant next in queue
	lm.tryGrantNext()

	return ticket, nil
}

// ExtendLock extends the current lock duration
func (lm *LockManager) ExtendLock(ticketID string) (*model.Ticket, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if !lm.config.LockExtendable {
		return nil, ErrExtendDisabled
	}

	ticket, ok := lm.tickets[ticketID]
	if !ok {
		return nil, ErrTicketNotFound
	}

	if lm.currentLock == nil || lm.currentLock.TicketID != ticketID {
		return nil, ErrNotLockHolder
	}

	if ticket.ExtendCount >= lm.config.LockExtendMax {
		return nil, ErrMaxExtendReached
	}

	extendDuration := time.Duration(lm.config.LockMaxDuration) * time.Second
	ticket.Extend(extendDuration)

	log.Info().
		Str("ticket_id", ticketID).
		Int("extend_count", ticket.ExtendCount).
		Time("new_expires_at", ticket.ExpiresAt).
		Msg("Lock extended")

	// Log event
	if lm.eventLogger != nil {
		lm.eventLogger.LogLockExtended(ticketID, ticket.ToolID, ticket.ThreadID, ticket.ExtendCount)
	}

	return ticket, nil
}

// ExpireCurrentLock force expires the current lock
func (lm *LockManager) ExpireCurrentLock(reason string) *model.Ticket {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.currentLock == nil {
		return nil
	}

	ticket := lm.currentLock
	holdDuration := ticket.HoldDuration()
	ticket.Expire()
	lm.currentLock = nil

	// Cleanup
	lm.cleanupTicket(ticket)

	log.Warn().
		Str("ticket_id", ticket.TicketID).
		Str("tool_id", ticket.ToolID).
		Str("reason", reason).
		Msg("Lock expired")

	// Log event
	if lm.eventLogger != nil {
		lm.eventLogger.LogLockExpired(ticket.TicketID, ticket.ToolID, ticket.ThreadID, reason, holdDuration.Milliseconds())
	}

	// Try to grant next
	lm.tryGrantNext()

	return ticket
}

// ExpireWaitingTickets expires tickets that have exceeded TTL
func (lm *LockManager) ExpireWaitingTickets() []*model.Ticket {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	ttl := time.Duration(lm.config.TicketTTL) * time.Second
	expired := make([]*model.Ticket, 0)

	// Create new queue without expired tickets
	newQueue := make([]*model.Ticket, 0, len(lm.queue))

	for _, ticket := range lm.queue {
		if ticket.IsTTLExpired(ttl) {
			ticket.Expire()
			lm.cleanupTicket(ticket)
			expired = append(expired, ticket)

			log.Warn().
				Str("ticket_id", ticket.TicketID).
				Str("tool_id", ticket.ToolID).
				Dur("ttl", ttl).
				Msg("Ticket expired due to TTL")

			// Log event
			if lm.eventLogger != nil {
				lm.eventLogger.LogTicketExpired(ticket.TicketID, ticket.ToolID, ticket.ThreadID, "ttl_expired")
			}
		} else {
			newQueue = append(newQueue, ticket)
		}
	}

	lm.queue = newQueue

	return expired
}

// CheckGracePeriod checks if current lock holder has polled within grace period
func (lm *LockManager) CheckGracePeriod() *model.Ticket {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.currentLock == nil {
		return nil
	}

	gracePeriod := time.Duration(lm.config.LockGracePeriod) * time.Second

	if lm.currentLock.IsGracePeriodExpired(gracePeriod) {
		ticket := lm.currentLock
		holdDuration := ticket.HoldDuration()
		ticket.Expire()
		lm.currentLock = nil
		lm.cleanupTicket(ticket)

		log.Warn().
			Str("ticket_id", ticket.TicketID).
			Str("tool_id", ticket.ToolID).
			Dur("grace_period", gracePeriod).
			Msg("Lock expired due to grace period")

		// Log event
		if lm.eventLogger != nil {
			lm.eventLogger.LogLockExpired(ticket.TicketID, ticket.ToolID, ticket.ThreadID, "grace_period_expired", holdDuration.Milliseconds())
		}

		lm.tryGrantNext()

		return ticket
	}

	return nil
}

// CheckLockExpiry checks if current lock has exceeded max duration
func (lm *LockManager) CheckLockExpiry() *model.Ticket {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.currentLock == nil {
		return nil
	}

	if lm.currentLock.IsLockExpired() {
		ticket := lm.currentLock
		holdDuration := ticket.HoldDuration()
		ticket.Expire()
		lm.currentLock = nil
		lm.cleanupTicket(ticket)

		log.Warn().
			Str("ticket_id", ticket.TicketID).
			Str("tool_id", ticket.ToolID).
			Msg("Lock expired due to max duration")

		// Log event
		if lm.eventLogger != nil {
			lm.eventLogger.LogLockExpired(ticket.TicketID, ticket.ToolID, ticket.ThreadID, "max_duration_expired", holdDuration.Milliseconds())
		}

		lm.tryGrantNext()

		return ticket
	}

	return nil
}

// RemoveToolTickets removes all tickets for a specific tool
func (lm *LockManager) RemoveToolTickets(toolID string) []string {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	removed := make([]string, 0)

	// Check current lock
	if lm.currentLock != nil && lm.currentLock.ToolID == toolID {
		removed = append(removed, lm.currentLock.TicketID)
		lm.cleanupTicket(lm.currentLock)
		lm.currentLock = nil
	}

	// Remove from queue
	newQueue := make([]*model.Ticket, 0, len(lm.queue))
	for _, ticket := range lm.queue {
		if ticket.ToolID == toolID {
			removed = append(removed, ticket.TicketID)
			lm.cleanupTicket(ticket)
		} else {
			newQueue = append(newQueue, ticket)
		}
	}
	lm.queue = newQueue

	if len(removed) > 0 {
		log.Info().
			Str("tool_id", toolID).
			Strs("tickets", removed).
			Msg("Removed tickets for offline tool")

		lm.tryGrantNext()
	}

	return removed
}

// GetCurrentLock returns the current lock holder
func (lm *LockManager) GetCurrentLock() *model.Ticket {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	return lm.currentLock
}

// GetCurrentLockHolder returns the tool_id of current lock holder
func (lm *LockManager) GetCurrentLockHolder() string {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.currentLock == nil {
		return ""
	}
	return lm.currentLock.ToolID
}

// QueueLength returns the number of waiting tickets
func (lm *LockManager) QueueLength() int {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	return len(lm.queue)
}

// GetQueueStatus returns status info for the queue
func (lm *LockManager) GetQueueStatus() map[string]interface{} {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	result := map[string]interface{}{
		"queue_length": len(lm.queue),
	}

	if lm.currentLock != nil {
		result["current_lock"] = map[string]interface{}{
			"ticket_id":     lm.currentLock.TicketID,
			"tool_id":       lm.currentLock.ToolID,
			"thread_id":     lm.currentLock.ThreadID,
			"granted_at":    lm.currentLock.GrantedAt,
			"expires_in_ms": lm.currentLock.RemainingTime().Milliseconds(),
		}
	}

	queueInfo := make([]map[string]interface{}, 0, len(lm.queue))
	for i, ticket := range lm.queue {
		queueInfo = append(queueInfo, map[string]interface{}{
			"position":   i + 1,
			"tool_id":    ticket.ToolID,
			"thread_id":  ticket.ThreadID,
			"waiting_ms": ticket.WaitDuration().Milliseconds(),
		})
	}
	result["queue"] = queueInfo

	return result
}

// EstimateWaitTime estimates wait time based on position
func (lm *LockManager) EstimateWaitTime(position int) time.Duration {
	if position <= 0 {
		return 0
	}
	avgLockTime := time.Duration(lm.config.LockMaxDuration/2) * time.Second
	return time.Duration(position) * avgLockTime
}

// Internal helper methods

func (lm *LockManager) tryGrantNext() {
	// Already has a lock
	if lm.currentLock != nil {
		return
	}

	// Queue is empty
	if len(lm.queue) == 0 {
		return
	}

	// Get first ticket from queue
	ticket := lm.queue[0]
	lm.queue = lm.queue[1:]

	// Grant the lock
	lockDuration := time.Duration(lm.config.LockMaxDuration) * time.Second
	waitDuration := ticket.WaitDuration()
	ticket.Grant(lockDuration)
	lm.currentLock = ticket

	log.Info().
		Str("ticket_id", ticket.TicketID).
		Str("tool_id", ticket.ToolID).
		Str("thread_id", ticket.ThreadID).
		Dur("wait_duration", waitDuration).
		Time("expires_at", ticket.ExpiresAt).
		Msg("Lock granted")

	// Log event
	if lm.eventLogger != nil {
		lm.eventLogger.LogLockGranted(ticket.TicketID, ticket.ToolID, ticket.ThreadID, waitDuration.Milliseconds())
	}
}

func (lm *LockManager) getQueuePosition(ticketID string) int {
	// If it's the current lock, position is 0 (has lock)
	if lm.currentLock != nil && lm.currentLock.TicketID == ticketID {
		return 0
	}

	// Find in queue
	for i, ticket := range lm.queue {
		if ticket.TicketID == ticketID {
			return i + 1
		}
	}

	return -1 // Not found
}

func (lm *LockManager) cleanupTicket(ticket *model.Ticket) {
	delete(lm.tickets, ticket.TicketID)
	delete(lm.threadKeys, ticket.Key())
}
