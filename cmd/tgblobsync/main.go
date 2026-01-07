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
	"tg-blobsync/internal/usecase"
)

// These variables will be set by the linker during build
// -ldflags "-X main.AppID=12345 -X main.AppHash=abcdef..."
var (
	AppID   string
	AppHash string
)

func main() {
	// Parse Flags
	pushCmd := flag.NewFlagSet("push", flag.ExitOnError)
	pullCmd := flag.NewFlagSet("pull", flag.ExitOnError)

	// Common flags
	var (
		groupID int64
		topicID int64
		dirPath string
		workers int
		verbose bool
	)

	// Helper to setup common flags
	setupFlags := func(f *flag.FlagSet) {
		f.Int64Var(&groupID, "group-id", 0, "ID of the Supergroup")
		f.Int64Var(&topicID, "topic-id", 0, "ID of the Topic")
		f.StringVar(&dirPath, "dir", "", "Path to the directory to sync")
		f.IntVar(&workers, "workers", 4, "Number of concurrent workers (not implemented yet)")
		f.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	}

	setupFlags(pushCmd)
	setupFlags(pullCmd)

	if len(os.Args) < 2 {
		fmt.Println("Usage: tgblobsync <command> [flags]")
		fmt.Println("Commands: push, pull")
		os.Exit(1)
	}

	mode := os.Args[1]

	switch mode {
	case "push":
		pushCmd.Parse(os.Args[2:])
	case "pull":
		pullCmd.Parse(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", mode)
		os.Exit(1)
	}

	if dirPath == "" {
		fmt.Println("Error: --dir is required")
		os.Exit(1)
	}

	// Validate App Credentials
	if AppID == "" || AppHash == "" {
		// Fallback for dev/debug if env vars present
		if idStr := os.Getenv("APP_ID"); idStr != "" {
			AppID = idStr
		}
		if hash := os.Getenv("APP_HASH"); hash != "" {
			AppHash = hash
		}

		if AppID == "" || AppHash == "" {
			log.Fatal("AppID and AppHash must be provided via ldflags or env vars")
		}
	}

	appIDInt, err := strconv.Atoi(AppID)
	if err != nil {
		log.Fatalf("Invalid AppID: %v", err)
	}

	// Initialize Dependencies
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	console := ui.NewConsoleUI()

	sessionPath, err := config.GetSessionPath()
	if err != nil {
		log.Fatalf("Failed to get session path: %v", err)
	}

	log.Printf("Session file: %s", sessionPath)

	tgClient, err := telegram.NewTelegramClient(appIDInt, AppHash, sessionPath, console)
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

	// Interactive Selection if needed
	if groupID == 0 {
		log.Println("Fetching groups...")
		groups, err := tgClient.ListGroups(ctx)
		if err != nil {
			log.Fatalf("Failed to list groups: %v", err)
		}

		selectedGroup, err := console.SelectGroup(groups)
		if err != nil {
			log.Fatalf("Group selection failed: %v", err)
		}
		groupID = selectedGroup.ID
		log.Printf("Selected Group: %s (%d)", selectedGroup.Title, groupID)
	} else {
		// Ensure we have the AccessHash for the provided ID
		log.Printf("Resolving group %d...", groupID)
		if err := tgClient.ResolveGroup(ctx, groupID); err != nil {
			log.Fatalf("Failed to resolve group: %v", err)
		}
	}

	if topicID == 0 {
		log.Println("Fetching topics...")
		topics, err := tgClient.ListTopics(ctx, groupID)
		if err != nil {
			log.Fatalf("Failed to list topics: %v", err)
		}

		selectedTopic, err := console.SelectTopic(topics)
		if err != nil {
			log.Fatalf("Topic selection failed: %v", err)
		}
		topicID = selectedTopic.ID
		log.Printf("Selected Topic: %s (%d)", selectedTopic.Title, topicID)
	}

	// Initialize Core Logic
	localFS := filesystem.NewLocalFileSystem()
	syncer := usecase.NewSynchronizer(localFS, tgClient, workers)

	// Execute Command
	switch mode {
	case "push":
		err = syncer.Push(ctx, dirPath, groupID, topicID)
	case "pull":
		err = syncer.Pull(ctx, dirPath, groupID, topicID)
	}

	if err != nil {
		log.Fatalf("Operation failed: %v", err)
	}

	log.Println("Done.")
}
