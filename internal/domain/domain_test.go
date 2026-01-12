package domain_test

import (
	"testing"
	"time"

	"tg-blobsync/internal/domain"
)

func TestEntities(t *testing.T) {
	// Basic test to ensure types are defined correctly and we can instantiate them.
	// This increases coverage for the package if there are any init() or similar (unlikely),
	// but mostly ensures we import the package.

	now := time.Now().Unix()

	remote := domain.RemoteFile{
		Meta: domain.FileMeta{
			Path:     "test/path",
			Checksum: "abc",
			ModTime:  now,
		},
		MessageID: 1,
		Size:      100,
	}

	if remote.Meta.Path != "test/path" {
		t.Error("RemoteFile path mismatch")
	}

	local := domain.LocalFile{
		Path:     "test/path",
		Checksum: "abc",
		ModTime:  now,
		Size:     100,
		AbsPath:  "/tmp/test/path",
	}

	if local.Path != "test/path" {
		t.Error("LocalFile path mismatch")
	}

	plan := domain.SyncPlan{
		Items: []domain.SyncItem{
			{
				Path:       "test/path",
				Action:     domain.ActionUpload,
				LocalFile:  &local,
				RemoteFile: nil,
			},
		},
		Summary: domain.SyncSummary{
			ToUpload: 1,
			Total:    1,
		},
	}

	if len(plan.Items) != 1 {
		t.Error("SyncPlan items count mismatch")
	}
}
