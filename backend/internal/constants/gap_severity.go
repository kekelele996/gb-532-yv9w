package constants

import (
	"fmt"
	"time"
)

type GapSeverity string
type GapState string

const (
	SeverityMinor    GapSeverity = "minor"
	SeverityMajor    GapSeverity = "major"
	SeverityCritical GapSeverity = "critical"

	GapDetected      GapState = "detected"
	GapReviewed      GapState = "reviewed"
	GapAccepted      GapState = "accepted"
	GapFalsePositive GapState = "false_positive"
	GapResurveyed    GapState = "resurveyed"
	GapClosed        GapState = "closed"
)

var gapTransitions = map[GapState]map[GapState]struct{}{
	GapDetected:      {GapReviewed: {}},
	GapReviewed:      {GapAccepted: {}, GapFalsePositive: {}},
	GapAccepted:      {GapResurveyed: {}},
	GapFalsePositive: {GapClosed: {}},
	GapResurveyed:    {GapClosed: {}},
	GapClosed:        {},
}

func (s GapState) Valid() bool {
	_, ok := gapTransitions[s]
	return ok
}

func (s GapState) CanTransition(target GapState) bool {
	allowed, ok := gapTransitions[s]
	if !ok {
		return false
	}
	_, ok = allowed[target]
	return ok
}

func ParseGapState(value string) (GapState, error) {
	state := GapState(value)
	if !state.Valid() {
		return "", fmt.Errorf("unknown gap state %q", value)
	}
	return state, nil
}

func SeverityForRatio(ratio float64) GapSeverity {
	switch {
	case ratio >= 0.12:
		return SeverityCritical
	case ratio >= 0.05:
		return SeverityMajor
	default:
		return SeverityMinor
	}
}

// GapDeadline 返回缺口复核期限：严重 4 小时、主要 1 天、轻微 3 天，自检测时间起算。
func GapDeadline(severity GapSeverity) time.Duration {
	switch severity {
	case SeverityCritical:
		return 4 * time.Hour
	case SeverityMajor:
		return 24 * time.Hour
	default:
		return 72 * time.Hour
	}
}
