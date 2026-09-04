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
	"tg-blobsync/internal/domain"
	"tg-blobsync/internal/usecase"
)

// These variables will be set by the linker during build
// -ldflags "-X main.AppID=12345 -X main.AppHash=abcdef..."
var (
	AppID   string
	AppHash string
)

type identifierResolver interface {
	ResolveGroup(ctx context.Context, groupID int64) error
	FindGroupByName(ctx context.Context, name string) (*domain.Group, error)
	FindTopicByName(ctx context.Context, groupID int64, name string) (*domain.Topic, error)
	ListGroups(ctx context.Context) ([]domain.Group, error)
	ListTopics(ctx context.Context, groupID int64) ([]domain.Topic, error)
	ListFiles(ctx context.Context, groupID int64, topicID int64) ([]domain.RemoteFile, error)
	GroupTotals(ctx context.Context, groupID int64) (domain.GroupTotals, error)
}

type selectionUI interface {
	SelectGroup(groups []domain.Group) (domain.Group, error)
	SelectTopic(topics []domain.Topic) (domain.Topic, error)
	SelectSubDir(existingSubDirs []string, currentPath string) (ui.SubDirSelection, error)
	ShowGroupTotals(totals domain.GroupTotals) error
	ConfirmCreateIndex(message string) (bool, error)
}

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

	console := ui.NewConsoleUI()
	console.SetCancel(cancel)
	defer console.Close()

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

	tgClient, err := telegram.NewTelegramClientWithChunking(cfg.AppID, cfg.AppHash, sessionFile, cfg.ChunkThreshold, cfg.ChunkSize)
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

restart_identifiers:
	groupID, topicID, err := resolveIdentifiers(ctx, cfg, tgClient, tgClient, console)
	if err != nil {
		return err
	}

	switch cfg.Command {
	case "push":
		err = runSync(ctx, cfg, tgClient, console, true, groupID, topicID)
	case "pull":
		err = runSync(ctx, cfg, tgClient, console, false, groupID, topicID)
	case "browse":
		err = runBrowse(ctx, cfg, tgClient, console, groupID, topicID)
	default:
		return fmt.Errorf("unknown command: %s", cfg.Command)
	}

	if err != nil {
		if err.Error() == "quitting" {
			return nil
		}
		if err.Error() == "back" {
			goto restart_identifiers
		}
		if navErr, ok := err.(*domain.NavigationError); ok && navErr.Type == "download" {
			if req, ok := navErr.Data.(*domain.DownloadRequest); ok {
				err = handleSingleDownload(ctx, cfg, tgClient, console, req, groupID, topicID)
				if err == nil {
					goto restart_identifiers // Return to browser after download
				}
			}
		}
	}
	return err
}

// immediateSubDirs returns the sorted, deduplicated names of the directories
// directly contained in the given prefix, derived from remote file paths.
// Paths use forward slashes; prefix may be empty (topic root).
func immediateSubDirs(files []domain.RemoteFile, prefix string) []string {
	searchPrefix := ""
	if prefix != "" {
		searchPrefix = prefix + "/"
	}
	seen := make(map[string]bool)
	for _, f := range files {
		path := filepath.ToSlash(f.Meta.Path)
		if !strings.HasPrefix(path, searchPrefix) {
			continue
		}
		rest := strings.TrimPrefix(path, searchPrefix)
		if idx := strings.Index(rest, "/"); idx > 0 {
			seen[rest[:idx]] = true
		}
	}
	var result []string
	for s := range seen {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

func resolveIdentifiers(ctx context.Context, cfg *config.CLIConfig, storage identifierResolver, blobStorage domain.BlobStorage, console selectionUI) (int64, int64, error) {
	for {
		groupID, topicID, err := resolveIdentifiersInternal(ctx, cfg, storage, blobStorage, console)
		if err != nil {
			if err.Error() == "quitting" {
				return 0, 0, err
			}
			if err.Error() == "back" {
				// Reset names to force re-selection if we were in a "back" flow
				cfg.GroupName = ""
				cfg.TopicName = ""
				cfg.SubDir = ""
				continue
			}
		}
		return groupID, topicID, err
	}
}

func resolveIdentifiersInternal(ctx context.Context, cfg *config.CLIConfig, storage identifierResolver, blobStorage domain.BlobStorage, console selectionUI) (int64, int64, error) {
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
			if g == nil {
				return 0, 0, fmt.Errorf("group not found: %s", cfg.GroupName)
			}
			groupID = g.ID
		}
	} else {
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
	for {
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
				if t == nil {
					return 0, 0, fmt.Errorf("topic not found: %s", cfg.TopicName)
				}
				topicID = t.ID
			}
		} else {
			log.Println("Fetching topics...")
			topics, err := storage.ListTopics(ctx, groupID)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to list topics: %w", err)
			}
			selected, err := console.SelectTopic(topics)
			if err != nil {
				if err.Error() == "back" {
					cfg.GroupName = "" // Reset group to force re-selection
					return 0, 0, err
				}
				if err.Error() == "totals" {
					totalsCalc := usecase.NewGroupTotalsCalculator(blobStorage, console)
					totals, tErr := totalsCalc.Compute(ctx, groupID)
					if tErr != nil {
						return 0, 0, tErr
					}
					_ = console.ShowGroupTotals(totals)
					continue
				}
				return 0, 0, err
			}
			topicID = selected.ID
		}

		// 3. SubDir selection
		if cfg.SubDir == "" && (cfg.Command == "push" || cfg.Command == "pull") {
			var files []domain.RemoteFile
			var err error

			index, _, indexed, getIndexErr := blobStorage.GetIndex(ctx, groupID, topicID)
			if getIndexErr == nil && indexed {
				files = index.RemoteFiles()
			} else {
				files, err = storage.ListFiles(ctx, groupID, topicID)
			}

			if err == nil && len(files) > 0 {
				sel, err := console.SelectSubDir(immediateSubDirs(files, ""), "")
				if err != nil {
					if err.Error() == "back" {
						cfg.TopicName = "" // Reset topic to force re-selection
						return 0, 0, err   // Propagate back to caller to avoid tight loop
					}
					return 0, 0, err
				}
				switch sel.Action {
				case ui.SubDirUp:
					// At root, going up returns to topic selection
					cfg.TopicName = ""
					return 0, 0, fmt.Errorf("back")
				case ui.SubDirEnter, ui.SubDirCustom:
					cfg.SubDir = sel.Value
				case ui.SubDirThis:
					cfg.SubDir = ""
				}
			}
		}
		break
	}

	return groupID, topicID, nil
}

func runSync(ctx context.Context, cfg *config.CLIConfig, storage domain.BlobStorage, ui domain.UserInterface, push bool, groupID int64, topicID int64) error {
	localFS := filesystem.NewLocalFileSystem()
	syncer := usecase.NewSynchronizer(localFS, storage, cfg.Workers, ui, cfg.SkipMD5)
	syncer.SetSubDir(cfg.SubDir)

	if push {
		return syncer.Push(ctx, cfg.DirPath, groupID, topicID)
	}
	return syncer.Pull(ctx, cfg.DirPath, groupID, topicID)
}

func runBrowse(ctx context.Context, cfg *config.CLIConfig, storage domain.BlobStorage, console usecase.BrowseUI, groupID, topicID int64) error {
	browser := usecase.NewBrowser(storage, console)
	err := browser.ListAndBrowse(ctx, groupID, topicID)
	return err
}

func handleSingleDownload(ctx context.Context, cfg *config.CLIConfig, storage domain.BlobStorage, ui domain.UserInterface, req *domain.DownloadRequest, groupID, topicID int64) error {
	localFS := filesystem.NewLocalFileSystem()
	downloader := usecase.NewSingleDownloader(localFS, storage, ui)
	return downloader.Download(ctx, req, cfg.DirPath, groupID, topicID)
}
