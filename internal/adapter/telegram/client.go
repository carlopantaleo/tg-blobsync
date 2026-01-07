package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// TelegramClient implements domain.BlobStorage using gotd.
type TelegramClient struct {
	client   *telegram.Client
	api      *tg.Client
	sender   *message.Sender
	uploader *uploader.Uploader
	ctx      context.Context

	peerCache      map[int64]int64 // map[ChannelID]AccessHash
	progressStarts map[int64]time.Time
	mu             sync.RWMutex
}

// AuthInput defines an interface for interactive authentication input.
type AuthInput interface {
	GetPhoneNumber() (string, error)
	GetCode() (string, error)
	GetPassword() (string, error)
}

func NewTelegramClient(appID int, appHash string, sessionFile string, input AuthInput) (*TelegramClient, error) {
	// Ensure session directory exists
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0700); err != nil {
		return nil, fmt.Errorf("failed to create session dir: %w", err)
	}

	opts := telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionFile},
	}

	client := telegram.NewClient(appID, appHash, opts)

	tc := &TelegramClient{
		client:         client,
		peerCache:      make(map[int64]int64),
		progressStarts: make(map[int64]time.Time),
	}

	return tc, nil
}

// Start connects and authenticates the client.
func (t *TelegramClient) Start(ctx context.Context, input AuthInput) error {
	t.ctx = ctx

	// We use a channel to signal when authentication is done and we are ready
	ready := make(chan error, 1)

	go func() {
		err := t.client.Run(ctx, func(ctx context.Context) error {
			// Auth flow
			status, err := t.client.Auth().Status(ctx)
			if err != nil {
				return err
			}

			if !status.Authorized {
				flow := auth.NewFlow(
					termAuth{input: input},
					auth.SendCodeOptions{},
				)
				if err := t.client.Auth().IfNecessary(ctx, flow); err != nil {
					return err
				}
			}

			// Initialize helpers
			t.api = t.client.API()
			t.sender = message.NewSender(t.api)
			t.uploader = uploader.NewUploader(t.api).WithProgress(t)

			// Signal ready
			select {
			case ready <- nil:
			default:
			}

			// Block until context done to keep connection alive
			<-ctx.Done()
			return ctx.Err()
		})
		if err != nil {
			// If Run returns error immediately
			select {
			case ready <- err:
			default:
			}
		}
	}()

	// Wait for ready signal
	select {
	case err := <-ready:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *TelegramClient) Close() error {
	return nil
}

func (t *TelegramClient) getAccessHash(id int64) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	h, ok := t.peerCache[id]
	return h, ok
}

func (t *TelegramClient) setAccessHash(id int64, hash int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peerCache[id] = hash
}
