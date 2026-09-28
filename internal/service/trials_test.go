package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wyw14/solo-038/internal/trial"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime, err := NewRuntime(Config{DataDir: t.TempDir(), FestoonCapacity: 120, RegisterGap: 500 * time.Millisecond, RegisterTolerance: 0.25})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	return runtime
}

func recordInput(speed, deviation float64, waste int) RecordResultInput {
	return RecordResultInput{
		Speed:               speed,
		Profile:             trial.DefaultProfile(),
		RegisterDeviationMM: []float64{deviation, -deviation / 2},
		WasteSheets:         waste,
	}
}

func TestTrialServiceFlowKeepsProductionUntilApproval(t *testing.T) {
	runtime := newTestRuntime(t)

	if _, configured := runtime.ProductionConfig(); configured {
		t.Fatalf("no formal production config should exist before approval")
	}

	created, err := runtime.CreateTrial(CreateTrialInput{Name: "plate-1"})
	if err != nil {
		t.Fatalf("create trial: %v", err)
	}
	baseline := created.Groups[0]
	if baseline.Kind != trial.GroupBaseline || baseline.Settings.Speed != 8 || baseline.Settings.Profile.Name != "production" {
		t.Fatalf("baseline must snapshot formal defaults: %+v", baseline)
	}
	if _, err := runtime.CreateTrial(CreateTrialInput{Name: "duplicate"}); !errors.Is(err, trial.ErrTrialOpen) {
		t.Fatalf("expected ErrTrialOpen, got %v", err)
	}

	if _, err := runtime.RecordTrialResult(created.ID, baseline.ID, recordInput(8, 0.05, 140)); err != nil {
		t.Fatalf("record baseline: %v", err)
	}
	_, experiment, err := runtime.AddTrialGroup(created.ID, AddGroupInput{Name: "hotter", Settings: trial.Settings{Speed: 9.5, Profile: trial.DefaultProfile()}})
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	if _, err := runtime.RecordTrialResult(created.ID, experiment.ID, recordInput(9.5, 0.03, 90)); err != nil {
		t.Fatalf("record experiment: %v", err)
	}

	if _, configured := runtime.ProductionConfig(); configured {
		t.Fatalf("trial runs must not change formal production config")
	}

	report, err := runtime.TrialComparison(created.ID)
	if err != nil {
		t.Fatalf("comparison: %v", err)
	}
	if report.RecommendedGroup != experiment.ID {
		t.Fatalf("experiment has less waste and better register, should be recommended: %+v", report.Groups)
	}

	approved, settings, err := runtime.ApproveTrial(created.ID, experiment.ID, "shift-lead")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != trial.TrialApproved || settings.Settings.Speed != 9.5 {
		t.Fatalf("approval did not adopt experiment settings: %+v", settings)
	}
	formal, configured := runtime.ProductionConfig()
	if !configured || formal.Settings.Speed != 9.5 || formal.GroupID != experiment.ID {
		t.Fatalf("formal config must equal approved group: %+v", formal)
	}
	for _, group := range approved.Groups {
		if group.ID == baseline.ID && (group.Status != trial.GroupNotAdopted || len(group.Results) != 1) {
			t.Fatalf("unadopted baseline data must be retained: %+v", group)
		}
	}
	if len(runtime.ListTrials()) != 1 {
		t.Fatalf("decided trial must remain listed")
	}
}
