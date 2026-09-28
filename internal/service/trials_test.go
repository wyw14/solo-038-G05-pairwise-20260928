package service

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/trial"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime, err := NewRuntime(Config{DataDir: t.TempDir(), FestoonCapacity: 120, RegisterGap: 500 * time.Millisecond, RegisterTolerance: 2})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	return runtime
}

func experimentParameters() trial.Parameters {
	return trial.Parameters{Speed: 9.5, Profile: dryer.Profile{Name: "exp-hot", Zones: []dryer.Zone{{Name: "entry", Temperature: 90, Airflow: 22}, {Name: "exit", Temperature: 120, Airflow: 26}}}}
}

func approveStudy(t *testing.T, runtime *Runtime) (trial.Study, trial.Group) {
	t.Helper()
	criteria := trial.Criteria{MaxWasteSheets: 30, MaxMeanOffset: 0.15, MaxMaxOffset: 0.4, MinSpeed: 6}
	study, err := runtime.CreateTrialStudy(context.Background(), criteria)
	if err != nil {
		t.Fatalf("create study: %v", err)
	}
	experiment, err := runtime.AddTrialGroup(study.ID, "hot-fast", experimentParameters())
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	baseline := study.Groups[0]
	if _, err := runtime.RecordTrialResult(study.ID, baseline.ID, trial.Result{RegisterOffsets: []float64{0.1, -0.1}, WasteSheets: 20}); err != nil {
		t.Fatalf("record baseline result: %v", err)
	}
	if _, err := runtime.RecordTrialResult(study.ID, experiment.ID, trial.Result{RegisterOffsets: []float64{0.05}, WasteSheets: 8}); err != nil {
		t.Fatalf("record experiment result: %v", err)
	}
	if _, err := runtime.ApproveTrialGroup(study.ID, experiment.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return study, experiment
}

func TestTrialPhaseKeepsProductionUntilApply(t *testing.T) {
	runtime := newTestRuntime(t)
	before := runtime.ProductionParameters()
	if before.Speed != 8 || before.Profile.Name != "production" {
		t.Fatalf("unexpected default production parameters: %+v", before)
	}

	criteria := trial.Criteria{MaxWasteSheets: 30, MaxMeanOffset: 0.15, MaxMaxOffset: 0.4, MinSpeed: 6}
	study, err := runtime.CreateTrialStudy(context.Background(), criteria)
	if err != nil {
		t.Fatalf("create study: %v", err)
	}
	if len(study.Groups) != 1 || !reflect.DeepEqual(study.Groups[0].Parameters, before) {
		t.Fatalf("baseline should snapshot production parameters, got %+v", study.Groups[0].Parameters)
	}
	experiment, err := runtime.AddTrialGroup(study.ID, "hot-fast", experimentParameters())
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	baseline := study.Groups[0]
	if _, err := runtime.RecordTrialResult(study.ID, baseline.ID, trial.Result{RegisterOffsets: []float64{0.1, -0.1}, WasteSheets: 20}); err != nil {
		t.Fatalf("record baseline result: %v", err)
	}
	if _, err := runtime.RecordTrialResult(study.ID, experiment.ID, trial.Result{RegisterOffsets: []float64{0.05}, WasteSheets: 8}); err != nil {
		t.Fatalf("record experiment result: %v", err)
	}
	comparison, err := runtime.EvaluateTrialStudy(study.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if comparison.RecommendedGroupID != experiment.ID {
		t.Fatalf("expected experiment to be recommended, got %s", comparison.RecommendedGroupID)
	}
	if got := runtime.ProductionParameters(); !reflect.DeepEqual(got, before) {
		t.Fatalf("trial phase must not change production, got %+v", got)
	}
	if _, err := runtime.ApproveTrialGroup(study.ID, experiment.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got := runtime.ProductionParameters(); !reflect.DeepEqual(got, before) {
		t.Fatalf("approval alone must not change production, got %+v", got)
	}

	applied, err := runtime.ApplyTrialStudy(context.Background(), study.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Status != trial.StudyApplied {
		t.Fatalf("expected applied study, got %s", applied.Status)
	}
	got := runtime.ProductionParameters()
	if got.Speed != 9.5 || got.Profile.Name != "exp-hot" {
		t.Fatalf("expected applied parameters in production, got %+v", got)
	}
	stored, ok := runtime.TrialStudy(study.ID)
	if !ok {
		t.Fatal("expected study to be retained")
	}
	if len(stored.Groups) != 2 {
		t.Fatalf("expected both groups to be retained, got %d", len(stored.Groups))
	}
	for _, group := range stored.Groups {
		if group.ID == experiment.ID && group.Status != trial.GroupAdopted {
			t.Fatalf("expected adopted experiment, got %s", group.Status)
		}
		if group.ID == baseline.ID {
			if group.Status != trial.GroupNotAdopted {
				t.Fatalf("expected not adopted baseline, got %s", group.Status)
			}
			if !group.ResultRecorded || group.Result.WasteSheets != 20 {
				t.Fatalf("unadopted baseline data was not retained: %+v", group)
			}
		}
	}
}

func TestApplyTrialStudyRejectsMovedEdition(t *testing.T) {
	runtime := newTestRuntime(t)
	study, _ := approveStudy(t, runtime)
	current, ok := runtime.jobs.Current()
	if !ok {
		t.Fatal("expected an active job")
	}
	current.PlateGeneration++
	if err := runtime.jobs.Put(current); err != nil {
		t.Fatalf("bump plate generation: %v", err)
	}
	if _, err := runtime.ApplyTrialStudy(context.Background(), study.ID); err == nil || !strings.Contains(err.Error(), "moved past") {
		t.Fatalf("expected edition guard to reject apply, got %v", err)
	}
	if got := runtime.ProductionParameters(); got.Speed != 8 {
		t.Fatalf("rejected apply must not change production, got %+v", got)
	}
}

func TestOperationsUseAppliedProductionParameters(t *testing.T) {
	runtime := newTestRuntime(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	study, _ := approveStudy(t, runtime)
	if _, err := runtime.ApplyTrialStudy(ctx, study.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := runtime.CreateOperation(ctx, "load_roll"); err != nil {
		t.Fatalf("load roll: %v", err)
	}
	if _, err := runtime.CreateOperation(ctx, "feed_web"); err != nil {
		t.Fatalf("feed web: %v", err)
	}
	if _, _, _, feedSpeed := runtime.unwind.Snapshot(); feedSpeed != 9.5 {
		t.Fatalf("expected feed to run at the applied speed, got %v", feedSpeed)
	}
	if _, err := runtime.CreateOperation(ctx, "start_dryer"); err != nil {
		t.Fatalf("start dryer: %v", err)
	}
	if _, _, target, _, _, _ := runtime.dryer.Snapshot(); target != 120 {
		t.Fatalf("expected dryer to ramp toward the applied profile, got target %v", target)
	}
}
