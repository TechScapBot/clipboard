package config

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the clipboard controller
type Config struct {
	mu sync.RWMutex

	// Server
	Port int `yaml:"port" json:"port"`

	// Heartbeat
	HeartbeatTimeout  int `yaml:"heartbeat_timeout" json:"heartbeat_timeout"`
	HeartbeatInterval int `yaml:"heartbeat_interval" json:"heartbeat_interval"`

	// Polling
	PollInterval int `yaml:"poll_interval" json:"poll_interval"`

	// Ticket
	TicketTTL       int  `yaml:"ticket_ttl" json:"ticket_ttl"`
	TicketTTLOnPoll bool `yaml:"ticket_ttl_on_poll" json:"ticket_ttl_on_poll"`

	// Lock
	LockMaxDuration int  `yaml:"lock_max_duration" json:"lock_max_duration"`
	LockExtendable  bool `yaml:"lock_extendable" json:"lock_extendable"`
	LockExtendMax   int  `yaml:"lock_extend_max" json:"lock_extend_max"`
	LockGracePeriod int  `yaml:"lock_grace_period" json:"lock_grace_period"`

	// Priority (for future use)
	PriorityEnabled bool `yaml:"priority_enabled" json:"priority_enabled"`

	// Logging
	LogDir           string `yaml:"log_dir" json:"log_dir"`
	LogRetentionDays int    `yaml:"log_retention_days" json:"log_retention_days"`
	LogLevel         string `yaml:"log_level" json:"log_level"`
	LogRequests      bool   `yaml:"log_requests" json:"log_requests"`
	LogEvents        bool   `yaml:"log_events" json:"log_events"`
	LogMetrics       bool   `yaml:"log_metrics" json:"log_metrics"`
	LogSummary       bool   `yaml:"log_summary" json:"log_summary"`
	LogHeartbeats    bool   `yaml:"log_heartbeats" json:"log_heartbeats"` // Log heartbeat events (noisy, default false)

	// Client Retry (suggestions for client)
	ClientRetryMax     int `yaml:"client_retry_max" json:"client_retry_max"`
	ClientRetryDelayMs int `yaml:"client_retry_delay_ms" json:"client_retry_delay_ms"`
}

// Default returns a Config with default values
func Default() *Config {
	return &Config{
		Port:               8899,
		HeartbeatTimeout:   300,
		HeartbeatInterval:  120,
		PollInterval:       200,
		TicketTTL:          120,
		TicketTTLOnPoll:    true,
		LockMaxDuration:    20,
		LockExtendable:     true,
		LockExtendMax:      2,
		LockGracePeriod:    5,
		PriorityEnabled:    false,
		LogDir:             "./logs",
		LogRetentionDays:   30,
		LogLevel:           "info",
		ClientRetryMax:     3,
		ClientRetryDelayMs: 1000,
	}
}

// Load loads config from a YAML file, applying defaults for missing values
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, use defaults
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// GetClientConfig returns config values relevant to clients
func (c *Config) GetClientConfig() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"heartbeat_interval":    c.HeartbeatInterval,
		"heartbeat_timeout":     c.HeartbeatTimeout,
		"poll_interval":         c.PollInterval,
		"ticket_ttl":            c.TicketTTL,
		"lock_max_duration":     c.LockMaxDuration,
		"client_retry_max":      c.ClientRetryMax,
		"client_retry_delay_ms": c.ClientRetryDelayMs,
	}
}

// Update updates config values at runtime (only safe values)
func (c *Config) Update(updates map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := updates["poll_interval"].(int); ok {
		c.PollInterval = v
	}
	if v, ok := updates["ticket_ttl"].(int); ok {
		c.TicketTTL = v
	}
	if v, ok := updates["lock_max_duration"].(int); ok {
		c.LockMaxDuration = v
	}
	if v, ok := updates["lock_grace_period"].(int); ok {
		c.LockGracePeriod = v
	}
	if v, ok := updates["lock_extendable"].(bool); ok {
		c.LockExtendable = v
	}
	if v, ok := updates["lock_extend_max"].(int); ok {
		c.LockExtendMax = v
	}
}

// ToMap returns all config as a map
func (c *Config) ToMap() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"port":                  c.Port,
		"heartbeat_timeout":     c.HeartbeatTimeout,
		"heartbeat_interval":    c.HeartbeatInterval,
		"poll_interval":         c.PollInterval,
		"ticket_ttl":            c.TicketTTL,
		"ticket_ttl_on_poll":    c.TicketTTLOnPoll,
		"lock_max_duration":     c.LockMaxDuration,
		"lock_extendable":       c.LockExtendable,
		"lock_extend_max":       c.LockExtendMax,
		"lock_grace_period":     c.LockGracePeriod,
		"priority_enabled":      c.PriorityEnabled,
		"log_dir":               c.LogDir,
		"log_retention_days":    c.LogRetentionDays,
		"log_level":             c.LogLevel,
		"client_retry_max":      c.ClientRetryMax,
		"client_retry_delay_ms": c.ClientRetryDelayMs,
	}
}
