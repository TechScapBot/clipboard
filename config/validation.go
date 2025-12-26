package config

import (
	"errors"
	"fmt"
)

// Validate checks if the config values are valid
func (c *Config) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Port must be valid
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	// poll_interval must be less than ticket_ttl (in ms vs seconds)
	if c.PollInterval >= c.TicketTTL*1000 {
		return fmt.Errorf("poll_interval (%dms) must be less than ticket_ttl (%ds = %dms)",
			c.PollInterval, c.TicketTTL, c.TicketTTL*1000)
	}

	// lock_grace_period must be less than lock_max_duration
	if c.LockGracePeriod >= c.LockMaxDuration {
		return fmt.Errorf("lock_grace_period (%ds) must be less than lock_max_duration (%ds)",
			c.LockGracePeriod, c.LockMaxDuration)
	}

	// heartbeat_interval must be less than heartbeat_timeout
	if c.HeartbeatInterval >= c.HeartbeatTimeout {
		return fmt.Errorf("heartbeat_interval (%ds) must be less than heartbeat_timeout (%ds)",
			c.HeartbeatInterval, c.HeartbeatTimeout)
	}

	// log_retention_days must be at least 1
	if c.LogRetentionDays < 1 {
		return errors.New("log_retention_days must be at least 1")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("log_level must be one of: debug, info, warn, error (got: %s)", c.LogLevel)
	}

	// Positive values check
	if c.HeartbeatTimeout <= 0 {
		return errors.New("heartbeat_timeout must be positive")
	}
	if c.HeartbeatInterval <= 0 {
		return errors.New("heartbeat_interval must be positive")
	}
	if c.PollInterval <= 0 {
		return errors.New("poll_interval must be positive")
	}
	if c.TicketTTL <= 0 {
		return errors.New("ticket_ttl must be positive")
	}
	if c.LockMaxDuration <= 0 {
		return errors.New("lock_max_duration must be positive")
	}
	if c.LockGracePeriod <= 0 {
		return errors.New("lock_grace_period must be positive")
	}
	if c.LockExtendMax < 0 {
		return errors.New("lock_extend_max must be non-negative")
	}

	return nil
}
