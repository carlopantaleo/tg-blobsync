package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

// CLIConfig holds the configuration parsed from command line arguments.
type CLIConfig struct {
	Command        string
	AppID          int
	AppHash        string
	SessionPath    string
	GroupID        int64
	TopicID        int64
	DirPath        string
	SubDir         string
	Workers        int
	UploadThreads  int
	SkipMD5        bool
	NonInteractive bool
}

// ParseCLI parses command line arguments and environment variables.
func ParseCLI(appIDDef string, appHashDef string) (*CLIConfig, error) {
	if len(os.Args) < 2 {
		return nil, fmt.Errorf("usage: tgblobsync <command> [flags]\nCommands: push, pull, list")
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)

	cfg := &CLIConfig{Command: cmd}

	fs.Int64Var(&cfg.GroupID, "group-id", 0, "ID of the Supergroup")
	fs.Int64Var(&cfg.TopicID, "topic-id", 0, "ID of the Topic")
	fs.StringVar(&cfg.DirPath, "dir", "", "Path to the directory to sync (required for push/pull)")
	fs.StringVar(&cfg.SubDir, "sub-dir", "", "Synchronize only a specific subdirectory within the topic")
	fs.IntVar(&cfg.Workers, "workers", 1, "Number of concurrent files")
	fs.IntVar(&cfg.UploadThreads, "upload-threads", 8, "Number of parallel threads for a single file upload")
	fs.BoolVar(&cfg.SkipMD5, "skip-md5", false, "Skip MD5 calculation and use modification time instead")
	fs.BoolVar(&cfg.NonInteractive, "non-interactive", false, "Disable interactive UI and progress bars")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return nil, err
	}

	// Validate App Credentials
	appIDStr := os.Getenv("APP_ID")
	if appIDDef != "" {
		appIDStr = appIDDef
	}
	appHashStr := os.Getenv("APP_HASH")
	if appHashDef != "" {
		appHashStr = appHashDef
	}

	if appIDStr == "" || appHashStr == "" {
		return nil, fmt.Errorf("AppID and AppHash must be provided via ldflags or env vars (APP_ID/APP_HASH)")
	}

	var err error
	cfg.AppID, err = strconv.Atoi(appIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid AppID: %v", err)
	}
	cfg.AppHash = appHashStr

	cfg.SessionPath, err = GetSessionPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get session path: %v", err)
	}

	// Command specific validation
	if (cmd == "push" || cmd == "pull") && cfg.DirPath == "" {
		return nil, fmt.Errorf("--dir is required for push/pull commands")
	}

	if cfg.NonInteractive {
		if cfg.GroupID == 0 || cfg.TopicID == 0 {
			return nil, fmt.Errorf("--group-id and --topic-id are required in non-interactive mode")
		}
	}

	return cfg, nil
}
