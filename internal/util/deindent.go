package util

import "strings"

func Deindent(s string) string {
	lines := strings.Split(s, "\n")

	// Drop leading/trailing completely empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}

	// Find common indent (spaces/tabs) of all non-empty lines
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		i := 0
		for i < len(line) {
			r := rune(line[i])
			if r != ' ' && r != '\t' {
				break
			}
			i++
		}
		if minIndent == -1 || i < minIndent {
			minIndent = i
		}
	}

	// Remove that indent
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		if minIndent > len(line) {
			minIndent = len(line)
		}
		lines[i] = line[minIndent:]
	}
	return strings.Join(lines, "\n")
}
