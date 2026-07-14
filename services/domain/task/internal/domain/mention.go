package domain

import "regexp"

var mentionRegex = regexp.MustCompile(`@([\w.@-]+)`)

// ExtractMentionHandles returns unique handles from @-mentions in body.
// Handles are the raw strings after @, which may be email addresses.
func ExtractMentionHandles(body string) []string {
	matches := mentionRegex.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool, len(matches))
	handles := make([]string, 0, len(matches))
	for _, m := range matches {
		h := m[1]
		if !seen[h] {
			seen[h] = true
			handles = append(handles, h)
		}
	}
	return handles
}
