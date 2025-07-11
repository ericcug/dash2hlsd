package main_test

import (
	"bytes"
	"dash2hlsd/internal/channels"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

const testChannelsJSON = `{
	"Name": "mytv",
	"Id": "mytv",
	"UserAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5.2 Safari/605.1.15",
	"Channels": [
		{
			"Name": "SUPER FREE (免費)",
			"Id": "superfree",
			"Manifest": "https://mytvsuper.ewc.workers.dev/mytvsuper/CWIN",
			"Keys": [
				"0737b75ee8906c00bb7bb8f666da72a0:15f515458cdb5107452f943a111cbe89"
			]
		},
		{
			"Name": "myTV SUPER直播足球6台",
			"Id": "EVT6",
			"Manifest": "https://mytvsuper.ewc.workers.dev/mytvsuper/EVT6",
			"Keys": [
				"e069fc056280e4caa7d0ffb99024c05a:d3693103f232f28b4781bbc7e499c43a"
			]
		}
	]
}`

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for the test file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "channels.json")

	// Write the test JSON to the temporary file
	err := os.WriteFile(configPath, []byte(testChannelsJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write temporary config file: %v", err)
	}

	// Call the function we are testing
	config, err := channels.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Assert top-level properties
	if config.Name != "mytv" {
		t.Errorf("Expected Name to be 'mytv', got '%s'", config.Name)
	}
	if config.Id != "mytv" {
		t.Errorf("Expected Id to be 'mytv', got '%s'", config.Id)
	}
	expectedUserAgent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5.2 Safari/605.1.15"
	if config.UserAgent != expectedUserAgent {
		t.Errorf("Expected UserAgent to be '%s', got '%s'", expectedUserAgent, config.UserAgent)
	}

	// Assert number of channels
	if len(config.Channels) != 2 {
		t.Fatalf("Expected 2 channels, got %d", len(config.Channels))
	}

	// Assert properties of the first channel
	ch1 := config.Channels[0]
	if ch1.Name != "SUPER FREE (免費)" {
		t.Errorf("Expected Channel 1 Name to be 'SUPER FREE (免費)', got '%s'", ch1.Name)
	}
	if ch1.Id != "superfree" {
		t.Errorf("Expected Channel 1 Id to be 'superfree', got '%s'", ch1.Id)
	}
	if ch1.ManifestURL != "https://mytvsuper.ewc.workers.dev/mytvsuper/CWIN" {
		t.Errorf("Expected Channel 1 ManifestURL to be 'https://mytvsuper.ewc.workers.dev/mytvsuper/CWIN', got '%s'", ch1.ManifestURL)
	}
	expectedKey1, _ := hex.DecodeString("15f515458cdb5107452f943a111cbe89")
	if !bytes.Equal(ch1.Key, expectedKey1) {
		t.Errorf("Expected Channel 1 Key to be '%x', got '%x'", expectedKey1, ch1.Key)
	}

	// Assert properties of the second channel
	ch2 := config.Channels[1]
	if ch2.Name != "myTV SUPER直播足球6台" {
		t.Errorf("Expected Channel 2 Name to be 'myTV SUPER直播足球6台', got '%s'", ch2.Name)
	}
	if ch2.Id != "EVT6" {
		t.Errorf("Expected Channel 2 Id to be 'EVT6', got '%s'", ch2.Id)
	}
	if ch2.ManifestURL != "https://mytvsuper.ewc.workers.dev/mytvsuper/EVT6" {
		t.Errorf("Expected Channel 2 ManifestURL to be 'https://mytvsuper.ewc.workers.dev/mytvsuper/EVT6', got '%s'", ch2.ManifestURL)
	}
	expectedKey2, _ := hex.DecodeString("d3693103f232f28b4781bbc7e499c43a")
	if !bytes.Equal(ch2.Key, expectedKey2) {
		t.Errorf("Expected Channel 2 Key to be '%x', got '%x'", expectedKey2, ch2.Key)
	}
}
