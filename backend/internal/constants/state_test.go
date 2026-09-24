package constants

import (
	"testing"
	"time"
)

func TestRunStateTransitions(t *testing.T) {
	valid := [][2]RunState{{RunImported, RunQualityChecked}, {RunQualityChecked, RunProcessing}, {RunProcessing, RunProcessed}, {RunProcessed, RunSuperseded}}
	for _, transition := range valid {
		if !transition[0].CanTransition(transition[1]) {
			t.Errorf("expected %s -> %s", transition[0], transition[1])
		}
	}
	if RunImported.CanTransition(RunProcessed) {
		t.Fatal("imported -> processed must be rejected")
	}
	if RunRejected.CanTransition(RunProcessing) {
		t.Fatal("rejected state must be terminal")
	}
}

func TestGapStateAndSeverity(t *testing.T) {
	if !GapDetected.CanTransition(GapReviewed) {
		t.Fatal("detected -> reviewed should be allowed")
	}
	if GapDetected.CanTransition(GapClosed) {
		t.Fatal("detected -> closed must be rejected")
	}
	if SeverityForRatio(0.13) != SeverityCritical || SeverityForRatio(0.07) != SeverityMajor || SeverityForRatio(0.01) != SeverityMinor {
		t.Fatal("severity thresholds are inconsistent")
	}
}

func TestGapDeadlines(t *testing.T) {
	cases := []struct {
		severity GapSeverity
		want     time.Duration
	}{
		{SeverityCritical, 4 * time.Hour},
		{SeverityMajor, 24 * time.Hour},
		{SeverityMinor, 72 * time.Hour},
	}
	for _, tc := range cases {
		if got := GapDeadline(tc.severity); got != tc.want {
			t.Errorf("%s deadline = %s, want %s", tc.severity, got, tc.want)
		}
	}
}
