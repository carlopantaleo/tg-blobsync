package ui

import (
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
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ConsoleUI handles user interactions via the terminal.
type ConsoleUI struct {
	progress       *mpb.Progress
	nonInteractive bool
	totalFiles     int
	startedFiles   int
	completedFiles int
	mu             sync.Mutex

	tuiProgram        *tea.Program
	tuiModel          *model
	originalLogOutput io.Writer
}

func NewConsoleUI(nonInteractive bool) *ConsoleUI {
	ui := &ConsoleUI{
		nonInteractive:    nonInteractive,
		originalLogOutput: log.Writer(),
	}

	if !nonInteractive {
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
	}

	return ui
}

func (u *ConsoleUI) Close() {
	if u.tuiProgram != nil {
		u.tuiProgram.Quit()
		log.SetOutput(u.originalLogOutput)
	}
}

func (u *ConsoleUI) SetTotalFiles(total int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.totalFiles = total
	u.startedFiles = 0
	u.completedFiles = 0
}

// Progress Reporter Implementation

func (u *ConsoleUI) Start(name string, total int64) domain.ProgressTask {
	u.mu.Lock()
	u.startedFiles++
	currentFileNum := u.startedFiles
	totalFiles := u.totalFiles
	u.mu.Unlock()

	displayName := name
	if totalFiles > 0 {
		displayName = fmt.Sprintf("[%d/%d] %s", currentFileNum, totalFiles, name)
	}

	if u.nonInteractive {
		return &nonInteractiveTask{
			name:      displayName,
			total:     total,
			startTime: time.Now(),
			onComplete: func() {
				u.mu.Lock()
				u.completedFiles++
				u.mu.Unlock()
			},
		}
	}

	bar := u.progress.AddBar(total,
		mpb.PrependDecorators(
			decor.Name(displayName, decor.WC{W: len(displayName) + 1}),
			decor.Counters(decor.SizeB1024(0), "% .2f / % .2f", decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			decor.OnComplete(
				decor.Percentage(decor.WCSyncSpace), "done",
			),
			decor.AverageSpeed(decor.SizeB1024(0), "% .2f", decor.WCSyncSpace),
		),
	)
	return &mpbTask{
		bar: bar,
		onComplete: func() {
			u.mu.Lock()
			u.completedFiles++
			u.mu.Unlock()
		},
	}
}

func (u *ConsoleUI) Wait() {
	if u.nonInteractive {
		return
	}
	u.progress.Wait()
	// Re-initialize progress for next use if needed
	u.progress = mpb.New(mpb.WithWidth(64))
}

// ConfirmSync prompts the user to confirm the sync plan.
func (u *ConsoleUI) ConfirmSync(plan domain.SyncPlan) (bool, error) {
	if u.nonInteractive {
		return true, nil
	}

	for {
		items := []list.Item{
			listItem{title: "Start Transfer", value: "start"},
			listItem{title: "Show Detailed Changes", value: "details"},
			listItem{title: "Cancel/Exit", value: "cancel"},
		}

		l := list.New(items, list.NewDefaultDelegate(), 0, 0)
		l.Title = "Action Required"

		u.tuiProgram.Send(showListMsg{list: l})

		res := <-u.tuiModel.responseChan
		if item, ok := res.(listItem); ok {
			switch item.value.(string) {
			case "start":
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
	var sb strings.Builder
	sb.WriteString("--- Detailed Changes ---\n\nActions:\n")
	for _, item := range plan.Items {
		symbol := "?"
		actionName := ""

		switch item.Action {
		case domain.ActionUpload:
			if item.RemoteFile != nil {
				symbol = "[*] Update"
				actionName = "Upload (update)"
			} else {
				symbol = "[+] New   "
				actionName = "Upload (new)"
			}
		case domain.ActionDownload:
			if item.LocalFile != nil {
				symbol = "[*] Update"
				actionName = "Download (update)"
			} else {
				symbol = "[v] New   "
				actionName = "Download (new)"
			}
		case domain.ActionDeleteRemote:
			symbol = "[-] Delete"
			actionName = "Delete Remote"
		case domain.ActionDeleteLocal:
			symbol = "[-] Delete"
			actionName = "Delete Local"
		case domain.ActionSkip:
			symbol = "[.] Skip  "
			actionName = "Skip"
		}

		reasonStr := ""
		if item.Reason != "" {
			reasonStr = fmt.Sprintf(" (%s)", item.Reason)
		}

		sb.WriteString(fmt.Sprintf("  %s %-40s %-20s %s\n", symbol, item.Path, actionName, reasonStr))
	}
	sb.WriteString("------------------------\n")

	if u.nonInteractive {
		fmt.Print(sb.String())
		return
	}

	u.tuiProgram.Send(updateContentMsg(sb.String()))
	u.Prompt("Press Enter to continue")
}

type mpbTask struct {
	bar        *mpb.Bar
	onComplete func()
}

func (t *mpbTask) Increment(n int) {
	t.bar.IncrBy(n)
}

func (t *mpbTask) SetCurrent(current int64) {
	t.bar.SetCurrent(current)
}

func (t *mpbTask) Complete() {
	t.bar.SetTotal(-1, true)
	if t.onComplete != nil {
		t.onComplete()
	}
}

func (t *mpbTask) Abort() {
	t.bar.Abort(true)
}

type nonInteractiveTask struct {
	name       string
	total      int64
	current    int64
	startTime  time.Time
	onComplete func()
}

func (t *nonInteractiveTask) Increment(n int) {
	t.current += int64(n)
}

func (t *nonInteractiveTask) SetCurrent(current int64) {
	t.current = current
}

func (t *nonInteractiveTask) Complete() {
	elapsed := time.Since(t.startTime).Seconds()
	speed := float64(t.current) / elapsed
	fmt.Printf("Finished: %s | Size: %s | Speed: %s/s\n",
		t.name,
		formatSize(t.current),
		formatSize(int64(speed)),
	)
	if t.onComplete != nil {
		t.onComplete()
	}
}

func (t *nonInteractiveTask) Abort() {
	fmt.Printf("Failed: %s (Transfer aborted due to error)\n", t.name)
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

	if u.nonInteractive {
		return "", errors.New("cannot select session in non-interactive mode")
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

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select Session"

	u.tuiProgram.Send(showListMsg{list: l})

	res := <-u.tuiModel.responseChan
	if item, ok := res.(listItem); ok {
		return item.value.(string), nil
	}

	return "", errors.New("selection cancelled")
}

// ConfirmDeleteSession prompts the user to confirm session deletion.
func (u *ConsoleUI) ConfirmDeleteSession(session domain.SessionInfo) (bool, error) {
	if u.nonInteractive {
		return false, nil
	}

	items := []list.Item{
		listItem{title: "Yes, Delete", value: true},
		listItem{title: "No, Keep", value: false},
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = fmt.Sprintf("Delete session %s?", session.ID)

	u.tuiProgram.Send(showListMsg{list: l})

	res := <-u.tuiModel.responseChan
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
	u.tuiProgram.Send(updateContentMsg(sb.String()))
}

// SelectSessionAction prompts the user for a session action.
func (u *ConsoleUI) SelectSessionAction() (string, error) {
	if u.nonInteractive {
		return "exit", nil
	}

	items := []list.Item{
		listItem{title: "Create New Session", value: "create"},
		listItem{title: "Select Active Session", value: "select"},
		listItem{title: "Delete Session", value: "delete"},
		listItem{title: "Exit", value: "exit"},
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Choose Action"

	u.tuiProgram.Send(showListMsg{list: l})

	res := <-u.tuiModel.responseChan
	if item, ok := res.(listItem); ok {
		return item.value.(string), nil
	}

	return "exit", nil
}

// GetPhoneNumber prompts the user for the phone number.
func (u *ConsoleUI) GetPhoneNumber() (string, error) {
	if u.nonInteractive {
		return "", errors.New("cannot prompt for phone number in non-interactive mode")
	}

	ti := textinput.New()
	ti.Placeholder = "+39..."
	ti.Focus()

	u.tuiModel.promptLabel = "Enter Phone Number (international format, e.g. +39...)"
	u.tuiProgram.Send(showPromptMsg{input: ti})

	res := <-u.tuiModel.responseChan
	if s, ok := res.(string); ok {
		return s, nil
	}
	return "", errors.New("prompt cancelled")
}

// GetCode prompts the user for the authentication code.
func (u *ConsoleUI) GetCode() (string, error) {
	if u.nonInteractive {
		return "", errors.New("cannot prompt for code in non-interactive mode")
	}

	ti := textinput.New()
	ti.Placeholder = "12345"
	ti.Focus()

	u.tuiModel.promptLabel = "Enter Code"
	u.tuiProgram.Send(showPromptMsg{input: ti})

	res := <-u.tuiModel.responseChan
	if s, ok := res.(string); ok {
		return s, nil
	}
	return "", errors.New("prompt cancelled")
}

// GetPassword prompts the user for their 2FA password.
func (u *ConsoleUI) GetPassword() (string, error) {
	if u.nonInteractive {
		return "", errors.New("cannot prompt for password in non-interactive mode")
	}

	ti := textinput.New()
	ti.Placeholder = "Password"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Focus()

	u.tuiModel.promptLabel = "Enter 2FA Password"
	u.tuiProgram.Send(showPromptMsg{input: ti})

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

	if u.nonInteractive {
		return domain.Group{}, errors.New("cannot select group in non-interactive mode")
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

	u.tuiProgram.Send(showListMsg{list: l})

	res := <-u.tuiModel.responseChan
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

	if u.nonInteractive {
		return domain.Topic{}, errors.New("cannot select topic in non-interactive mode")
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

	u.tuiProgram.Send(showListMsg{list: l})

	res := <-u.tuiModel.responseChan
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
	if u.nonInteractive {
		return "", nil
	}

	items := []list.Item{
		listItem{title: ".. [Back to Topics]", value: "back"},
		listItem{title: "[ Root / No subdirectory ]", value: ""},
		listItem{title: "[ Enter custom path ]", value: "custom"},
	}
	for _, s := range existingSubDirs {
		items = append(items, listItem{title: s, value: s})
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	l := list.New(items, d, 0, 0)
	l.Title = "Select or enter subdirectory"

	u.tuiProgram.Send(showListMsg{list: l})

	res := <-u.tuiModel.responseChan
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
	if u.nonInteractive {
		return "", errors.New("cannot prompt in non-interactive mode")
	}

	ti := textinput.New()
	ti.Focus()

	u.tuiModel.promptLabel = label
	u.tuiProgram.Send(showPromptMsg{input: ti})

	res := <-u.tuiModel.responseChan
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
func (u *ConsoleUI) BrowseFiles(files []domain.RemoteFile) error {
	if len(files) == 0 {
		u.tuiProgram.Send(updateContentMsg("No files to browse."))
		return nil
	}

	currentDir := ""
	for {
		// Filter items in current directory
		menu, currentDirTotalSize, err := u.buildBrowserItems(files, currentDir)
		if err != nil {
			return err
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

		u.tuiProgram.Send(showListMsg{list: l})

		res := <-u.tuiModel.responseChan
		selectedItem, ok := res.(listItem)
		if !ok {
			return errors.New("browsing cancelled")
		}

		selected := selectedItem.value.(browserMenuEntry)
		if selected.Label == "Exit Browser" {
			return nil
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
			u.showFileDetails(selected.File)
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

	menu = append(menu, browserMenuEntry{Label: "Exit Browser", IsDir: false})

	return menu, currentDirTotalSize, nil
}

func (u *ConsoleUI) showFileDetails(f *domain.RemoteFile) {
	var sb strings.Builder
	sb.WriteString("\n--- File Details ---\n")
	sb.WriteString(fmt.Sprintf("Path:     %s\n", f.Meta.Path))
	sb.WriteString(fmt.Sprintf("Size:     %s\n", formatSize(f.Size)))
	sb.WriteString(fmt.Sprintf("ModTime:  %s\n", time.Unix(f.Meta.ModTime, 0).Format(time.RFC3339)))
	if f.Meta.Checksum != "" {
		sb.WriteString(fmt.Sprintf("Checksum: %s\n", f.Meta.Checksum))
	}
	if f.Meta.Flags != "" {
		sb.WriteString(fmt.Sprintf("Flags:    %s\n", f.Meta.Flags))
	}
	sb.WriteString(fmt.Sprintf("MsgID:    %d\n", f.MessageID))
	sb.WriteString("--------------------\n\n")

	u.tuiProgram.Send(updateContentMsg(sb.String()))
	u.Prompt("Press Enter to continue browsing")
}
