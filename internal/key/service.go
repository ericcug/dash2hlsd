package key

import (
	"dash2hlsd/internal/config"
	"fmt"
)

// Service provides decryption keys based on channel configuration.
// It is initialized once at startup and is safe for concurrent reads.
type Service struct {
	channelKeyMap map[string][]byte
}

// NewService creates and initializes a new key service from the given configuration.
// It extracts all channel keys and stores them in an internal map for fast lookups.
func NewService(cfg *config.ChannelConfig) (*Service, error) {
	keyMap := make(map[string][]byte)
	for _, channel := range cfg.Channels {
		if len(channel.Key) > 0 {
			// The key has already been decoded in the config loader.
			// We map it directly by the channel ID.
			if _, exists := keyMap[channel.Id]; exists {
				return nil, fmt.Errorf("duplicate channel ID found in config: %s", channel.Id)
			}
			keyMap[channel.Id] = channel.Key
		}
	}

	return &Service{
		channelKeyMap: keyMap,
	}, nil
}

// GetKeyForChannel retrieves a key for a given channel ID.
// It returns the key and a boolean indicating if the key was found.
func (s *Service) GetKeyForChannel(channelId string) ([]byte, bool) {
	// No lock needed as the map is read-only after initialization.
	key, found := s.channelKeyMap[channelId]
	return key, found
}
