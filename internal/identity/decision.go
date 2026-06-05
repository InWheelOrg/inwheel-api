/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

// Kind discriminates a Decision's outcome.
type Kind int

const (
	// KindConfident means the matcher found a place with score >= ConfidentThreshold.
	KindConfident Kind = iota
	// KindLowConfidence means the best score was in [LowConfidenceThreshold, ConfidentThreshold).
	KindLowConfidence
	// KindNoMatch means no candidate scored at or above LowConfidenceThreshold.
	KindNoMatch
)

// String returns a stable lowercase tag for logs and metrics.
func (k Kind) String() string {
	switch k {
	case KindConfident:
		return "confident"
	case KindLowConfidence:
		return "low_confidence"
	case KindNoMatch:
		return "no_match"
	default:
		return "unknown"
	}
}

// Decision is the matcher's answer for one Record.
type Decision struct {
	// Kind is the outcome category.
	Kind Kind
	// MatchedPlaceID is the chosen place's ID. Empty when Kind == KindNoMatch.
	MatchedPlaceID string
	// Confidence is the best candidate's score in [0, 1]. Zero when Kind == KindNoMatch.
	Confidence float64
}
