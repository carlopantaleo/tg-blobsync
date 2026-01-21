package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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

	sessionDir := filepath.Dir(cfg.SessionPath)
	sessionRepo := telegram.NewSessionManager(cfg.AppID, cfg.AppHash, sessionDir, console)

	if cfg.Command == "sessions" {
		manager := usecase.NewSessionManager(sessionRepo, console)
		return manager.Manage(ctx)
	}

	active, err := sessionRepo.GetActiveSession()
	if err != nil {
		return fmt.Errorf("failed to check active session: %w", err)
	}

	if active == nil {
		log.Println("No active session found. Please login.")
		activeInfo, err := sessionRepo.AddSession(ctx)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}
		active = &activeInfo
	}

	sessionFile := filepath.Join(sessionDir, "session_"+active.ID+".json")
	log.Printf("Using session: %s", active.ID)

	tgClient, err := telegram.NewTelegramClient(cfg.AppID, cfg.AppHash, sessionFile)
	if err != nil {
		return fmt.Errorf("failed to create telegram client: %w", err)
	}

	log.Println("Connecting to Telegram...")
	if err := tgClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start telegram client: %w", err)
	}
	defer tgClient.Close()

	log.Println("Connected!")

	tgClient.SetUploadThreads(cfg.UploadThreads)
	tgClient.SetProgressTracker(console)

	groupID, topicID, err := resolveIdentifiers(ctx, cfg, tgClient, console)
	if err != nil {
		return err
	}

	switch cfg.Command {
	case "push":
		return runSync(ctx, cfg, tgClient, console, true, groupID, topicID)
	case "pull":
		return runSync(ctx, cfg, tgClient, console, false, groupID, topicID)
	case "list":
		return runList(ctx, tgClient, console, groupID, topicID)
	default:
		return fmt.Errorf("unknown command: %s", cfg.Command)
	}
}

func resolveIdentifiers(ctx context.Context, cfg *config.CLIConfig, storage *telegram.TelegramClient, console *ui.ConsoleUI) (int64, int64, error) {
	var groupID int64
	var topicID int64

	// 1. Resolve Group
	if cfg.GroupName != "" {
		// Try to parse as ID first
		if id, err := strconv.ParseInt(cfg.GroupName, 10, 64); err == nil {
			groupID = id
			if err := storage.ResolveGroup(ctx, groupID); err != nil {
				return 0, 0, fmt.Errorf("failed to resolve group ID %d: %w", groupID, err)
			}
		} else {
			// Resolve by name
			g, err := storage.FindGroupByName(ctx, cfg.GroupName)
			if err != nil {
				return 0, 0, err
			}
			groupID = g.ID
		}
	} else {
		if cfg.NonInteractive {
			return 0, 0, fmt.Errorf("group name is required in non-interactive mode")
		}
		log.Println("Fetching groups...")
		groups, err := storage.ListGroups(ctx)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to list groups: %w", err)
		}
		selected, err := console.SelectGroup(groups)
		if err != nil {
			return 0, 0, err
		}
		groupID = selected.ID
	}

	// 2. Resolve Topic
	if cfg.TopicName != "" {
		// Try to parse as ID first
		if id, err := strconv.ParseInt(cfg.TopicName, 10, 64); err == nil {
			topicID = id
		} else {
			// Resolve by name
			t, err := storage.FindTopicByName(ctx, groupID, cfg.TopicName)
			if err != nil {
				return 0, 0, err
			}
			topicID = t.ID
		}
	} else {
		if cfg.NonInteractive {
			return 0, 0, fmt.Errorf("topic name is required in non-interactive mode")
		}
		log.Println("Fetching topics...")
		topics, err := storage.ListTopics(ctx, groupID)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to list topics: %w", err)
		}
		selected, err := console.SelectTopic(topics)
		if err != nil {
			return 0, 0, err
		}
		topicID = selected.ID
	}

	// 3. SubDir selection (if not already specified)
	if cfg.SubDir == "" && !cfg.NonInteractive && (cfg.Command == "push" || cfg.Command == "pull") {
		// Fetch existing subdirs to help user
		files, err := storage.ListFiles(ctx, groupID, topicID)
		if err == nil {
			subdirsMap := make(map[string]bool)
			for _, f := range files {
				path := filepath.ToSlash(f.Meta.Path)
				parts := strings.Split(path, "/")
				if len(parts) > 1 {
					subdirsMap[parts[0]] = true
				}
			}
			var existing []string
			for s := range subdirsMap {
				existing = append(existing, s)
			}
			sort.Strings(existing)

			selectedSubDir, err := console.SelectSubDir(existing)
			if err != nil {
				return 0, 0, err
			}
			cfg.SubDir = selectedSubDir
		}
	}

	return groupID, topicID, nil
}

func runSync(ctx context.Context, cfg *config.CLIConfig, storage *telegram.TelegramClient, ui *ui.ConsoleUI, push bool, groupID int64, topicID int64) error {
	localFS := filesystem.NewLocalFileSystem()
	syncer := usecase.NewSynchronizer(localFS, storage, cfg.Workers, ui, cfg.SkipMD5)
	syncer.SetSubDir(cfg.SubDir)

	if push {
		return syncer.Push(ctx, cfg.DirPath, groupID, topicID)
	}
	return syncer.Pull(ctx, cfg.DirPath, groupID, topicID)
}

func runList(ctx context.Context, storage *telegram.TelegramClient, ui *ui.ConsoleUI, groupID, topicID int64) error {
	browser := usecase.NewBrowser(storage, ui)
	return browser.ListAndBrowse(ctx, groupID, topicID)
}
