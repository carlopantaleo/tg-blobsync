package domain

import (
	"testing"
	"time"
)

func TestEntities(t *testing.T) {
	// Basic test to ensure types are defined correctly and we can instantiate them.
	// This increases coverage for the package if there are any init() or similar (unlikely),
	// but mostly ensures we import the package.

	now := time.Now().Unix()

	remote := RemoteFile{
		Meta: FileMeta{
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

	local := LocalFile{
		Path:     "test/path",
		Checksum: "abc",
		ModTime:  now,
		Size:     100,
		AbsPath:  "/tmp/test/path",
	}

	if local.Path != "test/path" {
		t.Error("LocalFile path mismatch")
	}

	plan := SyncPlan{
		Items: []SyncItem{
			{
				Path:       "test/path",
				Action:     ActionUpload,
				LocalFile:  &local,
				RemoteFile: nil,
			},
		},
		Summary: SyncSummary{
			ToUpload: 1,
			Total:    1,
		},
	}

	if len(plan.Items) != 1 {
		t.Error("SyncPlan items count mismatch")
	}
}

func TestNavigationError(t *testing.T) {
	err := &NavigationError{Type: "download", Data: "x"}
	if err.Error() != "download" {
		t.Fatalf("unexpected error string: %s", err.Error())
	}
}
