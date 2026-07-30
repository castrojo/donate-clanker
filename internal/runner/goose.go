package runner

import "strings"

const hiveAssignmentHeading = "Hive assignment (verbatim):"

func PrepareTaskPrompt(policy string, assignment string) string {
	trimmedPolicy := strings.TrimSpace(policy)
	if trimmedPolicy == "" {
		return assignment
	}

	var prompt strings.Builder
	prompt.Grow(len(policy) + len(hiveAssignmentHeading) + len(assignment) + 4)
	prompt.WriteString(policy)
	prompt.WriteString("\n\n")
	prompt.WriteString(hiveAssignmentHeading)
	prompt.WriteString("\n")
	prompt.WriteString(assignment)
	return prompt.String()
}
