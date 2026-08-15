// Package config loads and persists CLI profiles.
//
// Configuration lives at ~/.config/tropmail/config.toml (or $TROPMAIL_CONFIG_DIR).
// API keys are never written there: they go to the OS keyring, falling back to a
// 0600 credentials file when no keyring is available.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// DefaultProfile is used when no profile is named.
const DefaultProfile = "default"

// DefaultBaseURL is the production API endpoint.
const DefaultBaseURL = "https://api.tropmail.com/api/v1"

// ErrNoProfile is returned when the requested profile does not exist.
var ErrNoProfile = errors.New("profile not found")

// Profile is one named set of connection settings.
type Profile struct {
	BaseURL string `toml:"base_url,omitempty"`
	// Email is cached from the last successful login, for display only.
	Email string `toml:"email,omitempty"`
	Tier  string `toml:"tier,omitempty"`
}

// Config is the on-disk configuration file.
type Config struct {
	DefaultProfile string             `toml:"default_profile,omitempty"`
	Profiles       map[string]Profile `toml:"profiles,omitempty"`
}

// Dir returns the configuration directory, honouring TROPMAIL_CONFIG_DIR.
func Dir() (string, error) {
	if custom := os.Getenv("TROPMAIL_CONFIG_DIR"); custom != "" {
		return custom, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config directory: %w", err)
	}
	return filepath.Join(base, "tropmail"), nil
}

// Path returns the configuration file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the configuration file. A missing file yields an empty config so
// the CLI starts instantly on a fresh machine.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	cfg := &Config{Profiles: map[string]Profile{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return cfg, nil
}

// Save writes the configuration file with 0600 permissions.
func (c *Config) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	path := filepath.Join(dir, "config.toml")
	temp, err := os.CreateTemp(dir, "config-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	defer os.Remove(temp.Name())

	if err := toml.NewEncoder(temp).Encode(c); err != nil {
		temp.Close()
		return fmt.Errorf("encode config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Chmod(temp.Name(), 0o600); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// ResolveName returns the profile name to use, preferring an explicit choice,
// then TROPMAIL_PROFILE, then the configured default.
func (c *Config) ResolveName(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv("TROPMAIL_PROFILE"); env != "" {
		return env
	}
	if c.DefaultProfile != "" {
		return c.DefaultProfile
	}
	return DefaultProfile
}

// Get returns a profile by name, or an empty profile when it does not exist yet.
func (c *Config) Get(name string) Profile {
	return c.Profiles[name]
}

// Set stores a profile and makes it the default when none is set.
func (c *Config) Set(name string, profile Profile) {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = profile
	if c.DefaultProfile == "" {
		c.DefaultProfile = name
	}
}

// Remove deletes a profile.
func (c *Config) Remove(name string) error {
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNoProfile, name)
	}
	delete(c.Profiles, name)
	if c.DefaultProfile == name {
		c.DefaultProfile = ""
		for _, remaining := range c.Names() {
			c.DefaultProfile = remaining
			break
		}
	}
	return nil
}

// Names returns the configured profile names in sorted order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Endpoint returns the profile's base URL, falling back to the default.
func (p Profile) Endpoint() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return DefaultBaseURL
}
