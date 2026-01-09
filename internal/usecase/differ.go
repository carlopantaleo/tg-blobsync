package usecase

import (
	"tg-blobsync/internal/domain"
)

type SyncDiffer interface {
	DiffPush(local map[string]domain.LocalFile, remote map[string]domain.RemoteFile) domain.SyncPlan
	DiffPull(local map[string]domain.LocalFile, remote map[string]domain.RemoteFile) domain.SyncPlan
}

type differ struct {
	skipMD5 bool
}

func NewDiffer(skipMD5 bool) SyncDiffer {
	return &differ{
		skipMD5: skipMD5,
	}
}

func (d *differ) DiffPush(local map[string]domain.LocalFile, remote map[string]domain.RemoteFile) domain.SyncPlan {
	var items []domain.SyncItem
	summary := domain.SyncSummary{}

	// Check local files (Upload or Update)
	for path, localFile := range local {
		remoteFile, exists := remote[path]

		item := domain.SyncItem{
			Path:      path,
			LocalFile: &localFile,
		}

		if !exists {
			item.Action = domain.ActionUpload
			item.Reason = "New file"
			items = append(items, item)
			summary.ToUpload++
		} else {
			item.RemoteFile = &remoteFile
			if d.shouldUpdate(localFile, remoteFile) {
				item.Action = domain.ActionUpload
				item.Reason = "Changed"
				items = append(items, item)
				summary.ToUpdate++
			}
		}
	}

	// Check remote files (Delete)
	for path, remoteFile := range remote {
		if _, exists := local[path]; !exists {
			items = append(items, domain.SyncItem{
				Path:       path,
				Action:     domain.ActionDeleteRemote,
				RemoteFile: &remoteFile,
				Reason:     "Deleted locally",
			})
			summary.ToDelete++
		}
	}

	summary.Total = len(items)
	return domain.SyncPlan{Items: items, Summary: summary}
}

func (d *differ) DiffPull(local map[string]domain.LocalFile, remote map[string]domain.RemoteFile) domain.SyncPlan {
	var items []domain.SyncItem
	summary := domain.SyncSummary{}

	// Check remote files (Download or Update)
	for path, remoteFile := range remote {
		localFile, exists := local[path]

		item := domain.SyncItem{
			Path:       path,
			RemoteFile: &remoteFile,
		}

		if !exists {
			item.Action = domain.ActionDownload
			item.Reason = "New remote file"
			items = append(items, item)
			summary.ToDownload++
		} else {
			item.LocalFile = &localFile
			if d.shouldUpdate(localFile, remoteFile) {
				item.Action = domain.ActionDownload
				item.Reason = "Changed remote"
				items = append(items, item)
				summary.ToUpdate++
			}
		}
	}

	// Check local files (Delete)
	for path, localFile := range local {
		if _, exists := remote[path]; !exists {
			items = append(items, domain.SyncItem{
				Path:      path,
				Action:    domain.ActionDeleteLocal,
				LocalFile: &localFile,
				Reason:    "Deleted remotely",
			})
			summary.ToDelete++
		}
	}

	summary.Total = len(items)
	return domain.SyncPlan{Items: items, Summary: summary}
}

func (d *differ) shouldUpdate(local domain.LocalFile, remote domain.RemoteFile) bool {
	if d.skipMD5 {
		remoteSize := remote.Size
		if remote.Meta.Flags == "EMPTY_FILE" {
			remoteSize = 0
		}
		// Compare ModTime and Size
		return remote.Meta.ModTime != local.ModTime || remoteSize != local.Size
	}
	// Compare Checksum
	return remote.Meta.Checksum != local.Checksum
}
