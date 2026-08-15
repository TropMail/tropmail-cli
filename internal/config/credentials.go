package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/zalando/go-keyring"
)

// keyringService namespaces our secrets in the OS keyring.
const keyringService = "tropmail"

// ErrNoCredentials is returned when a profile has no stored API key.
var ErrNoCredentials = errors.New("no API key stored for this profile")

// Storage names where a key was found or written.
type Storage string

const (
	StorageKeyring     Storage = "keyring"
	StorageFile        Storage = "file"
	StorageEnvironment Storage = "environment"
)

type credentialsFile struct {
	Keys map[string]string `toml:"keys"`
}

func credentialsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.toml"), nil
}

// StoreKey saves an API key for a profile, preferring the OS keyring and
// falling back to a 0600 file on headless machines with no secret service.
func StoreKey(profile, apiKey string) (Storage, error) {
	if err := keyring.Set(keyringService, profile, apiKey); err == nil {
		return StorageKeyring, nil
	}
	if err := storeKeyInFile(profile, apiKey); err != nil {
		return "", err
	}
	return StorageFile, nil
}

func storeKeyInFile(profile, apiKey string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	file := credentialsFile{Keys: map[string]string{}}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		_ = toml.Unmarshal(raw, &file)
		if file.Keys == nil {
			file.Keys = map[string]string{}
		}
	}
	file.Keys[profile] = apiKey

	temp, err := os.CreateTemp(filepath.Dir(path), "credentials-*.toml")
	if err != nil {
		return fmt.Errorf("create temp credentials: %w", err)
	}
	defer os.Remove(temp.Name())

	if err := toml.NewEncoder(temp).Encode(file); err != nil {
		temp.Close()
		return fmt.Errorf("encode credentials: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp credentials: %w", err)
	}
	if err := os.Chmod(temp.Name(), 0o600); err != nil {
		return fmt.Errorf("chmod credentials: %w", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// LoadKey returns the API key for a profile.
//
// TROPMAIL_API_KEY wins over stored credentials so scripts and CI can override
// whatever a developer has logged in with.
func LoadKey(profile string) (string, Storage, error) {
	if env := os.Getenv("TROPMAIL_API_KEY"); env != "" {
		return env, StorageEnvironment, nil
	}
	if key, err := keyring.Get(keyringService, profile); err == nil && key != "" {
		return key, StorageKeyring, nil
	}

	path, err := credentialsPath()
	if err != nil {
		return "", "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", ErrNoCredentials
	}
	var file credentialsFile
	if err := toml.Unmarshal(raw, &file); err != nil {
		return "", "", fmt.Errorf("parse %s: %w", path, err)
	}
	if key := file.Keys[profile]; key != "" {
		return key, StorageFile, nil
	}
	return "", "", ErrNoCredentials
}

// DeleteKey removes a stored API key from both the keyring and the file.
func DeleteKey(profile string) error {
	keyringErr := keyring.Delete(keyringService, profile)

	path, err := credentialsPath()
	if err != nil {
		return err
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) && keyringErr != nil {
			return ErrNoCredentials
		}
		return nil
	}

	var file credentialsFile
	if err := toml.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if _, ok := file.Keys[profile]; !ok {
		if keyringErr != nil {
			return ErrNoCredentials
		}
		return nil
	}
	delete(file.Keys, profile)

	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("rewrite credentials: %w", err)
	}
	defer handle.Close()
	if err := toml.NewEncoder(handle).Encode(file); err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	return nil
}
