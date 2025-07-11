package main_test

import (
	"bytes"
	"dash2hlsd/internal/channels"
	"dash2hlsd/internal/key"
	"encoding/hex"
	"testing"
)

// TestKeyService_NewServiceAndGetKey verifies the creation of the service and key retrieval.
func TestKeyService_NewServiceAndGetKey(t *testing.T) {
	key1, _ := hex.DecodeString("15f515458cdb5107452f943a111cbe89")
	key2, _ := hex.DecodeString("d3693103f232f28b4781bbc7e499c43a")

	mockConfig := &channels.ChannelConfig{
		Channels: []channels.Channel{
			{
				Id:  "channel1",
				Key: key1,
			},
			{
				Id:  "channel2",
				Key: key2,
			},
			{
				Id:  "channel3_no_key",
				Key: nil, // No key for this channel
			},
		},
	}

	service, err := key.NewService(mockConfig)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Test case 1: Get key for channel1
	retrievedKey1, found1 := service.GetKeyForChannel("channel1")
	if !found1 {
		t.Errorf("Expected to find key for channel1, but did not")
	}
	if !bytes.Equal(retrievedKey1, key1) {
		t.Errorf("Retrieved key for channel1 is incorrect. Got %x, want %x", retrievedKey1, key1)
	}

	// Test case 2: Get key for channel2
	retrievedKey2, found2 := service.GetKeyForChannel("channel2")
	if !found2 {
		t.Errorf("Expected to find key for channel2, but did not")
	}
	if !bytes.Equal(retrievedKey2, key2) {
		t.Errorf("Retrieved key for channel2 is incorrect. Got %x, want %x", retrievedKey2, key2)
	}

	// Test case 3: Get key for a channel with no key
	_, found3 := service.GetKeyForChannel("channel3_no_key")
	if found3 {
		// The service stores the key, even if it's nil/empty. The check should be for presence in the map.
		// The current implementation will find it and return a nil slice. This is expected.
		// Let's verify the returned key is indeed nil or empty.
		retrievedKey3, _ := service.GetKeyForChannel("channel3_no_key")
		if len(retrievedKey3) != 0 {
			t.Errorf("Expected key for channel3_no_key to be empty, but it was not")
		}
	} else {
		t.Errorf("Expected to find channel3_no_key in the map, but it was not found")
	}

	// Test case 4: Get key for a non-existent channel
	_, found4 := service.GetKeyForChannel("non_existent_channel")
	if found4 {
		t.Errorf("Expected not to find key for non_existent_channel, but did")
	}
}

// TestKeyService_NewServiceWithDuplicateID verifies that the service fails to initialize with duplicate channel IDs.
func TestKeyService_NewServiceWithDuplicateID(t *testing.T) {
	key1, _ := hex.DecodeString("15f515458cdb5107452f943a111cbe89")

	mockConfig := &channels.ChannelConfig{
		Channels: []channels.Channel{
			{
				Id:  "duplicate_id",
				Key: key1,
			},
			{
				Id:  "another_channel",
				Key: []byte("somekey"),
			},
			{
				Id:  "duplicate_id", // Duplicate ID
				Key: []byte("anotherkey"),
			},
		},
	}

	_, err := key.NewService(mockConfig)
	if err == nil {
		t.Fatal("Expected NewService to return an error for duplicate channel IDs, but it did not")
	}

	expectedError := "duplicate channel ID found in config: duplicate_id"
	if err.Error() != expectedError {
		t.Errorf("Expected error message '%s', got '%s'", expectedError, err.Error())
	}
}
