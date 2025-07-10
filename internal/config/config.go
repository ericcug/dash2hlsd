package config

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Channel defines the final, processed structure for a single channel.
type Channel struct {
	Name        string
	Id          string
	ManifestURL string
	// Key is the processed decryption key, decoded from a hex string.
	Key []byte
}

// ChannelConfig holds the fully processed application configuration.
type ChannelConfig struct {
	Name      string
	Id        string
	UserAgent string
	Channels  []Channel
}

// rawChannel is used for intermediate unmarshaling from the JSON file,
// to handle the specific format of the "Keys" field.
type rawChannel struct {
	Name        string   `json:"Name"`
	Id          string   `json:"Id"`
	ManifestURL string   `json:"Manifest"`
	Keys        []string `json:"Keys"` // Raw 'kid:key' string from JSON
}

// rawConfig is the intermediate structure that maps directly to the JSON file.
type rawConfig struct {
	Name      string       `json:"Name"`
	Id        string       `json:"Id"`
	UserAgent string       `json:"UserAgent"`
	Channels  []rawChannel `json:"Channels"`
}

// LoadConfig reads and parses the configuration file from the given path.
// It performs the crucial step of processing the raw key strings into byte slices.
func LoadConfig(path string) (*ChannelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", path, err)
	}

	var rawCfg rawConfig
	if err := json.Unmarshal(data, &rawCfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config JSON: %w", err)
	}

	// Process the raw channels into the final, clean Channel structs.
	processedChannels := make([]Channel, 0, len(rawCfg.Channels))
	for _, rc := range rawCfg.Channels {
		var keyBytes []byte
		// As per the spec, a channel may not be encrypted.
		if len(rc.Keys) > 0 && rc.Keys[0] != "" {
			// Take the first key, split by ':', and decode the second part (the key).
			keyParts := strings.Split(rc.Keys[0], ":")
			if len(keyParts) != 2 {
				return nil, fmt.Errorf("invalid key format for channel '%s': expected 'kid:key', got '%s'", rc.Id, rc.Keys[0])
			}

			keyHex := keyParts[1]
			keyBytes, err = hex.DecodeString(keyHex)
			if err != nil {
				return nil, fmt.Errorf("failed to decode hex key for channel '%s': %w", rc.Id, err)
			}
		}

		processedChannels = append(processedChannels, Channel{
			Name:        rc.Name,
			Id:          rc.Id,
			ManifestURL: rc.ManifestURL,
			Key:         keyBytes,
		})
	}

	// Assemble the final, clean configuration object.
	finalConfig := &ChannelConfig{
		Name:      rawCfg.Name,
		Id:        rawCfg.Id,
		UserAgent: rawCfg.UserAgent,
		Channels:  processedChannels,
	}

	return finalConfig, nil
}
