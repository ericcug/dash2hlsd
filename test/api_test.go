package main_test

import (
	"dash2hlsd/internal/api"
	"dash2hlsd/internal/channels"
	"dash2hlsd/internal/key"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPI_HandleKey tests the /key/{channelId} endpoint.
// It passes nil for the session manager dependency, as this specific handler does not use it,
// allowing for a focused unit test on the key service logic within the API layer.
func TestAPI_HandleKey(t *testing.T) {
	// 1. Setup
	keyHex := "15f515458cdb5107452f943a111cbe89"
	keyBytes, _ := hex.DecodeString(keyHex)

	mockConfig := &channels.ChannelConfig{
		Channels: []channels.Channel{
			{Id: "channel_with_key", Key: keyBytes},
		},
	}

	keyService, err := key.NewService(mockConfig)
	require.NoError(t, err, "Failed to create key service")

	// Create the API handler, passing nil for the unused dependency.
	handler := api.New(nil, keyService)
	server := httptest.NewServer(handler)
	defer server.Close()

	// 2. Test Case: Key Found
	t.Run("Key Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.URL+"/key/channel_with_key", nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"))

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, keyBytes, body)
	})

	// 3. Test Case: Key Not Found
	t.Run("Key Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.URL+"/key/unknown_channel", nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
