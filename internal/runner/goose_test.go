package runner

import "testing"

func TestPrepareTaskPromptPreservesPolicyWhitespace(t *testing.T) {
	policy := "  inspect local repository evidence first.\nUse Context7 opportunistically.  \n"
	assignment := "Task ID: hive-123\nTitle: Fix the bug\n\nFollow the original assignment exactly."

	prompt := PrepareTaskPrompt(policy, assignment)

	want := policy + "\n\n" + hiveAssignmentHeading + "\n" + assignment
	if prompt != want {
		t.Fatalf("PrepareTaskPrompt() = %q, want %q", prompt, want)
	}
}

func TestPrepareTaskPromptReturnsAssignmentWhenPolicyEmpty(t *testing.T) {
	assignment := "Task ID: hive-123\nTitle: Fix the bug"

	if got := PrepareTaskPrompt("\n \t", assignment); got != assignment {
		t.Fatalf("PrepareTaskPrompt() = %q, want %q", got, assignment)
	}
}
