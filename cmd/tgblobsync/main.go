package main

import (
	"context"
	"fmt"
	"log"
	"os"

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
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.ParseCLI(AppID, AppHash)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	console := ui.NewConsoleUI(cfg.NonInteractive)

	log.Printf("Session file: %s", cfg.SessionPath)

	tgClient, err := telegram.NewTelegramClient(cfg.AppID, cfg.AppHash, cfg.SessionPath, console)
	if err != nil {
		return fmt.Errorf("failed to create telegram client: %w", err)
	}

	log.Println("Connecting to Telegram...")
	if err := tgClient.Start(ctx, console); err != nil {
		return fmt.Errorf("failed to start telegram client: %w", err)
	}
	defer tgClient.Close()

	log.Println("Connected!")

	tgClient.SetUploadThreads(cfg.UploadThreads)
	tgClient.SetProgressTracker(console)

	if err := ensureSelection(ctx, cfg, tgClient, console); err != nil {
		return err
	}

	switch cfg.Command {
	case "push":
		return runSync(ctx, cfg, tgClient, console, true)
	case "pull":
		return runSync(ctx, cfg, tgClient, console, false)
	case "list":
		return runList(ctx, cfg, tgClient, console)
	default:
		return fmt.Errorf("unknown command: %s", cfg.Command)
	}
}

func ensureSelection(ctx context.Context, cfg *config.CLIConfig, storage *telegram.TelegramClient, console *ui.ConsoleUI) error {
	selector := usecase.NewSelector(storage)

	if cfg.GroupID == 0 {
		log.Println("Fetching groups...")
		groups, err := selector.ListGroups(ctx)
		if err != nil {
			return fmt.Errorf("failed to list groups: %w", err)
		}

		selectedGroup, err := console.SelectGroup(groups)
		if err != nil {
			return fmt.Errorf("group selection failed: %w", err)
		}
		cfg.GroupID = selectedGroup.ID
		log.Printf("Selected Group: %s (%d)", selectedGroup.Title, cfg.GroupID)
	} else {
		log.Printf("Resolving group %d...", cfg.GroupID)
		if err := storage.ResolveGroup(ctx, cfg.GroupID); err != nil {
			return fmt.Errorf("failed to resolve group: %w", err)
		}
	}

	if cfg.TopicID == 0 {
		log.Println("Fetching topics...")
		topics, err := selector.ListTopics(ctx, cfg.GroupID)
		if err != nil {
			return fmt.Errorf("failed to list topics: %w", err)
		}

		selectedTopic, err := console.SelectTopic(topics)
		if err != nil {
			return fmt.Errorf("topic selection failed: %w", err)
		}
		cfg.TopicID = selectedTopic.ID
		log.Printf("Selected Topic: %s (%d)", selectedTopic.Title, cfg.TopicID)
	}
	return nil
}

func runSync(ctx context.Context, cfg *config.CLIConfig, storage *telegram.TelegramClient, ui *ui.ConsoleUI, push bool) error {
	localFS := filesystem.NewLocalFileSystem()
	syncer := usecase.NewSynchronizer(localFS, storage, cfg.Workers, ui, cfg.SkipMD5)
	syncer.SetSubDir(cfg.SubDir)

	if push {
		return syncer.Push(ctx, cfg.DirPath, cfg.GroupID, cfg.TopicID)
	}
	return syncer.Pull(ctx, cfg.DirPath, cfg.GroupID, cfg.TopicID)
}

func runList(ctx context.Context, cfg *config.CLIConfig, storage *telegram.TelegramClient, ui *ui.ConsoleUI) error {
	browser := usecase.NewBrowser(storage, ui)
	return browser.ListAndBrowse(ctx, cfg.GroupID, cfg.TopicID)
}
