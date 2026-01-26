package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CLIConfig holds the configuration parsed from command line arguments.
type CLIConfig struct {
	Command       string
	AppID         int
	AppHash       string
	SessionPath   string
	GroupName     string
	TopicName     string
	DirPath       string
	SubDir        string
	Workers       int
	UploadThreads int
	SkipMD5       bool
}

// ParseCLI parses command line arguments and environment variables.
func ParseCLI(appIDDef string, appHashDef string) (*CLIConfig, error) {
	if len(os.Args) < 2 {
		return nil, fmt.Errorf("usage: tgblobsync <command> [flags] [args]\nRun 'tgblobsync <command> --help' for more information")
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)

	cfg := &CLIConfig{Command: cmd}

	fs.IntVar(&cfg.Workers, "workers", 1, "Number of concurrent files")
	fs.IntVar(&cfg.UploadThreads, "upload-threads", 8, "Number of parallel threads for a single file upload")
	fs.BoolVar(&cfg.SkipMD5, "skip-md5", false, "Skip MD5 calculation and use modification time instead")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", cmd)
		switch cmd {
		case "push":
			fmt.Fprintf(os.Stderr, "  tgblobsync push [flags] <local-path> [<group>:<topic>[:subdir]]\n")
		case "pull":
			fmt.Fprintf(os.Stderr, "  tgblobsync pull [flags] [<group>:<topic>[:subdir]] <local-path>\n")
		case "browse":
			fmt.Fprintf(os.Stderr, "  tgblobsync browse [flags] [<group>:<topic>]\n")
		}
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return nil, err
	}

	remaining := fs.Args()

	switch cmd {
	case "push":
		if len(remaining) > 0 {
			cfg.DirPath = remaining[0]
		}
		if len(remaining) > 1 {
			parseTarget(remaining[1], cfg)
		}
	case "pull":
		if len(remaining) > 0 {
			if len(remaining) == 1 {
				cfg.DirPath = remaining[0]
			} else {
				parseTarget(remaining[0], cfg)
				cfg.DirPath = remaining[1]
			}
		}
	case "browse":
		if len(remaining) > 0 {
			parseTarget(remaining[0], cfg)
		}
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
	if cmd == "sessions" {
		return cfg, nil
	}

	if (cmd == "push" || cmd == "pull") && cfg.DirPath == "" {
		return nil, fmt.Errorf("local path is required for push/pull commands")
	}

	return cfg, nil
}

func parseTarget(target string, cfg *CLIConfig) {
	parts := strings.Split(target, ":")
	if len(parts) > 0 && parts[0] != "" {
		cfg.GroupName = parts[0]
	}
	if len(parts) > 1 && parts[1] != "" {
		cfg.TopicName = parts[1]
	}
	if len(parts) > 2 && parts[2] != "" {
		cfg.SubDir = parts[2]
	}
}
