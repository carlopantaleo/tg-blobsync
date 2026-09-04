package usecase

import (
	"sort"
	"tg-blobsync/internal/domain"
)

type SyncDiffer interface {
	DiffPush(local map[string]domain.LocalFile, remote map[string]domain.RemoteFile) domain.SyncPlan
	DiffPull(local map[string]domain.LocalFile, remote map[string]domain.RemoteFile) domain.SyncPlan
}

type differ struct {
	skipMD5  bool
	noDelete bool
}

func NewDiffer(skipMD5 bool, noDelete ...bool) SyncDiffer {
	d := &differ{
		skipMD5: skipMD5,
	}
	if len(noDelete) > 0 {
		d.noDelete = noDelete[0]
	}
	return d
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
			summary.TotalSize += localFile.Size
		} else {
			item.RemoteFile = &remoteFile
			if d.shouldUpdate(localFile, remoteFile) {
				item.Action = domain.ActionUpload
				item.Reason = "Changed"
				items = append(items, item)
				summary.ToUpdate++
				summary.TotalSize += localFile.Size
			}
		}
	}

	// Check remote files (Delete)
	for path, remoteFile := range remote {
		if _, exists := local[path]; !exists && !d.noDelete {
			items = append(items, domain.SyncItem{
				Path:       path,
				Action:     domain.ActionDeleteRemote,
				RemoteFile: &remoteFile,
				Reason:     "Deleted locally",
			})
			summary.ToDelete++
			summary.TotalSize += remoteFile.Size
		}
	}

	// Sort items alphabetically by path
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})

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
			summary.TotalSize += remoteFile.Size
		} else {
			item.LocalFile = &localFile
			if d.shouldUpdate(localFile, remoteFile) {
				item.Action = domain.ActionDownload
				item.Reason = "Changed remote"
				items = append(items, item)
				summary.ToUpdate++
				summary.TotalSize += remoteFile.Size
			}
		}
	}

	// Check local files (Delete)
	for path, localFile := range local {
		if _, exists := remote[path]; !exists && !d.noDelete {
			items = append(items, domain.SyncItem{
				Path:      path,
				Action:    domain.ActionDeleteLocal,
				LocalFile: &localFile,
				Reason:    "Deleted remotely",
			})
			summary.ToDelete++
			summary.TotalSize += localFile.Size
		}
	}

	// Sort items alphabetically by path
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})

	summary.Total = len(items)
	return domain.SyncPlan{Items: items, Summary: summary}
}

func (d *differ) shouldUpdate(local domain.LocalFile, remote domain.RemoteFile) bool {
	if d.skipMD5 {
		return remote.Meta.ModTime != local.ModTime || remote.Size != local.Size
	}
	// Compare Checksum
	return remote.Meta.Checksum != local.Checksum
}
