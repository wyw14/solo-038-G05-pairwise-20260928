package trial

import (
	"strings"
	"testing"
	"time"

	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/model"
)

var testNow = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

func testJob() model.Job {
	job := model.NewJob(testNow)
	job.PlateGeneration = 1
	return job
}

func testParameters(speed float64) Parameters {
	return Parameters{Speed: speed, Profile: dryer.Profile{Name: "test", Zones: []dryer.Zone{{Name: "entry", Temperature: 80, Airflow: 20}}}}
}

func testCriteria() Criteria {
	return Criteria{MaxWasteSheets: 30, MaxMeanOffset: 0.15, MaxMaxOffset: 0.4, MinSpeed: 6}
}

func testResult(waste int, offsets ...float64) Result {
	return Result{RegisterOffsets: offsets, WasteSheets: waste, RecordedAt: testNow}
}

func openStudy(t *testing.T) Study {
	t.Helper()
	study, err := NewStudy(testJob(), testCriteria(), testParameters(8), testNow)
	if err != nil {
		t.Fatalf("create study: %v", err)
	}
	return study
}

func TestNewStudySeedsBaselineGroup(t *testing.T) {
	study := openStudy(t)
	if study.Status != StudyOpen {
		t.Fatalf("expected open study, got %s", study.Status)
	}
	if len(study.Groups) != 1 {
		t.Fatalf("expected exactly the baseline group, got %d groups", len(study.Groups))
	}
	baseline := study.Groups[0]
	if baseline.Kind != GroupBaseline || baseline.Status != GroupCollecting {
		t.Fatalf("unexpected baseline group: %+v", baseline)
	}
	if baseline.Parameters.Speed != 8 {
		t.Fatalf("baseline should snapshot production parameters, got speed %v", baseline.Parameters.Speed)
	}
}

func TestNewStudyValidation(t *testing.T) {
	job := testJob()
	job.PlateGeneration = 0
	if _, err := NewStudy(job, testCriteria(), testParameters(8), testNow); err == nil {
		t.Fatal("expected plate generation to be required")
	}
	if _, err := NewStudy(testJob(), Criteria{MaxWasteSheets: -1}, testParameters(8), testNow); err == nil {
		t.Fatal("expected negative criteria to be rejected")
	}
	invalid := testParameters(0)
	if _, err := NewStudy(testJob(), testCriteria(), invalid, testNow); err == nil {
		t.Fatal("expected invalid baseline parameters to be rejected")
	}
}

func TestStudyLifecycle(t *testing.T) {
	study := openStudy(t)
	baseline := study.Groups[0]
	experiment, err := study.AddGroup("faster", testParameters(9.5), testNow)
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	if experiment.Kind != GroupExperiment {
		t.Fatalf("expected experiment group, got %s", experiment.Kind)
	}
	if err := study.RecordResult("missing", testResult(1, 0.1), testNow); err == nil {
		t.Fatal("expected recording against an unknown group to fail")
	}
	if err := study.RecordResult(baseline.ID, testResult(25, 0.1, -0.12, 0.08), testNow); err != nil {
		t.Fatalf("record baseline result: %v", err)
	}
	if err := study.RecordResult(experiment.ID, testResult(10, 0.04, -0.05), testNow); err != nil {
		t.Fatalf("record experiment result: %v", err)
	}
	if got, _ := study.Group(experiment.ID); got.Status != GroupComplete || !got.ResultRecorded {
		t.Fatalf("expected completed group with recorded result, got %+v", got)
	}
	if err := study.Approve(experiment.ID, testNow); err != nil {
		t.Fatalf("approve passing group: %v", err)
	}
	if study.Status != StudyApproved || study.ApprovedGroupID != experiment.ID {
		t.Fatalf("expected approved study, got %s", study.Status)
	}
	if err := study.RecordResult(baseline.ID, testResult(1, 0.1), testNow); err == nil {
		t.Fatal("expected recording after approval to fail")
	}
	if _, err := study.AddGroup("late", testParameters(7), testNow); err == nil {
		t.Fatal("expected adding a group after approval to fail")
	}
	adopted, err := study.MarkApplied(testNow)
	if err != nil {
		t.Fatalf("mark applied: %v", err)
	}
	if adopted.ID != experiment.ID {
		t.Fatalf("expected experiment to be adopted, got %s", adopted.ID)
	}
	if study.Status != StudyApplied || study.AppliedAt.IsZero() {
		t.Fatalf("expected applied study with timestamp, got %s", study.Status)
	}
	for _, group := range study.Groups {
		switch group.ID {
		case experiment.ID:
			if group.Status != GroupAdopted {
				t.Fatalf("expected adopted group, got %s", group.Status)
			}
		default:
			if group.Status != GroupNotAdopted {
				t.Fatalf("expected not adopted group, got %s", group.Status)
			}
			if !group.ResultRecorded || group.Result.WasteSheets != 25 {
				t.Fatalf("unadopted group data was not retained: %+v", group)
			}
		}
	}
	if _, err := study.MarkApplied(testNow); err == nil {
		t.Fatal("expected applying twice to fail")
	}
}

func TestApproveRequiresPassingGroup(t *testing.T) {
	study := openStudy(t)
	experiment, err := study.AddGroup("wasteful", testParameters(9), testNow)
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	if err := study.RecordResult(experiment.ID, testResult(45, 0.1), testNow); err != nil {
		t.Fatalf("record result: %v", err)
	}
	err = study.Approve(experiment.ID, testNow)
	if err == nil || !strings.Contains(err.Error(), "does not meet the study criteria") {
		t.Fatalf("expected criteria rejection, got %v", err)
	}
	if err := study.Approve("missing", testNow); err == nil {
		t.Fatal("expected approving an unknown group to fail")
	}
	if err := study.Approve(study.Groups[0].ID, testNow); err == nil {
		t.Fatal("expected approving a group without a result to fail")
	}
}

func TestRecordResultValidation(t *testing.T) {
	study := openStudy(t)
	baseline := study.Groups[0]
	if err := study.RecordResult(baseline.ID, testResult(1), testNow); err == nil {
		t.Fatal("expected empty register offsets to be rejected")
	}
	if err := study.RecordResult(baseline.ID, testResult(-1, 0.1), testNow); err == nil {
		t.Fatal("expected negative waste sheets to be rejected")
	}
	if err := study.RecordResult(baseline.ID, testResult(1, 100), testNow); err == nil {
		t.Fatal("expected out of range register offset to be rejected")
	}
	undated := testResult(1, 0.1)
	undated.RecordedAt = time.Time{}
	if err := study.RecordResult(baseline.ID, undated, testNow); err == nil {
		t.Fatal("expected a missing recording time to be rejected")
	}
}
