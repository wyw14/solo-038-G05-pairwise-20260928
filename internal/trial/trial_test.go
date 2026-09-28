package trial

import (
	"errors"
	"testing"
	"time"

	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/model"
)

func testJob() model.Job {
	return model.NewJob(time.Now())
}

func testSettings(speed float64, target float64) Settings {
	return Settings{Speed: speed, Profile: dryer.Profile{Name: "curve", Zones: []dryer.Zone{{Name: "entry", Temperature: target - 20, Airflow: 20}, {Name: "exit", Temperature: target, Airflow: 24}}}}
}

func testResult(speed, deviation float64, waste int) Result {
	register, err := NewRegisterResult([]float64{deviation, -deviation, deviation / 2})
	if err != nil {
		panic(err)
	}
	return Result{Speed: speed, Profile: testSettings(speed, 100).Profile, Register: register, WasteSheets: waste}
}

func TestOpenCreatesBaselineAndBlocksSecondOpen(t *testing.T) {
	state := NewState()
	job := testJob()
	created, err := state.Open(job, 1, "plate-1 run", DefaultCriteria(), testSettings(8, 100), time.Now())
	if err != nil {
		t.Fatalf("open trial: %v", err)
	}
	if created.Status != TrialOpen || len(created.Groups) != 1 || created.Groups[0].Kind != GroupBaseline {
		t.Fatalf("trial not initialised with a baseline group: %+v", created)
	}
	if _, err := state.Open(job, 1, "second", DefaultCriteria(), testSettings(8, 100), time.Now()); !errors.Is(err, ErrTrialOpen) {
		t.Fatalf("expected ErrTrialOpen, got %v", err)
	}
}

func TestGroupLifecycleAndProductionIsolation(t *testing.T) {
	state := NewState()
	production := NewProductionStore()
	job := testJob()
	now := time.Now()
	trialValue, err := state.Open(job, 1, "", DefaultCriteria(), testSettings(8, 100), now)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	baseline := trialValue.Groups[0]

	if _, _, err := state.AddGroup(trialValue.ID, GroupExperiment, "faster", testSettings(10, 110), now); !errors.Is(err, ErrGroupOpen) {
		t.Fatalf("expected ErrGroupOpen while baseline recording, got %v", err)
	}

	if _, err := state.RecordResult(trialValue.ID, baseline.ID, testResult(8, 0.04, 120), now); err != nil {
		t.Fatalf("record baseline: %v", err)
	}

	if _, ok := production.Get(job.ID, 1); ok {
		t.Fatalf("recording trial results must not change production settings")
	}

	_, experiment, err := state.AddGroup(trialValue.ID, GroupExperiment, "faster", testSettings(10, 110), now)
	if err != nil {
		t.Fatalf("add experiment: %v", err)
	}
	trialValue, err = state.RecordResult(trialValue.ID, experiment.ID, testResult(10, 0.08, 260), now)
	if err != nil {
		t.Fatalf("record experiment: %v", err)
	}

	report, err := Compare(trialValue)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(report.Groups) != 2 || report.Groups[0].GroupID != baseline.ID {
		t.Fatalf("baseline (fewer waste sheets) should rank first: %+v", report.Groups)
	}
	if report.RecommendedGroup != baseline.ID {
		t.Fatalf("expected baseline recommendation, got %s", report.RecommendedGroup)
	}

	approved, err := state.Approve(trialValue.ID, baseline.ID, "lead-operator", now)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != TrialApproved || approved.ApprovedGroup != baseline.ID {
		t.Fatalf("trial not approved correctly: %+v", approved)
	}
	for _, group := range approved.Groups {
		switch group.ID {
		case baseline.ID:
			if group.Status != GroupApproved {
				t.Fatalf("baseline should be approved")
			}
		default:
			if group.Status != GroupNotAdopted || len(group.Results) != 1 {
				t.Fatalf("unadopted group data must be retained, got %+v", group)
			}
		}
	}

	settings := production.Adopt(approved, approved.Groups[0], now)
	if settings.Settings.Speed != 8 || settings.GroupID != baseline.ID {
		t.Fatalf("adopted production settings wrong: %+v", settings)
	}
	if _, err := state.RecordResult(trialValue.ID, experiment.ID, testResult(10, 0.1, 5), now); !errors.Is(err, ErrTrialDecided) {
		t.Fatalf("decided trial must reject more results, got %v", err)
	}
}

func TestCompareRejectsIneligibleGroup(t *testing.T) {
	state := NewState()
	job := testJob()
	now := time.Now()
	criteria := Criteria{MaxWasteSheets: 200, MaxRegisterDeviation: 0.1, WeightWaste: 1}
	trialValue, err := state.Open(job, 3, "limits", criteria, testSettings(8, 100), now)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	baseline := trialValue.Groups[0]
	if _, err := state.RecordResult(trialValue.ID, baseline.ID, testResult(8, 0.05, 100), now); err != nil {
		t.Fatalf("baseline result: %v", err)
	}
	_, loose, err := state.AddGroup(trialValue.ID, GroupExperiment, "loose", testSettings(11, 120), now)
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	trialValue, err = state.RecordResult(trialValue.ID, loose.ID, testResult(11, 0.2, 250), now)
	if err != nil {
		t.Fatalf("loose result: %v", err)
	}
	report, err := Compare(trialValue)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if report.RecommendedGroup != baseline.ID {
		t.Fatalf("only baseline is within limits, recommendation wrong: %s", report.RecommendedGroup)
	}
	for _, score := range report.Groups {
		if score.GroupID == loose.ID && score.Eligible {
			t.Fatalf("loose group violates register and waste limits: %+v", score)
		}
	}
}

func TestCompareRequiresTwoRecordedGroups(t *testing.T) {
	state := NewState()
	job := testJob()
	now := time.Now()
	trialValue, err := state.Open(job, 1, "", DefaultCriteria(), testSettings(8, 100), now)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := state.RecordResult(trialValue.ID, trialValue.Groups[0].ID, testResult(8, 0.05, 100), now); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := Compare(trialValue); !errors.Is(err, ErrNotEnoughGroups) {
		t.Fatalf("expected ErrNotEnoughGroups, got %v", err)
	}
}

func TestNewRegisterResult(t *testing.T) {
	result, err := NewRegisterResult([]float64{0.1, -0.2, 0.05})
	if err != nil {
		t.Fatalf("register result: %v", err)
	}
	if result.MaxDeviation != 0.2 || result.MeanDeviation <= 0 || result.StddevDeviation <= 0 {
		t.Fatalf("register summary wrong: %+v", result)
	}
	if _, err := NewRegisterResult(nil); err == nil {
		t.Fatalf("expected error for empty samples")
	}
}
