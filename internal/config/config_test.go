package config

import (
	"os"
	"testing"
)

func TestParseCLI(t *testing.T) {
	// Save original args and env
	origArgs := os.Args
	origAppID := os.Getenv("APP_ID")
	origAppHash := os.Getenv("APP_HASH")

	defer func() {
		os.Args = origArgs
		os.Setenv("APP_ID", origAppID)
		os.Setenv("APP_HASH", origAppHash)
	}()

	tests := []struct {
		name        string
		args        []string
		envAppID    string
		envAppHash  string
		defAppID    string
		defAppHash  string
		expectedCmd string
		wantErr     bool
		validate    func(*testing.T, *CLIConfig)
	}{
		{
			name:        "Push with only local path",
			args:        []string{"tgblobsync", "push", "/tmp/data"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "push",
			wantErr:     false,
			validate: func(t *testing.T, cfg *CLIConfig) {
				if cfg.DirPath != "/tmp/data" {
					t.Errorf("expected DirPath /tmp/data, got %s", cfg.DirPath)
				}
			},
		},
		{
			name:        "Push with skip-md5",
			args:        []string{"tgblobsync", "push", "--skip-md5", "/tmp/data"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "push",
			wantErr:     false,
			validate: func(t *testing.T, cfg *CLIConfig) {
				if !cfg.SkipMD5 {
					t.Error("expected SkipMD5 true")
				}
			},
		},
		{
			name:        "Pull with only local path",
			args:        []string{"tgblobsync", "pull", "/tmp/data"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "pull",
			wantErr:     false,
			validate: func(t *testing.T, cfg *CLIConfig) {
				if cfg.DirPath != "/tmp/data" {
					t.Errorf("expected DirPath /tmp/data, got %s", cfg.DirPath)
				}
			},
		},
		{
			name:        "Pull with skip-md5",
			args:        []string{"tgblobsync", "pull", "--skip-md5", "/tmp/data"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "pull",
			wantErr:     false,
			validate: func(t *testing.T, cfg *CLIConfig) {
				if !cfg.SkipMD5 {
					t.Error("expected SkipMD5 true")
				}
			},
		},
		{
			name:        "Pull with target and local path",
			args:        []string{"tgblobsync", "pull", "MyGroup:MyTopic:sub/dir", "/tmp/data"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "pull",
			wantErr:     false,
			validate: func(t *testing.T, cfg *CLIConfig) {
				if cfg.GroupName != "MyGroup" || cfg.TopicName != "MyTopic" || cfg.SubDir != "sub/dir" {
					t.Errorf("invalid target parsing: %s:%s:%s", cfg.GroupName, cfg.TopicName, cfg.SubDir)
				}
				if cfg.DirPath != "/tmp/data" {
					t.Errorf("expected DirPath /tmp/data, got %s", cfg.DirPath)
				}
			},
		},
		{
			name:        "Valid Push Command",
			args:        []string{"tgblobsync", "push", "/tmp/data", "MyGroup:MyTopic"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "push",
			wantErr:     false,
		},
		{
			name:        "Valid Pull Command",
			args:        []string{"tgblobsync", "pull", "MyGroup:MyTopic", "/tmp/data"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "pull",
			wantErr:     false,
		},
		{
			name:        "Push with spaces in target",
			args:        []string{"tgblobsync", "push", "/tmp/data", "My Group:My Topic:remote/path"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "push",
			wantErr:     false,
			validate: func(t *testing.T, cfg *CLIConfig) {
				if cfg.GroupName != "My Group" || cfg.TopicName != "My Topic" || cfg.SubDir != "remote/path" {
					t.Errorf("invalid target parsing with spaces: %s:%s:%s", cfg.GroupName, cfg.TopicName, cfg.SubDir)
				}
			},
		},
		{
			name:        "Missing Dir for Push",
			args:        []string{"tgblobsync", "push"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "push",
			wantErr:     true,
		},
		{
			name:        "Missing App Credentials",
			args:        []string{"tgblobsync", "list"},
			envAppID:    "",
			envAppHash:  "",
			expectedCmd: "list",
			wantErr:     true,
		},
		{
			name:        "Credentials from defaults",
			args:        []string{"tgblobsync", "list"},
			envAppID:    "",
			envAppHash:  "",
			defAppID:    "99999",
			defAppHash:  "xyz",
			expectedCmd: "list",
			wantErr:     false,
		},
		{
			name:        "Non-interactive missing IDs",
			args:        []string{"tgblobsync", "push", "--non-interactive", "/tmp"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "push",
			wantErr:     true,
		},
		{
			name:        "No args",
			args:        []string{"tgblobsync"},
			envAppID:    "12345",
			envAppHash:  "abcdef",
			expectedCmd: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			os.Setenv("APP_ID", tt.envAppID)
			os.Setenv("APP_HASH", tt.envAppHash)

			cfg, err := ParseCLI(tt.defAppID, tt.defAppHash)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCLI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && cfg != nil {
				if cfg.Command != tt.expectedCmd {
					t.Errorf("ParseCLI() command = %v, want %v", cfg.Command, tt.expectedCmd)
				}
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
				// Verify other fields if needed, e.g. AppID
				expectedID := tt.envAppID
				if expectedID == "" {
					expectedID = tt.defAppID
				}
				// Skip verifying ID if it failed parsing (already handled by wantErr)
			}
		})
	}
}

func TestGetSessionPath(t *testing.T) {
	path, err := GetSessionPath()
	if err != nil {
		t.Fatalf("GetSessionPath() failed: %v", err)
	}
	if path == "" {
		t.Error("GetSessionPath() returned empty string")
	}
}
