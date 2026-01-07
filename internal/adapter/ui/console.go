package ui

import (
	"errors"
	"strconv"
	"strings"
	"tg-blobsync/internal/domain"

	"github.com/manifoldco/promptui"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// ConsoleUI handles user interactions via the terminal.
type ConsoleUI struct {
	progress *mpb.Progress
}

func NewConsoleUI() *ConsoleUI {
	return &ConsoleUI{
		progress: mpb.New(mpb.WithWidth(64)),
	}
}

// Progress Reporter Implementation

func (u *ConsoleUI) Start(name string, total int64) domain.ProgressTask {
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
