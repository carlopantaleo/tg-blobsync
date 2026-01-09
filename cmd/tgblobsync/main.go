package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"tg-blobsync/internal/adapter/filesystem"
	"tg-blobsync/internal/adapter/telegram"
	"tg-blobsync/internal/adapter/ui"
	"tg-blobsync/internal/config"
	"tg-blobsync/internal/domain"
	"tg-blobsync/internal/usecase"
)

// These variables will be set by the linker during build
// -ldflags "-X main.AppID=12345 -X main.AppHash=abcdef..."
var (
	AppID   string
	AppHash string
)

type Config struct {
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

func main() {
	config, err := parseConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Initialize Dependencies
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	console := ui.NewConsoleUI(config.NonInteractive)

	log.Printf("Session file: %s", config.SessionPath)

	tgClient, err := telegram.NewTelegramClient(config.AppID, config.AppHash, config.SessionPath, console)
	if err != nil {
		log.Fatalf("Failed to create telegram client: %v", err)
	}

	// Start Client (Connects & Auths)
	log.Println("Connecting to Telegram...")
	if err := tgClient.Start(ctx, console); err != nil {
		log.Fatalf("Failed to start telegram client: %v", err)
	}
	defer tgClient.Close()

	log.Println("Connected!")

	tgClient.SetUploadThreads(config.UploadThreads)

	// Interactive Selection if needed
	if config.GroupID == 0 {
		log.Println("Fetching groups...")
		groups, err := tgClient.ListGroups(ctx)
		if err != nil {
			log.Fatalf("Failed to list groups: %v", err)
		}

		selectedGroup, err := console.SelectGroup(groups)
		if err != nil {
			log.Fatalf("Group selection failed: %v", err)
		}
		config.GroupID = selectedGroup.ID
		log.Printf("Selected Group: %s (%d)", selectedGroup.Title, config.GroupID)
	} else {
		// Ensure we have the AccessHash for the provided ID
		log.Printf("Resolving group %d...", config.GroupID)
		if err := tgClient.ResolveGroup(ctx, config.GroupID); err != nil {
			log.Fatalf("Failed to resolve group: %v", err)
		}
	}

	if config.TopicID == 0 {
		log.Println("Fetching topics...")
		topics, err := tgClient.ListTopics(ctx, config.GroupID)
		if err != nil {
			log.Fatalf("Failed to list topics: %v", err)
		}

		selectedTopic, err := console.SelectTopic(topics)
		if err != nil {
			log.Fatalf("Topic selection failed: %v", err)
		}
		config.TopicID = selectedTopic.ID
		log.Printf("Selected Topic: %s (%d)", selectedTopic.Title, config.TopicID)
	}

	tgClient.SetProgressReporter(console)

	var opErr error
	// Execute Command
	switch config.Command {
	case "push":
		localFS := filesystem.NewLocalFileSystem()
		syncer := usecase.NewSynchronizer(localFS, tgClient, config.Workers, console, config.SkipMD5)
		syncer.SetSubDir(config.SubDir)
		opErr = syncer.Push(ctx, config.DirPath, config.GroupID, config.TopicID)
	case "pull":
		localFS := filesystem.NewLocalFileSystem()
		syncer := usecase.NewSynchronizer(localFS, tgClient, config.Workers, console, config.SkipMD5)
		syncer.SetSubDir(config.SubDir)
		opErr = syncer.Pull(ctx, config.DirPath, config.GroupID, config.TopicID)
	case "list":
		opErr = runList(ctx, tgClient, console, config)
	}

	if opErr != nil {
		log.Fatalf("Operation failed: %v", opErr)
	}

	log.Println("Done.")
}

func parseConfig() (*Config, error) {
	if len(os.Args) < 2 {
		return nil, fmt.Errorf("usage: tgblobsync <command> [flags]\nCommands: push, pull, list")
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)

	cfg := &Config{Command: cmd}

	fs.Int64Var(&cfg.GroupID, "group-id", 0, "ID of the Supergroup")
	fs.Int64Var(&cfg.TopicID, "topic-id", 0, "ID of the Topic")
	fs.StringVar(&cfg.DirPath, "dir", "", "Path to the directory to sync (required for push/pull)")
	fs.StringVar(&cfg.SubDir, "sub-dir", "", "Synchronize only a specific subdirectory within the topic")
	fs.IntVar(&cfg.Workers, "workers", 1, "Number of concurrent files")
	fs.IntVar(&cfg.UploadThreads, "upload-threads", 8, "Number of parallel threads for a single file upload")
	fs.BoolVar(&cfg.SkipMD5, "skip-md5", false, "Skip MD5 calculation and use modification time instead")
	fs.BoolVar(&cfg.NonInteractive, "non-interactive", false, "Disable interactive UI and progress bars")

	fs.Parse(os.Args[2:])

	// Validate App Credentials
	appIDStr := os.Getenv("APP_ID")
	if AppID != "" {
		appIDStr = AppID
	}
	appHashStr := os.Getenv("APP_HASH")
	if AppHash != "" {
		appHashStr = AppHash
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

	cfg.SessionPath, err = config.GetSessionPath()
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

func runList(ctx context.Context, storage domain.BlobStorage, ui *ui.ConsoleUI, cfg *Config) error {
	log.Println("Fetching remote files...")
	files, err := storage.ListFiles(ctx, cfg.GroupID, cfg.TopicID)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No files found in this topic.")
		return nil
	}

	return ui.BrowseFiles(files)
}
