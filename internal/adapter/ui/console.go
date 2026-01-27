package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"tg-blobsync/internal/domain"
	"time"

	"github.com/vbauerster/mpb/v8"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ConsoleUI handles user interactions via the terminal.
type ConsoleUI struct {
	progress           *mpb.Progress
	totalFiles         int
	startedFiles       int
	completedFiles     int
	activeTasks        map[string]*ConsoleTask
	interactiveContent string
	mu                 sync.Mutex

	tuiProgram        *tea.Program
	tuiModel          *model
	originalLogOutput io.Writer
}

// NewConsoleUI constructs an interactive console UI.
func NewConsoleUI() *ConsoleUI {
	ui := &ConsoleUI{originalLogOutput: log.Writer()}

	m := initialModel()
	ui.tuiModel = &m
	ui.tuiProgram = tea.NewProgram(m, tea.WithAltScreen())

	// Start Bubble Tea in a goroutine
	go func() {
		if _, err := ui.tuiProgram.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		}
	}()

	// Redirect log output to TUI
	log.SetOutput(&TUIWriter{program: ui.tuiProgram})

	ui.progress = mpb.New(
		mpb.WithOutput(&TUIWriter{program: ui.tuiProgram}), // This might need careful handling
		mpb.WithWidth(64),
	)

	return ui
}

// SetCancel assigns a cancel function that will be triggered on quit (q/ctrl+c).
func (u *ConsoleUI) SetCancel(cancel context.CancelFunc) {
	if u.tuiProgram == nil {
		return
	}
	u.send(setCancelMsg{cancel: cancel})
}

// NewConsoleUITest constructs a headless ConsoleUI for tests.
func NewConsoleUITest() *ConsoleUI {
	m := initialModel()
	return &ConsoleUI{
		tuiModel:          &m,
		progress:          mpb.New(mpb.WithWidth(64)),
		originalLogOutput: log.Writer(),
	}
}

func (u *ConsoleUI) send(msg tea.Msg) {
	if u.tuiProgram != nil {
		u.tuiProgram.Send(msg)
	}
}

func (u *ConsoleUI) WaitForInput(message string) error {
	if u.tuiProgram == nil {
		return nil
	}

	// Update content and show prompt separately to ensure content is visible
	u.send(updateContentMsg(message))

	// We can use a simple prompt to wait for Enter
	ti := textinput.New()
	ti.Focus()
	u.tuiModel.promptLabel = "Press Enter to continue..."
	u.send(showPromptMsg{input: ti})

	_, ok := <-u.tuiModel.responseChan
	if !ok {
		return errors.New("quitting")
	}
	return nil
}

func (u *ConsoleUI) Close() {
	if u.tuiProgram != nil {
		u.tuiProgram.Quit()
		_ = u.tuiProgram.ReleaseTerminal()
		log.SetOutput(u.originalLogOutput)
	}
}

func (u *ConsoleUI) SetTotalFiles(total int) {
	u.mu.Lock()
	u.totalFiles = total
	u.startedFiles = 0
	u.completedFiles = 0
	u.activeTasks = make(map[string]*ConsoleTask)
	u.mu.Unlock()
	u.updateInteractive()
}

// Progress Reporter Implementation

func (u *ConsoleUI) Start(name string, total int64) domain.ProgressTask {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.startedFiles++
	task := &ConsoleTask{
		ui:        u,
		name:      name,
		total:     total,
		startTime: time.Now(),
	}
	if u.activeTasks == nil {
		u.activeTasks = make(map[string]*ConsoleTask)
	}
	u.activeTasks[name] = task
	u.updateInteractiveLocked()
	return task
}

func (u *ConsoleUI) updateInteractive() {
	if u.tuiProgram == nil {
		return
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	u.updateInteractiveLocked()
}

func (u *ConsoleUI) updateInteractiveLocked() {
	if u.tuiProgram == nil {
		return
	}
	var sb strings.Builder
	if u.totalFiles > 0 {
		sb.WriteString(fmt.Sprintf("Progress: %d/%d files completed\n\n", u.completedFiles, u.totalFiles))

		// Sort task names for consistent display
		var names []string
		for name := range u.activeTasks {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			task := u.activeTasks[name]
			percent := float64(0)
			if task.total > 0 {
				percent = float64(task.current) / float64(task.total) * 100
			}

			// Calculate speed
			speedStr := ""
			elapsed := time.Since(task.startTime).Seconds()
			if elapsed > 0 {
				speed := float64(task.current) / elapsed
				speedStr = fmt.Sprintf(" %8s/s", formatSize(int64(speed)))
			}

			// Simple progress bar
			barWidth := 20
			filled := int(float64(barWidth) * percent / 100)
			if filled > barWidth {
				filled = barWidth
			}
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

			sb.WriteString(fmt.Sprintf("%-30s [%s] %5.1f%% %s / %s%s\n", name, bar, percent, formatSize(task.current), formatSize(task.total), speedStr))
		}
	} else if u.interactiveContent != "" {
		sb.WriteString(u.interactiveContent)
	}

	u.send(updateContentMsg(sb.String()))
}

func (u *ConsoleUI) Wait() {
	u.progress.Wait()
	// Re-initialize progress for next use if needed
	u.progress = mpb.New(mpb.WithWidth(64))
}

// ConfirmSync prompts the user to confirm the sync plan.
func (u *ConsoleUI) ConfirmSync(plan domain.SyncPlan) (bool, error) {
	for {
		items := []list.Item{
			listItem{title: "Start Transfer", value: "start"},
			listItem{title: "Show Detailed Changes", value: "details"},
			listItem{title: "Cancel/Exit", value: "cancel"},
		}

		d := list.NewDefaultDelegate()
		d.ShowDescription = false
		d.SetHeight(1)
		d.SetSpacing(0)
		l := list.New(items, d, 0, 0)
		l.Title = "Action Required"

		u.send(showListMsg{list: l})

		res, ok := <-u.tuiModel.responseChan
		if !ok {
			return false, errors.New("quitting")
		}
		if item, ok := res.(listItem); ok {
			switch item.value.(string) {
			case "start":
				u.mu.Lock()
				u.interactiveContent = ""
				u.totalFiles = len(plan.Items) // Ensure total files is set for progress bars
				u.mu.Unlock()
				u.updateInteractive()
				return true, nil
			case "details":
				u.showDetailedChanges(plan)
			case "cancel":
				return false, nil
			}
		} else {
			return false, errors.New("selection cancelled")
		}
	}
}

func (u *ConsoleUI) showDetailedChanges(plan domain.SyncPlan) {
	if u.tuiProgram == nil {
		return
	}
	var items []list.Item
	for _, item := range plan.Items {
		var actionStr string
		var prefix string
		switch item.Action {
		case domain.ActionUpload:
			prefix = "[+]"
			actionStr = "Upload"
		case domain.ActionDownload:
			prefix = "[v]"
			actionStr = "Download"
		case domain.ActionDeleteRemote:
			prefix = "[-]"
			actionStr = "Delete Remote"
		case domain.ActionDeleteLocal:
			prefix = "[-]"
			actionStr = "Delete Local"
		case domain.ActionSkip:
			prefix = "[.]"
			actionStr = "Skip"
		}

		title := fmt.Sprintf("%-3s %-12s %-40s | %s", prefix, actionStr, item.Path, item.Reason)
		items = append(items, listItem{title: title, value: item})
	}

	// Add a back option
	items = append([]list.Item{listItem{title: ".. [Back to Confirmation]", value: "back"}}, items...)

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)

	l := list.New(items, d, 0, 0)
	l.Title = fmt.Sprintf("Detailed Changes (%d items)", len(plan.Items))

	u.send(showListMsg{list: l})

	// Wait for user to go back
	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return
	}

	// Whatever the user selects (unless it's back, we already handled the navigation),
	// we just return to the main confirmation loop.
	// The caller (ConfirmSync) is in a loop, so it will re-show the confirmation menu.
	_ = res
}

type ConsoleTask struct {
	ui        *ConsoleUI
	name      string
	total     int64
	mu        sync.Mutex
	current   int64
	startTime time.Time
}

func (t *ConsoleTask) Increment(n int) {
	t.mu.Lock()
	t.current += int64(n)
	t.mu.Unlock()
	t.ui.updateInteractive()
}

func (t *ConsoleTask) SetCurrent(current int64) {
	t.mu.Lock()
	t.current = current
	t.mu.Unlock()
	t.ui.updateInteractive()
}

func (t *ConsoleTask) Complete() {
	t.ui.mu.Lock()
	t.ui.completedFiles++
	delete(t.ui.activeTasks, t.name)
	t.ui.mu.Unlock()
	t.ui.updateInteractive()
}

func (t *ConsoleTask) Abort() {
	t.ui.mu.Lock()
	delete(t.ui.activeTasks, t.name)
	t.ui.mu.Unlock()
	t.ui.updateInteractive()
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// SelectSession prompts the user to select a session from the list.
func (u *ConsoleUI) SelectSession(sessions []domain.SessionInfo) (string, error) {
	if len(sessions) == 0 {
		return "", errors.New("no sessions available")
	}

	var items []list.Item
	for _, s := range sessions {
		active := ""
		if s.IsActive {
			active = " (Active)"
		}
		items = append(items, listItem{
			title: fmt.Sprintf("%s%s", s.ID, active),
			value: s.ID,
		})
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = "Select Session"

	u.send(showListMsg{list: l})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return "", errors.New("quitting")
	}
	if item, ok := res.(listItem); ok {
		return item.value.(string), nil
	}

	return "", errors.New("selection cancelled")
}

// ConfirmDeleteSession prompts the user to confirm session deletion.
func (u *ConsoleUI) ConfirmDeleteSession(session domain.SessionInfo) (bool, error) {
	items := []list.Item{
		listItem{title: "Yes, Delete", value: true},
		listItem{title: "No, Keep", value: false},
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = fmt.Sprintf("Delete session %s?", session.ID)

	u.send(showListMsg{list: l})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return false, errors.New("quitting")
	}
	if item, ok := res.(listItem); ok {
		return item.value.(bool), nil
	}

	return false, nil
}

// ShowSessions displays available sessions in the interactive area.
func (u *ConsoleUI) ShowSessions(sessions []domain.SessionInfo) {
	var sb strings.Builder
	sb.WriteString("--- Available Sessions ---\n")
	if len(sessions) == 0 {
		sb.WriteString("No sessions found.\n")
	} else {
		for _, s := range sessions {
			active := ""
			if s.IsActive {
				active = " [ACTIVE]"
			}
			sb.WriteString(fmt.Sprintf("- %s%s\n", s.ID, active))
		}
	}
	sb.WriteString("--------------------------\n")
	u.send(updateContentMsg(sb.String()))
}

// SelectSessionAction prompts the user for a session action.
func (u *ConsoleUI) SelectSessionAction() (string, error) {
	items := []list.Item{
		listItem{title: "Create New Session", value: "create"},
		listItem{title: "Select Active Session", value: "select"},
		listItem{title: "Delete Session", value: "delete"},
		listItem{title: "Exit", value: "exit"},
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = "Choose Action"

	u.send(showListMsg{list: l})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return "exit", errors.New("quitting")
	}
	if item, ok := res.(listItem); ok {
		return item.value.(string), nil
	}

	return "exit", nil
}

// GetPhoneNumber prompts the user for the phone number.
func (u *ConsoleUI) GetPhoneNumber() (string, error) {
	ti := textinput.New()
	ti.Placeholder = "+39..."
	ti.Focus()

	u.tuiModel.promptLabel = "Enter Phone Number (international format, e.g. +39...)"
	u.send(showPromptMsg{input: ti})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return "", errors.New("quitting")
	}
	if s, ok := res.(string); ok {
		return s, nil
	}
	return "", errors.New("prompt cancelled")
}

func (u *ConsoleUI) GetCode() (string, error) {
	ti := textinput.New()
	ti.Placeholder = "12345"
	ti.Focus()

	u.tuiModel.promptLabel = "Enter Code"
	u.send(showPromptMsg{input: ti})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return "", errors.New("quitting")
	}
	if s, ok := res.(string); ok {
		return s, nil
	}
	return "", errors.New("prompt cancelled")
}

// GetPassword prompts the user for their 2FA password.
func (u *ConsoleUI) GetPassword() (string, error) {
	ti := textinput.New()
	ti.Placeholder = "Password"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Focus()

	u.tuiModel.promptLabel = "Enter 2FA Password"
	u.send(showPromptMsg{input: ti})

	res := <-u.tuiModel.responseChan
	if s, ok := res.(string); ok {
		return s, nil
	}
	return "", errors.New("prompt cancelled")
}

// SelectGroup prompts the user to select a group from the list.
func (u *ConsoleUI) SelectGroup(groups []domain.Group) (domain.Group, error) {
	if len(groups) == 0 {
		return domain.Group{}, errors.New("no groups available")
	}

	var items []list.Item
	for _, g := range groups {
		items = append(items, listItem{title: g.Title, value: g})
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = "Select Group"

	u.send(showListMsg{list: l})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return domain.Group{}, errors.New("quitting")
	}
	if g, ok := res.(listItem); ok {
		return g.value.(domain.Group), nil
	}

	return domain.Group{}, errors.New("selection cancelled")
}

// SelectTopic prompts the user to select a topic from the list.
func (u *ConsoleUI) SelectTopic(topics []domain.Topic) (domain.Topic, error) {
	if len(topics) == 0 {
		return domain.Topic{}, errors.New("no topics available")
	}

	var items []list.Item
	items = append(items, listItem{title: ".. [Back to Groups]", value: "back"})
	for _, t := range topics {
		items = append(items, listItem{title: t.Title, value: t})
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = "Select Topic"

	u.send(showListMsg{list: l})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return domain.Topic{}, errors.New("quitting")
	}
	if item, ok := res.(listItem); ok {
		if val, ok := item.value.(string); ok && val == "back" {
			return domain.Topic{}, errors.New("back")
		}
		return item.value.(domain.Topic), nil
	}
	return domain.Topic{}, errors.New("selection cancelled")
}

// SelectSubDir prompts the user for a subdirectory path.
func (u *ConsoleUI) SelectSubDir(existingSubDirs []string) (string, error) {
	items := []list.Item{
		listItem{title: ".. [Back to Groups]", value: "back"},
		listItem{title: "[ Root / No subdirectory ]", value: ""},
		listItem{title: "[ Enter custom path ]", value: "custom"},
	}
	for _, s := range existingSubDirs {
		items = append(items, listItem{title: "\U0001F4C1 " + s, value: s})
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = "Select or enter subdirectory"

	u.send(showListMsg{list: l})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return "", errors.New("quitting")
	}
	if item, ok := res.(listItem); ok {
		val, isStr := item.value.(string)
		if isStr && val == "back" {
			return "", errors.New("back")
		}
		if isStr && val == "custom" {
			return u.Prompt("Enter custom subdirectory path")
		}
		return val, nil
	}

	return "", errors.New("selection cancelled")
}

// AskToCreateTopic prompts to create a new topic if needed (Not in requirements but useful)
// We'll stick to requirements: "User selects the topic".

// Helper to prompt for generic text
func (u *ConsoleUI) Prompt(label string) (string, error) {
	ti := textinput.New()
	ti.Focus()

	u.tuiModel.promptLabel = label
	u.send(showPromptMsg{input: ti})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return "", errors.New("quitting")
	}
	if s, ok := res.(string); ok {
		return s, nil
	}
	return "", errors.New("prompt cancelled")
}

func (u *ConsoleUI) PromptInt(label string) (int64, error) {
	res, err := u.Prompt(label)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(res, 10, 64)
}

// BrowseFiles allows interactive navigation of the virtual directory structure.
func (u *ConsoleUI) BrowseFiles(files []domain.RemoteFile) (interface{}, error) {
	if len(files) == 0 {
		u.send(updateContentMsg("No files to browse."))
		return nil, nil
	}

	currentDir := ""
	for {
		// Filter items in current directory
		menu, currentDirTotalSize, err := u.buildBrowserItems(files, currentDir)
		if err != nil {
			return nil, err
		}

		displayDir := currentDir
		if displayDir == "" {
			displayDir = "/"
		}

		var items []list.Item
		for _, entry := range menu {
			items = append(items, listItem{
				title: entry.Label,
				value: entry,
			})
		}

		d := list.NewDefaultDelegate()
		d.ShowDescription = false
		d.SetHeight(1)
		d.SetSpacing(0)
		l := list.New(items, d, 0, 0)
		l.Title = fmt.Sprintf("Browse Files - %s (%s)", displayDir, formatSize(currentDirTotalSize))

		u.send(showListMsg{list: l})

		res, ok := <-u.tuiModel.responseChan
		if !ok {
			return nil, errors.New("quitting")
		}
		selectedItem, ok := res.(listItem)
		if !ok {
			return nil, errors.New("browsing cancelled")
		}

		selected := selectedItem.value.(browserMenuEntry)
		if selected.Label == "Exit" {
			return nil, nil
		}

		if selected.Label == ".. [Back to Groups]" {
			return nil, errors.New("back")
		}

		if selected.IsDir {
			if selected.Label == ".. [Go Up]" {
				parts := strings.Split(currentDir, "/")
				if len(parts) <= 1 {
					currentDir = ""
				} else {
					currentDir = strings.Join(parts[:len(parts)-1], "/")
				}
				continue
			}

			dirName := selected.DirName
			if currentDir == "" {
				currentDir = dirName
			} else {
				currentDir = currentDir + "/" + dirName
			}
			continue
		}

		if selected.File != nil {
			action, err := u.showFileDetails(selected.File)
			if err != nil {
				return nil, err
			}
			if action == "download" {
				return &domain.DownloadRequest{File: *selected.File}, nil
			}
		}
	}
}

type browserMenuEntry struct {
	Label   string
	IsDir   bool
	DirName string
	File    *domain.RemoteFile
}

func (u *ConsoleUI) buildBrowserItems(files []domain.RemoteFile, currentDir string) ([]browserMenuEntry, int64, error) {
	type dirInfo struct {
		Size  int64
		IsDir bool
	}
	items := make(map[string]*dirInfo) // name -> info
	var filesInDir []domain.RemoteFile
	var currentDirTotalSize int64

	for _, f := range files {
		path := filepath.ToSlash(f.Meta.Path)
		if currentDir == "" {
			parts := strings.Split(path, "/")
			if len(parts) > 1 {
				if _, ok := items[parts[0]]; !ok {
					items[parts[0]] = &dirInfo{IsDir: true}
				}
				items[parts[0]].Size += f.Size
			} else {
				filesInDir = append(filesInDir, f)
			}
			currentDirTotalSize += f.Size
		} else {
			if strings.HasPrefix(path, currentDir+"/") {
				relPath := strings.TrimPrefix(path, currentDir+"/")
				parts := strings.Split(relPath, "/")
				if len(parts) > 1 {
					if _, ok := items[parts[0]]; !ok {
						items[parts[0]] = &dirInfo{IsDir: true}
					}
					items[parts[0]].Size += f.Size
				} else {
					filesInDir = append(filesInDir, f)
				}
				currentDirTotalSize += f.Size
			}
		}
	}

	var menu []browserMenuEntry
	menu = append(menu, browserMenuEntry{Label: ".. [Back to Groups]", IsDir: false})

	if currentDir != "" {
		menu = append(menu, browserMenuEntry{Label: ".. [Go Up]", IsDir: true})
	}

	// Add directories
	var sortedDirs []string
	for d := range items {
		sortedDirs = append(sortedDirs, d)
	}
	sort.Strings(sortedDirs)

	for _, d := range sortedDirs {
		info := items[d]
		label := fmt.Sprintf("\U0001F4C1 %-30s %10s", d, formatSize(info.Size))
		menu = append(menu, browserMenuEntry{Label: label, IsDir: true, DirName: d})
	}

	// Add files
	sort.Slice(filesInDir, func(i, j int) bool {
		return filepath.Base(filesInDir[i].Meta.Path) < filepath.Base(filesInDir[j].Meta.Path)
	})

	for _, f := range filesInDir {
		modTime := time.Unix(f.Meta.ModTime, 0).Format("2006-01-02 15:04:05")
		label := fmt.Sprintf("\U0001F4C4 %-30s %10s  %s", filepath.Base(f.Meta.Path), formatSize(f.Size), modTime)
		fCopy := f // copy to avoid loop variable capture issues
		menu = append(menu, browserMenuEntry{Label: label, IsDir: false, File: &fCopy})
	}

	menu = append(menu, browserMenuEntry{Label: "Exit", IsDir: false})

	return menu, currentDirTotalSize, nil
}

func (u *ConsoleUI) showFileDetails(f *domain.RemoteFile) (string, error) {
	items := []list.Item{
		listItem{title: "Download File", value: "download"},
		listItem{title: ".. [Back to List]", value: "back"},
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = fmt.Sprintf("File: %s (%s)", filepath.Base(f.Meta.Path), formatSize(f.Size))

	var sb strings.Builder
	sb.WriteString("--- File Details ---\n")
	sb.WriteString(fmt.Sprintf("Path:     %s\n", f.Meta.Path))
	sb.WriteString(fmt.Sprintf("Size:     %s\n", formatSize(f.Size)))
	sb.WriteString(fmt.Sprintf("ModTime:  %s\n", time.Unix(f.Meta.ModTime, 0).Format(time.RFC3339)))
	if f.Meta.Checksum != "" {
		sb.WriteString(fmt.Sprintf("Checksum: %s\n", f.Meta.Checksum))
	}
	sb.WriteString(fmt.Sprintf("MsgID:    %d\n", f.MessageID))
	sb.WriteString("--------------------\n")

	u.send(updateContentMsg(sb.String()))
	u.send(showListMsg{list: l})

	res, ok := <-u.tuiModel.responseChan
	if !ok {
		return "back", errors.New("quitting")
	}
	if item, ok := res.(listItem); ok {
		return item.value.(string), nil
	}
	return "back", nil
}
