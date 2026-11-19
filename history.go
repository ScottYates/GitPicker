package main

// The commit message history.
//
// The last ten messages are kept in a dot file in the user's home folder, so
// the picker can offer them back. The file holds a JSON array of strings,
// newest first.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	// historyFileName is the dot file the history lives in.
	historyFileName = ".git-picker-commit-history"

	// historyLimit is how many messages are kept.
	historyLimit = 10
)

// homeDir is the user's home folder.
//
// It is a variable so a test can point it somewhere else.
var homeDir = os.UserHomeDir

// historyPath is where the history file lives, or "" when there is no home
// folder to put it in.
func historyPath() string {
	home, err := homeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, historyFileName)
}

// loadHistory reads the saved messages, newest first.
//
// A file that is missing, empty or unreadable is not an error: the picker
// simply starts with no history.
func loadHistory() []string {
	path := historyPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var messages []string
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil
	}
	if len(messages) > historyLimit {
		messages = messages[:historyLimit]
	}
	return messages
}

// saveHistory writes the messages back, newest first.
func saveHistory(messages []string) error {
	path := historyPath()
	if path == "" {
		return nil
	}
	if len(messages) > historyLimit {
		messages = messages[:historyLimit]
	}
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// rememberMessage puts a message at the front of the history.
//
// A message that is already in the history moves to the front rather than
// appearing twice, so the list holds the last ten distinct messages.
func rememberMessage(messages []string, message string) []string {
	message = strings.TrimSpace(message)
	if message == "" {
		return messages
	}
	updated := make([]string, 0, historyLimit)
	updated = append(updated, message)
	for _, existing := range messages {
		if existing != message && len(updated) < historyLimit {
			updated = append(updated, existing)
		}
	}
	return updated
}
