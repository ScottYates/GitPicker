package main

import "testing"

// TestRenderSnapshot prints the picker as it is drawn, so the layout can be
// inspected by eye.
//
//	go test -run TestRenderSnapshot -v
func TestRenderSnapshot(t *testing.T) {
	m := testModel("master", "feature/login-flow", "hotfix/urgent", "release/1.2.3")
	m.width = 78
	m.height = 18
	m.branches[0].Upstream = "origin/master"
	m.setBranches(m.branches)
	m.status = "Created and switched to feature/login-flow"
	t.Log("\n" + m.View())

	m.phase = phaseName
	m.nameBuf = "my-new-branch"
	t.Log("\n" + m.View())

	m.phase = phaseFiles
	m.files = []FileStatus{
		{Code: " M", Path: "src/ui.go"},
		{Code: "??", Path: "src/brand_new.go"},
		{Code: " D", Path: "src/old.go"},
	}
	m.fileSelected = map[string]bool{"src/ui.go": true}
	m.fileWidth = m.fileColumnWidth()
	t.Log("\n" + m.View())

	m.phase = phaseMessage
	m.messageBuf = "fix the thing that was broken"
	m.messagePos = len(m.messageBuf)
	m.history = []string{"fix the thing that was broken", "an older message"}
	m.historyIndex = 0
	t.Log("\n" + m.View())
}
