package trial

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLifecycleKeepsProductionUntilApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trials.json")
	production := testParameters(8)
	store := NewStore(path, production)

	job := testJob()
	study, err := store.CreateStudy(job, testCriteria(), testNow)
	if err != nil {
		t.Fatalf("create study: %v", err)
	}
	if len(study.Groups) != 1 || study.Groups[0].Parameters.Speed != 8 {
		t.Fatalf("baseline should snapshot production, got %+v", study.Groups[0].Parameters)
	}
	if _, err := store.CreateStudy(job, testCriteria(), testNow); err == nil {
		t.Fatal("expected a second study for the same edition to be rejected")
	}
	experiment, err := store.AddGroup(study.ID, "faster", testParameters(9.5), testNow)
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	baseline := study.Groups[0]
	if _, err := store.RecordResult(study.ID, baseline.ID, testResult(25, 0.1, 0.1), testNow); err != nil {
		t.Fatalf("record baseline result: %v", err)
	}
	if _, err := store.RecordResult(study.ID, experiment.ID, testResult(10, 0.05, -0.05), testNow); err != nil {
		t.Fatalf("record experiment result: %v", err)
	}
	if _, err := store.Approve(study.ID, experiment.ID, testNow); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got := store.Production(); got.Speed != production.Speed {
		t.Fatalf("trial activity must not change production, got speed %v", got.Speed)
	}
	applied, adopted, err := store.Apply(study.ID, testNow)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Status != StudyApplied {
		t.Fatalf("expected applied study, got %s", applied.Status)
	}
	if got := store.Production(); got.Speed != 9.5 {
		t.Fatalf("expected applied speed 9.5, got %v", got.Speed)
	}
	if adopted.Parameters.Profile.Name != "test" {
		t.Fatalf("expected adopted profile to reach production, got %s", adopted.Parameters.Profile.Name)
	}

	reloaded := NewStore(path, production)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Production(); got.Speed != 9.5 {
		t.Fatalf("expected production to survive reload, got %v", got.Speed)
	}
	restored, ok := reloaded.Study(study.ID)
	if !ok {
		t.Fatal("expected study to survive reload")
	}
	if restored.Status != StudyApplied || len(restored.Groups) != 2 {
		t.Fatalf("unexpected restored study: %+v", restored)
	}
	var retained *Group
	for index := range restored.Groups {
		if restored.Groups[index].ID == baseline.ID {
			retained = &restored.Groups[index]
		}
	}
	if retained == nil || retained.Status != GroupNotAdopted || !retained.ResultRecorded || retained.Result.WasteSheets != 25 {
		t.Fatalf("unadopted baseline data was not retained: %+v", retained)
	}
}

func TestStoreLoadWithMissingFileKeepsDefaults(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "trials.json"), testParameters(8))
	if err := store.Load(); err != nil {
		t.Fatalf("load with missing file: %v", err)
	}
	if got := store.Production(); got.Speed != 8 {
		t.Fatalf("expected default production, got %v", got.Speed)
	}
	if len(store.Studies()) != 0 {
		t.Fatal("expected no studies")
	}
}

func TestStoreLoadRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trials.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	if err := NewStore(path, testParameters(8)).Load(); err == nil {
		t.Fatal("expected corrupt file to be rejected")
	}
	if err := os.WriteFile(path, []byte(`{"version":0,"studies":[],"production":{}}`), 0o600); err != nil {
		t.Fatalf("write invalid document: %v", err)
	}
	if err := NewStore(path, testParameters(8)).Load(); err == nil {
		t.Fatal("expected invalid document to be rejected")
	}
}

func TestStoreRejectsUnknownStudy(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "trials.json"), testParameters(8))
	if _, err := store.AddGroup("missing", "label", testParameters(9), testNow); err == nil {
		t.Fatal("expected add group on an unknown study to fail")
	}
	if _, err := store.RecordResult("missing", "group", testResult(1, 0.1), testNow); err == nil {
		t.Fatal("expected record result on an unknown study to fail")
	}
	if _, err := store.Approve("missing", "group", testNow); err == nil {
		t.Fatal("expected approve on an unknown study to fail")
	}
	if _, _, err := store.Apply("missing", testNow); err == nil {
		t.Fatal("expected apply on an unknown study to fail")
	}
}
