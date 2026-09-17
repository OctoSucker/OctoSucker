package evaluation

import (
	"testing"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
)

func TestNormalizeAssessmentKeepsProgressAndRoutingIndependent(t *testing.T) {
	assessment, err := normalizeAssessment(assessmentJSON{
		Progress:       "continue",
		RoutingOutcome: "helpful",
		RoutingReason:  "necessary_prerequisite",
		Summary:        "The skill activation is required before using its tools.",
		NextStepHint:   "Use the activated skill.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Progress != model.ProgressContinue {
		t.Fatalf("progress = %q", assessment.Progress)
	}
	if assessment.RoutingOutcome != model.RoutingHelpful {
		t.Fatalf("routing outcome = %q", assessment.RoutingOutcome)
	}
	if assessment.RoutingReason != model.RoutingReasonNecessaryPrerequisite {
		t.Fatalf("routing reason = %q", assessment.RoutingReason)
	}
}

func TestNormalizeAssessmentRejectsUnknownRoutingReason(t *testing.T) {
	_, err := normalizeAssessment(assessmentJSON{
		Progress:       "complete",
		RoutingOutcome: "helpful",
		RoutingReason:  "looks_good",
		Summary:        "done",
	})
	if err == nil {
		t.Fatal("expected invalid routing reason to fail")
	}
}

func TestNormalizeAssessmentRequiresEvidenceForComplete(t *testing.T) {
	_, err := normalizeAssessment(assessmentJSON{
		Progress:       "complete",
		RoutingOutcome: "helpful",
		RoutingReason:  "goal_satisfied",
		Summary:        "done",
	})
	if err == nil {
		t.Fatal("expected missing evidence to fail")
	}
}

func TestNormalizeAssessmentAcceptsEvidenceBackedComplete(t *testing.T) {
	assessment, err := normalizeAssessment(assessmentJSON{
		Progress:          "complete",
		RoutingOutcome:    "helpful",
		RoutingReason:     "goal_satisfied",
		Summary:           "requested file was written",
		Evidence:          "tool result reports path=/workspace/report.md and status=written",
		CriteriaSatisfied: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Evidence == "" {
		t.Fatal("evidence was discarded")
	}
}
