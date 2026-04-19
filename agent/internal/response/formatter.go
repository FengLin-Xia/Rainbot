package response

import (
	"strings"
)

// Format assembles the final user-facing text from a StructuredResponse
// without any style processing (used as fallback).
func Format(sr StructuredResponse) string {
	if sr.FinalAnswer != "" {
		return sr.FinalAnswer
	}
	if len(sr.KeyPoints) > 0 {
		return strings.Join(sr.KeyPoints, "\n")
	}
	return ""
}
