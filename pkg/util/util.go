package util

import (
	"strings"
)

func MatchPrefix(prefix string, fields ...string) ([]string, bool) {
	var prefixMatches, exactMatches []string

	for _, str := range fields {
		if str == prefix {
			exactMatches = append(exactMatches, str)
		} else if strings.HasPrefix(str, prefix) {
			prefixMatches = append(prefixMatches, str)
		}
	}

	// If we have exact matches, return them
	// and set the exact match boolean
	if len(exactMatches) > 0 {
		return exactMatches, true
	}

	return prefixMatches, false
}
