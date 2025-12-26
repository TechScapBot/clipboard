package model

import (
	"time"

	"github.com/google/uuid"
)

// TicketStatus represents the status of a lock ticket
type TicketStatus string

const (
	TicketStatusWaiting  TicketStatus = "waiting"
	TicketStatusGranted  TicketStatus = "granted"
	TicketStatusExpired  TicketStatus = "expired"
	TicketStatusReleased TicketStatus = "released"
)

// Ticket represents a lock request in the queue
type Ticket struct {
	TicketID    string       `json:"ticket_id"`
	ToolID      string       `json:"tool_id"`
	ThreadID    string       `json:"thread_id"`
	RequestedAt time.Time    `json:"requested_at"`
	Status      TicketStatus `json:"status"`
	GrantedAt   time.Time    `json:"granted_at,omitempty"`
	ExpiresAt   time.Time    `json:"expires_at,omitempty"`
	LastPollAt  time.Time    `json:"last_poll_at"`
	ExtendCount int          `json:"extend_count"`
}

// NewTicket creates a new waiting ticket
func NewTicket(toolID, threadID string) *Ticket {
	now := time.Now()
	return &Ticket{
		TicketID:    uuid.New().String(),
		ToolID:      toolID,
		ThreadID:    threadID,
		RequestedAt: now,
		Status:      TicketStatusWaiting,
		LastPollAt:  now,
		ExtendCount: 0,
	}
}

// IsWaiting returns true if ticket is waiting in queue
func (t *Ticket) IsWaiting() bool {
	return t.Status == TicketStatusWaiting
}

// IsGranted returns true if ticket has been granted lock
func (t *Ticket) IsGranted() bool {
	return t.Status == TicketStatusGranted
}

// IsExpired returns true if ticket has expired
func (t *Ticket) IsExpired() bool {
	return t.Status == TicketStatusExpired
}

// IsReleased returns true if ticket has been released
func (t *Ticket) IsReleased() bool {
	return t.Status == TicketStatusReleased
}

// Grant grants the lock to this ticket
func (t *Ticket) Grant(lockDuration time.Duration) {
	now := time.Now()
	t.Status = TicketStatusGranted
	t.GrantedAt = now
	t.ExpiresAt = now.Add(lockDuration)
}

// Expire marks the ticket as expired
func (t *Ticket) Expire() {
	t.Status = TicketStatusExpired
}

// Release marks the ticket as released
func (t *Ticket) Release() {
	t.Status = TicketStatusReleased
}

// UpdatePollTime updates the last poll time
func (t *Ticket) UpdatePollTime() {
	t.LastPollAt = time.Now()
}

// IsLockExpired checks if the lock has expired (for granted tickets)
func (t *Ticket) IsLockExpired() bool {
	if !t.IsGranted() {
		return false
	}
	return time.Now().After(t.ExpiresAt)
}

// IsTTLExpired checks if the ticket TTL has expired (for waiting tickets)
func (t *Ticket) IsTTLExpired(ttl time.Duration) bool {
	if !t.IsWaiting() {
		return false
	}
	return time.Since(t.LastPollAt) > ttl
}

// IsGracePeriodExpired checks if grace period has passed without polling
func (t *Ticket) IsGracePeriodExpired(gracePeriod time.Duration) bool {
	if !t.IsGranted() {
		return false
	}
	// If no poll since granted, check grace period from granted time
	if t.LastPollAt.Before(t.GrantedAt) || t.LastPollAt.Equal(t.GrantedAt) {
		return time.Since(t.GrantedAt) > gracePeriod
	}
	return false
}

// Extend extends the lock duration
func (t *Ticket) Extend(extendDuration time.Duration) {
	t.ExpiresAt = time.Now().Add(extendDuration)
	t.ExtendCount++
}

// WaitDuration returns how long this ticket has been waiting
func (t *Ticket) WaitDuration() time.Duration {
	if t.IsGranted() {
		return t.GrantedAt.Sub(t.RequestedAt)
	}
	return time.Since(t.RequestedAt)
}

// HoldDuration returns how long this ticket has held the lock
func (t *Ticket) HoldDuration() time.Duration {
	if !t.IsGranted() && !t.IsReleased() {
		return 0
	}
	return time.Since(t.GrantedAt)
}

// RemainingTime returns remaining time before lock expires
func (t *Ticket) RemainingTime() time.Duration {
	if !t.IsGranted() {
		return 0
	}
	remaining := time.Until(t.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Key returns a unique key for this tool+thread combination
func (t *Ticket) Key() string {
	return t.ToolID + ":" + t.ThreadID
}

// ToJSON returns a map representation for JSON response
func (t *Ticket) ToJSON() map[string]interface{} {
	result := map[string]interface{}{
		"ticket_id":    t.TicketID,
		"tool_id":      t.ToolID,
		"thread_id":    t.ThreadID,
		"requested_at": t.RequestedAt,
		"status":       t.Status,
	}

	if t.IsGranted() {
		result["granted_at"] = t.GrantedAt
		result["expires_at"] = t.ExpiresAt
		result["extend_count"] = t.ExtendCount
	}

	return result
}
