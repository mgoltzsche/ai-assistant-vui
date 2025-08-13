package vui

import (
	"strings"
)

func renderPromptTemplate(prompt, wakeWord string) string {
	return strings.ReplaceAll(prompt, "{wakeWord}", wakeWord)
}
