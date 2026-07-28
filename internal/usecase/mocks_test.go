package usecase

import (
	"context"
	"io"
	"strings"
	"tg-blobsync/internal/domain"
)

// MockFileSystem
type MockFileSystem struct {
	Files map[string]domain.LocalFile
	Data  map[string][]byte
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		Files: make(map[string]domain.LocalFile),
		Data:  make(map[string][]byte),
	}
}

func (m *MockFileSystem) ListFiles(root string, skipMD5 bool) ([]domain.LocalFile, error) {
	var list []domain.LocalFile
	for _, f := range m.Files {
		list = append(list, f)
	}
	return list, nil
}

func (m *MockFileSystem) ReadFile(path string) (io.ReadCloser, error) {
	if data, ok := m.Data[path]; ok {
		return io.NopCloser(strings.NewReader(string(data))), nil
	}
	return nil, io.EOF
}

func (m *MockFileSystem) WriteFile(path string, data io.Reader) error {
	content, _ := io.ReadAll(data)
	m.Data[path] = content
	return nil
}

func (m *MockFileSystem) SetModTime(path string, modTime int64) error {
	return nil
}

func (m *MockFileSystem) DeleteFile(path string) error {
	delete(m.Data, path)
	return nil
}

func (m *MockFileSystem) EnsureDir(path string) error {
	return nil
}

// MockBlobStorage
type MockBlobStorage struct {
	Files         map[int64]map[int64][]domain.RemoteFile
	Indexes       map[int64]map[int64]*domain.FileIndex
	IndexIDs      map[int64]map[int64]int
	IndexUploads  int
	StaleIndexIDs map[int64]map[int64][]int
	DeletedIDs    []int
	Groups        []domain.Group
	Topics        map[int64][]domain.Topic
	LastDeleted   struct {
		GroupID   int64
		TopicID   int64
		MessageID int
	}
	ListFilesCalls           int
	ListIndexMessageIDsCalls int
	UploadFileCalls          int
	NextMessageID            int
}

func NewMockBlobStorage() *MockBlobStorage {
	return &MockBlobStorage{
		Files:         make(map[int64]map[int64][]domain.RemoteFile),
		Indexes:       make(map[int64]map[int64]*domain.FileIndex),
		IndexIDs:      make(map[int64]map[int64]int),
		StaleIndexIDs: make(map[int64]map[int64][]int),
		Groups:        []domain.Group{},
		Topics:        make(map[int64][]domain.Topic),
		NextMessageID: 1000,
	}
}

func (m *MockBlobStorage) Start(ctx context.Context) error {
	return nil
}

func (m *MockBlobStorage) ListGroups(ctx context.Context) ([]domain.Group, error) {
	return m.Groups, nil
}
func (m *MockBlobStorage) ListTopics(ctx context.Context, groupID int64) ([]domain.Topic, error) {
	if topics, ok := m.Topics[groupID]; ok {
		return topics, nil
	}
	return nil, nil
}

func (m *MockBlobStorage) ListFiles(ctx context.Context, groupID int64, topicID int64) ([]domain.RemoteFile, error) {
	m.ListFilesCalls++
	if topics, ok := m.Files[groupID]; ok {
		if files, ok := topics[topicID]; ok {
			return files, nil
		}
	}
	return []domain.RemoteFile{}, nil
}

func (m *MockBlobStorage) GetIndex(ctx context.Context, groupID int64, topicID int64) (*domain.FileIndex, int, bool, error) {
	if groupIndexes, ok := m.Indexes[groupID]; ok {
		if index, ok := groupIndexes[topicID]; ok {
			return index, m.IndexIDs[groupID][topicID], true, nil
		}
	}
	return nil, 0, false, nil
}

func (m *MockBlobStorage) UploadIndex(ctx context.Context, groupID int64, topicID int64, index domain.FileIndex) (int, error) {
	m.IndexUploads++
	if m.Indexes[groupID] == nil {
		m.Indexes[groupID] = make(map[int64]*domain.FileIndex)
		m.IndexIDs[groupID] = make(map[int64]int)
	}
	m.NextMessageID++
	messageID := m.NextMessageID
	m.Indexes[groupID][topicID] = &index
	m.IndexIDs[groupID][topicID] = messageID
	return messageID, nil
}

func (m *MockBlobStorage) ListIndexMessageIDs(ctx context.Context, groupID int64, topicID int64) ([]int, error) {
	m.ListIndexMessageIDsCalls++
	if topics, ok := m.StaleIndexIDs[groupID]; ok {
		if ids, ok := topics[topicID]; ok {
			return ids, nil
		}
	}
	_, messageID, ok, err := m.GetIndex(ctx, groupID, topicID)
	if err != nil {
		return nil, err
	}
	if ok {
		return []int{messageID}, nil
	}
	return nil, nil
}

func (m *MockBlobStorage) GroupTotals(ctx context.Context, groupID int64) (domain.GroupTotals, error) {
	var totals domain.GroupTotals
	if topics, ok := m.Files[groupID]; ok {
		for _, files := range topics {
			for _, f := range files {
				totals.Files++
				totals.TotalSize += f.Size
			}
		}
	}
	return totals, nil
}

func (m *MockBlobStorage) UploadFile(ctx context.Context, groupID int64, topicID int64, file domain.LocalFile) ([]int, error) {
	m.UploadFileCalls++
	if m.Files[groupID] == nil {
		m.Files[groupID] = make(map[int64][]domain.RemoteFile)
	}
	m.NextMessageID++
	messageID := m.NextMessageID
	m.Files[groupID][topicID] = append(m.Files[groupID][topicID], domain.RemoteFile{
		Meta: domain.FileMeta{
			Path: file.Path,
		},
		Size:      file.Size,
		MessageID: messageID,
	})
	return []int{messageID}, nil
}

func (m *MockBlobStorage) DeleteFile(ctx context.Context, groupID int64, topicID int64, messageID int) error {
	m.LastDeleted.GroupID = groupID
	m.LastDeleted.TopicID = topicID
	m.LastDeleted.MessageID = messageID
	m.DeletedIDs = append(m.DeletedIDs, messageID)
	return nil
}

func (m *MockBlobStorage) DownloadFile(ctx context.Context, groupID int64, topicID int64, messageID int, fileName string, size int64, task domain.ProgressTask) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("dummy content")), nil
}

func (m *MockBlobStorage) Close() error {
	return nil
}

func (m *MockBlobStorage) SetProgressTracker(tracker domain.ProgressTracker) {}

// MockUserInterface
type MockUserInterface struct {
	Confirmed bool
}

func (m *MockUserInterface) ConfirmSync(plan domain.SyncPlan) (bool, error) {
	return m.Confirmed, nil
}
func (m *MockUserInterface) SetTotalFiles(total int)                            {}
func (m *MockUserInterface) Start(name string, total int64) domain.ProgressTask { return &MockTask{} }
func (m *MockUserInterface) Wait()                                              {}

func (m *MockUserInterface) GetPhoneNumber() (string, error) { return "", nil }
func (m *MockUserInterface) GetCode() (string, error)        { return "", nil }
func (m *MockUserInterface) GetPassword() (string, error)    { return "", nil }

func (m *MockUserInterface) SelectSession(sessions []domain.SessionInfo) (string, error) {
	return "", nil
}
func (m *MockUserInterface) ConfirmDeleteSession(session domain.SessionInfo) (bool, error) {
	return true, nil
}
func (m *MockUserInterface) ShowSessions(sessions []domain.SessionInfo) {}
func (m *MockUserInterface) SelectSessionAction() (string, error)       { return "exit", nil }
func (m *MockUserInterface) WaitForInput(message string) error          { return nil }

type MockTask struct{}

func (m *MockTask) Increment(n int)             {}
func (m *MockTask) SetCurrent(current int64)    {}
func (m *MockTask) SetChunk(current, total int) {}
func (m *MockTask) Complete()                   {}
func (m *MockTask) Abort()                      {}

// MockBrowseUI
type MockBrowseUI struct {
	Files         []domain.RemoteFile
	ConfirmIndex  bool
	ConfirmCalled bool
}

func (m *MockBrowseUI) BrowseFiles(files []domain.RemoteFile) (interface{}, error) {
	m.Files = files
	return nil, nil
}

func (m *MockBrowseUI) ConfirmCreateIndex(message string) (bool, error) {
	m.ConfirmCalled = true
	return m.ConfirmIndex, nil
}
