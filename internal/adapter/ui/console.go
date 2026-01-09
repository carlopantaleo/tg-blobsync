package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"tg-blobsync/internal/domain"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// ConsoleUI handles user interactions via the terminal.
type ConsoleUI struct {
	progress       *mpb.Progress
	nonInteractive bool
}

func NewConsoleUI(nonInteractive bool) *ConsoleUI {
	var p *mpb.Progress
	if !nonInteractive {
		p = mpb.New(mpb.WithWidth(64))
	}
	return &ConsoleUI{
		progress:       p,
		nonInteractive: nonInteractive,
	}
}

// Progress Reporter Implementation

func (u *ConsoleUI) Start(name string, total int64) domain.ProgressTask {
	if u.nonInteractive {
		return &nonInteractiveTask{
			name:      name,
			total:     total,
			startTime: time.Now(),
		}
	}

	bar := u.progress.AddBar(total,
		mpb.PrependDecorators(
			decor.Name(name, decor.WC{W: len(name) + 1}),
			decor.Counters(decor.SizeB1024(0), "% .2f / % .2f", decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			decor.OnComplete(
				decor.Percentage(decor.WCSyncSpace), "done",
			),
			decor.AverageSpeed(decor.SizeB1024(0), "% .2f", decor.WCSyncSpace),
		),
	)
	return &mpbTask{bar: bar}
}

func (u *ConsoleUI) Wait() {
	if u.nonInteractive {
		return
	}
	u.progress.Wait()
	// Re-initialize progress for next use if needed
	u.progress = mpb.New(mpb.WithWidth(64))
}

type mpbTask struct {
	bar *mpb.Bar
}

func (t *mpbTask) Increment(n int) {
	t.bar.IncrBy(n)
}

func (t *mpbTask) SetCurrent(current int64) {
	t.bar.SetCurrent(current)
}

func (t *mpbTask) Complete() {
	t.bar.SetTotal(-1, true)
}

func (t *mpbTask) Abort() {
	t.bar.Abort(true)
}

type nonInteractiveTask struct {
	name      string
	total     int64
	current   int64
	startTime time.Time
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

// GetPhoneNumber prompts the user for their phone number.
func (u *ConsoleUI) GetPhoneNumber() (string, error) {
	prompt := promptui.Prompt{
		Label: "Enter Phone Number (international format, e.g. +39...)",
		Validate: func(input string) error {
			if len(input) < 5 {
				return errors.New("phone number too short")
			}
			return nil
		},
	}
	return prompt.Run()
}

// GetCode prompts the user for the authentication code.
func (u *ConsoleUI) GetCode() (string, error) {
	prompt := promptui.Prompt{
		Label: "Enter Code",
		Validate: func(input string) error {
			if len(input) == 0 {
				return errors.New("code cannot be empty")
			}
			return nil
		},
	}
	return prompt.Run()
}

// GetPassword prompts the user for their 2FA password.
func (u *ConsoleUI) GetPassword() (string, error) {
	prompt := promptui.Prompt{
		Label: "Enter 2FA Password",
		Mask:  '*',
	}
	return prompt.Run()
}

// SelectGroup prompts the user to select a group from the list.
func (u *ConsoleUI) SelectGroup(groups []domain.Group) (domain.Group, error) {
	if len(groups) == 0 {
		return domain.Group{}, errors.New("no groups available")
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "\U0001F449 {{ .Title | cyan }}",
		Inactive: "  {{ .Title | white }}",
		Selected: "\U0001F44D {{ .Title | green | cyan }}",
	}

	prompt := promptui.Select{
		Label:     "Select Group",
		Items:     groups,
		Templates: templates,
		Size:      10,
		Searcher: func(input string, index int) bool {
			group := groups[index]
			name := strings.Replace(strings.ToLower(group.Title), " ", "", -1)
			input = strings.Replace(strings.ToLower(input), " ", "", -1)
			return strings.Contains(name, input)
		},
	}

	i, _, err := prompt.Run()
	if err != nil {
		return domain.Group{}, err
	}

	return groups[i], nil
}

// SelectTopic prompts the user to select a topic from the list.
func (u *ConsoleUI) SelectTopic(topics []domain.Topic) (domain.Topic, error) {
	// Add an option to create a new topic? Or just select existing.
	// Spec says "User selects the topic".

	if len(topics) == 0 {
		return domain.Topic{}, errors.New("no topics available")
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "\U0001F449 {{ .Title | cyan }}",
		Inactive: "  {{ .Title | white }}",
		Selected: "\U0001F44D {{ .Title | green | cyan }}",
	}

	prompt := promptui.Select{
		Label:     "Select Topic",
		Items:     topics,
		Templates: templates,
		Size:      10,
		Searcher: func(input string, index int) bool {
			topic := topics[index]
			name := strings.Replace(strings.ToLower(topic.Title), " ", "", -1)
			input = strings.Replace(strings.ToLower(input), " ", "", -1)
			return strings.Contains(name, input)
		},
	}

	i, _, err := prompt.Run()
	if err != nil {
		return domain.Topic{}, err
	}

	return topics[i], nil
}

// AskToCreateTopic prompts to create a new topic if needed (Not in requirements but useful)
// We'll stick to requirements: "User selects the topic".

// Helper to prompt for generic text
func (u *ConsoleUI) Prompt(label string) (string, error) {
	prompt := promptui.Prompt{
		Label: label,
	}
	return prompt.Run()
}

func (u *ConsoleUI) PromptInt(label string) (int64, error) {
	prompt := promptui.Prompt{
		Label: label,
		Validate: func(input string) error {
			_, err := strconv.ParseInt(input, 10, 64)
			return err
		},
	}
	res, err := prompt.Run()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(res, 10, 64)
}

// BrowseFiles allows interactive navigation of the virtual directory structure.
func (u *ConsoleUI) BrowseFiles(files []domain.RemoteFile) error {
	if len(files) == 0 {
		fmt.Println("No files to browse.")
		return nil
	}

	currentDir := ""
	for {
		// Filter items in current directory
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

		type menuEntry struct {
			Label   string
			IsDir   bool
			DirName string
			File    *domain.RemoteFile
		}

		var menu []menuEntry
		if currentDir != "" {
			menu = append(menu, menuEntry{Label: ".. [Go Up]", IsDir: true})
		}

		// Add directories
		var sortedDirs []string
		for d := range items {
			sortedDirs = append(sortedDirs, d)
		}
		for _, d := range sortedDirs {
			info := items[d]
			label := fmt.Sprintf("\U0001F4C1 %s (%s)", d, formatSize(info.Size))
			menu = append(menu, menuEntry{Label: label, IsDir: true, DirName: d})
		}

		// Add files
		for _, f := range filesInDir {
			label := fmt.Sprintf("\U0001F4C4 %s (%s)", filepath.Base(f.Meta.Path), formatSize(f.Size))
			menu = append(menu, menuEntry{Label: label, IsDir: false, File: &f})
		}

		menu = append(menu, menuEntry{Label: "Exit Browser", IsDir: false})

		displayDir := currentDir
		if displayDir == "" {
			displayDir = "/"
		}

		templates := &promptui.SelectTemplates{
			Label:    fmt.Sprintf("Current directory: %s (%s)", displayDir, formatSize(currentDirTotalSize)),
			Active:   "\U0001F449 {{ .Label | cyan }}",
			Inactive: "  {{ .Label | white }}",
			Selected: "{{ if .File }}\U0001F44D {{ .Label | green }}{{ else }}\U0001F44D {{ .Label | yellow }}{{ end }}",
		}

		prompt := promptui.Select{
			Label:     "Browse Files",
			Items:     menu,
			Templates: templates,
			Size:      15,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			return err
		}

		selected := menu[idx]
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
			f := selected.File
			fmt.Printf("\n--- File Details ---\n")
			fmt.Printf("Path:     %s\n", f.Meta.Path)
			fmt.Printf("Size:     %s\n", formatSize(f.Size))
			fmt.Printf("ModTime:  %s\n", time.Unix(f.Meta.ModTime, 0).Format(time.RFC3339))
			if f.Meta.Checksum != "" {
				fmt.Printf("Checksum: %s\n", f.Meta.Checksum)
			}
			if f.Meta.Flags != "" {
				fmt.Printf("Flags:    %s\n", f.Meta.Flags)
			}
			fmt.Printf("MsgID:    %d\n", f.MessageID)
			fmt.Printf("--------------------\n\n")

			promptContinue := promptui.Prompt{
				Label:     "Press Enter to continue browsing",
				IsConfirm: false,
			}
			promptContinue.Run()
		}
	}
}
