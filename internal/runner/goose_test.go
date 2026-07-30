package runner

import (
	"strings"
	"testing"
)

func TestPrepareTaskPromptPrependsPolicyWithoutChangingAssignment(t *testing.T) {
	policy := "inspect local repository evidence first.\nUse Context7 opportunistically."
	assignment := "Task ID: hive-123\nTitle: Fix the bug\n\nFollow the original assignment exactly."

	prompt := PrepareTaskPrompt(policy, assignment)

	if !strings.HasPrefix(prompt, policy) {
		t.Fatalf("PrepareTaskPrompt() prefix = %q, want policy prefix %q", prompt, policy)
	}
	if !strings.Contains(prompt, "\n\nHive assignment (verbatim):\n") {
		t.Fatalf("PrepareTaskPrompt() missing assignment separator in %q", prompt)
	}
	if !strings.HasSuffix(prompt, assignment) {
		t.Fatalf("PrepareTaskPrompt() suffix = %q, want assignment suffix %q", prompt, assignment)
	}
}
