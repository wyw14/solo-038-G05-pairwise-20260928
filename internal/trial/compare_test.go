package trial

import (
	"testing"
	"time"
)

func recordResults(t *testing.T, study Study, results map[string]Result) Study {
	t.Helper()
	baseline := study.Groups[0]
	if _, ok := results[baseline.ID]; !ok {
		if err := study.RecordResult(baseline.ID, testResult(40, 0.2, 0.3), testNow); err != nil {
			t.Fatalf("record baseline result: %v", err)
		}
	}
	for groupID, result := range results {
		if err := study.RecordResult(groupID, result, testNow); err != nil {
			t.Fatalf("record result for %s: %v", groupID, err)
		}
	}
	return study
}

func TestCompareRanksByCriteria(t *testing.T) {
	study := openStudy(t)
	baseline := study.Groups[0]
	fast, err := study.AddGroup("fast", testParameters(9), testNow)
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	steady, err := study.AddGroup("steady", testParameters(9.5), testNow.Add(time.Second))
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	pending, err := study.AddGroup("pending", testParameters(7), testNow.Add(2*time.Second))
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	study = recordResults(t, study, map[string]Result{
		fast.ID:   testResult(12, 0.05, -0.05),
		steady.ID: testResult(20, 0.1),
	})
	comparison := Compare(study, testNow)
	if comparison.BaselineGroupID != baseline.ID {
		t.Fatalf("expected baseline %s, got %s", baseline.ID, comparison.BaselineGroupID)
	}
	if len(comparison.PendingGroupIDs) != 1 || comparison.PendingGroupIDs[0] != pending.ID {
		t.Fatalf("expected pending group %s, got %v", pending.ID, comparison.PendingGroupIDs)
	}
	if len(comparison.Evaluations) != 3 {
		t.Fatalf("expected 3 evaluations, got %d", len(comparison.Evaluations))
	}
	if comparison.Evaluations[0].GroupID != fast.ID {
		t.Fatalf("expected fewest waste sheets first, got %s", comparison.Evaluations[0].GroupID)
	}
	if comparison.Evaluations[1].GroupID != steady.ID {
		t.Fatalf("expected steady second, got %s", comparison.Evaluations[1].GroupID)
	}
	last := comparison.Evaluations[2]
	if last.GroupID != baseline.ID || last.Passed {
		t.Fatalf("expected failing baseline last, got %+v", last)
	}
	if len(last.Violations) == 0 {
		t.Fatal("expected baseline violations to be listed")
	}
	if comparison.RecommendedGroupID != fast.ID {
		t.Fatalf("expected fast to be recommended, got %s", comparison.RecommendedGroupID)
	}
}

func TestCompareTieBreaksOnRegisterThenSpeed(t *testing.T) {
	study := openStudy(t)
	wobblier, err := study.AddGroup("wobblier", testParameters(9.5), testNow)
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	smoother, err := study.AddGroup("smoother", testParameters(9), testNow.Add(time.Second))
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	study = recordResults(t, study, map[string]Result{
		wobblier.ID: testResult(10, 0.1, 0.1),
		smoother.ID: testResult(10, 0.05, 0.05),
	})
	comparison := Compare(study, testNow)
	if comparison.Evaluations[0].GroupID != smoother.ID {
		t.Fatalf("expected steadier register to win the waste tie, got %s", comparison.Evaluations[0].GroupID)
	}
}

func TestCompareWithNoPassingGroup(t *testing.T) {
	study := openStudy(t)
	experiment, err := study.AddGroup("wasteful", testParameters(9), testNow)
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	study = recordResults(t, study, map[string]Result{experiment.ID: testResult(90, 0.6)})
	comparison := Compare(study, testNow)
	if comparison.RecommendedGroupID != "" {
		t.Fatalf("expected no recommendation, got %s", comparison.RecommendedGroupID)
	}
	for _, evaluation := range comparison.Evaluations {
		if evaluation.Passed {
			t.Fatalf("expected every group to fail, got %+v", evaluation)
		}
	}
}

func TestEvaluateGroupWithoutResult(t *testing.T) {
	study := openStudy(t)
	evaluation := EvaluateGroup(study.Groups[0], study.Criteria)
	if evaluation.Passed || evaluation.ResultRecorded {
		t.Fatalf("expected unrecorded group to fail, got %+v", evaluation)
	}
	if len(evaluation.Violations) != 1 || evaluation.Violations[0] != "result not recorded" {
		t.Fatalf("unexpected violations: %v", evaluation.Violations)
	}
}
