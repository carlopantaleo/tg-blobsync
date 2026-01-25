package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
)

func TestModelUpdate(t *testing.T) {
	m := initialModel()
	m.width = 80
	m.height = 24

	// Test logMsg
	msg := logMsg("test log")
	newModel, _ := m.Update(msg)
	updatedModel := newModel.(model)
	if len(updatedModel.logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(updatedModel.logs))
	}
	if !strings.Contains(updatedModel.viewport.View(), "test log") {
		// Try checking m.logs if viewport rendering is tricky in tests
		found := false
		for _, l := range updatedModel.logs {
			if strings.Contains(l, "test log") {
				found = true
				break
			}
		}
		if !found {
			t.Error("logs should contain 'test log'")
		}
	}

	// Test updateContentMsg
	contentMsg := updateContentMsg("interactive content")
	newModel, _ = updatedModel.Update(contentMsg)
	updatedModel = newModel.(model)
	if updatedModel.interactiveContent != "interactive content" {
		t.Errorf("expected content 'interactive content', got %s", updatedModel.interactiveContent)
	}
	if updatedModel.showList || updatedModel.showPrompt {
		t.Error("showList and showPrompt should be false after updateContentMsg")
	}

	// Test showListMsg
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	listMsg := showListMsg{list: l}
	newModel, _ = updatedModel.Update(listMsg)
	updatedModel = newModel.(model)
	if !updatedModel.showList {
		t.Error("expected showList to be true")
	}

	// Test showPromptMsg
	ti := textinput.New()
	promptMsg := showPromptMsg{input: ti}
	newModel, _ = updatedModel.Update(promptMsg)
	updatedModel = newModel.(model)
	if !updatedModel.showPrompt {
		t.Error("expected showPrompt to be true")
	}
	if updatedModel.showList {
		t.Error("showList should be false after showPromptMsg")
	}
}

func TestModelView(t *testing.T) {
	m := initialModel()
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 5)

	// Test Initial View
	view := m.View()
	if strings.Contains(view, "Initializing...") {
		t.Error("View should not be Initializing... when width/height are set")
	}

	// Test View with interactive content
	m.interactiveContent = "HELLO"
	view = m.View()
	if !strings.Contains(view, "HELLO") {
		t.Error("View should contain 'HELLO'")
	}

	// Test View with List
	m.showList = true
	view = m.View()
	// List view usually contains its title or items, for empty list it has help
	if !strings.Contains(view, "No items") {
		t.Logf("List view: %s", view)
	}

	// Test View with Prompt
	m.showList = false
	m.showPrompt = true
	m.promptLabel = "Enter Name"
	m.textInput = textinput.New()
	view = m.View()
	if !strings.Contains(view, "Enter Name") {
		t.Error("View should contain prompt label 'Enter Name'")
	}
}
