package trial

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/wyw14/solo-038/internal/model"
)

// Document is the durable form of the trial store: every study (including
// unadopted groups) plus the parameters production currently runs with.
type Document struct {
	Version    uint64     `json:"version"`
	Studies    []Study    `json:"studies"`
	Production Parameters `json:"production"`
}

type Store struct {
	mu         sync.RWMutex
	path       string
	studies    map[string]Study
	production Parameters
}

func NewStore(path string, defaultProduction Parameters) *Store {
	return &Store{path: path, studies: map[string]Study{}, production: defaultProduction}
}

func (store *Store) Load() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	if document.Version == 0 {
		return fmt.Errorf("trial document version is invalid")
	}
	if err := document.Production.Validate(); err != nil {
		return fmt.Errorf("trial document production parameters: %w", err)
	}
	studies := map[string]Study{}
	for _, study := range document.Studies {
		studies[study.ID] = study
	}
	store.studies = studies
	store.production = document.Production
	return nil
}

func (store *Store) Production() Parameters {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.production
}

func (store *Store) Studies() []Study {
	store.mu.RLock()
	defer store.mu.RUnlock()
	values := make([]Study, 0, len(store.studies))
	for _, study := range store.studies {
		values = append(values, study)
	}
	sort.Slice(values, func(left, right int) bool {
		if !values[left].CreatedAt.Equal(values[right].CreatedAt) {
			return values[left].CreatedAt.Before(values[right].CreatedAt)
		}
		return values[left].ID < values[right].ID
	})
	return values
}

func (store *Store) Study(id string) (Study, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	study, ok := store.studies[id]
	return study, ok
}

// CreateStudy anchors a comparison to the job's current edition and seeds
// its baseline group from the parameters production is running with.
func (store *Store) CreateStudy(job model.Job, criteria Criteria, now time.Time) (Study, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, study := range store.studies {
		if study.JobID == job.ID && study.PlateGeneration == job.PlateGeneration {
			return Study{}, fmt.Errorf("a trial study already exists for job %s plate generation %d", job.ID, job.PlateGeneration)
		}
	}
	study, err := NewStudy(job, criteria, store.production, now)
	if err != nil {
		return Study{}, err
	}
	if err := store.commitLocked(study, store.production); err != nil {
		return Study{}, err
	}
	return study, nil
}

func (store *Store) AddGroup(studyID, label string, parameters Parameters, now time.Time) (Group, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	study, ok := store.studies[studyID]
	if !ok {
		return Group{}, fmt.Errorf("trial study %q was not found", studyID)
	}
	updated := mutableCopy(study)
	group, err := updated.AddGroup(label, parameters, now)
	if err != nil {
		return Group{}, err
	}
	if err := store.commitLocked(updated, store.production); err != nil {
		return Group{}, err
	}
	return group, nil
}

func (store *Store) RecordResult(studyID, groupID string, result Result, now time.Time) (Study, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	study, ok := store.studies[studyID]
	if !ok {
		return Study{}, fmt.Errorf("trial study %q was not found", studyID)
	}
	updated := mutableCopy(study)
	if err := updated.RecordResult(groupID, result, now); err != nil {
		return Study{}, err
	}
	if err := store.commitLocked(updated, store.production); err != nil {
		return Study{}, err
	}
	return updated, nil
}

func (store *Store) Approve(studyID, groupID string, now time.Time) (Study, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	study, ok := store.studies[studyID]
	if !ok {
		return Study{}, fmt.Errorf("trial study %q was not found", studyID)
	}
	updated := mutableCopy(study)
	if err := updated.Approve(groupID, now); err != nil {
		return Study{}, err
	}
	if err := store.commitLocked(updated, store.production); err != nil {
		return Study{}, err
	}
	return updated, nil
}

// Apply is the single place where a trial outcome reaches production: the
// approved group's parameters become the production parameters and the study
// is sealed with every unadopted group retained.
func (store *Store) Apply(studyID string, now time.Time) (Study, Group, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	study, ok := store.studies[studyID]
	if !ok {
		return Study{}, Group{}, fmt.Errorf("trial study %q was not found", studyID)
	}
	updated := mutableCopy(study)
	adopted, err := updated.MarkApplied(now)
	if err != nil {
		return Study{}, Group{}, err
	}
	if err := store.commitLocked(updated, adopted.Parameters); err != nil {
		return Study{}, Group{}, err
	}
	return updated, adopted, nil
}

// mutableCopy gives a study its own groups backing array so transition
// methods never mutate the stored study before the candidate state is durable.
func mutableCopy(study Study) Study {
	study.Groups = append([]Group(nil), study.Groups...)
	return study
}

// commitLocked persists the candidate state first and swaps it into the
// store only when the write is durable, so a failed write never leaves the
// in-memory state ahead of the disk.
func (store *Store) commitLocked(study Study, production Parameters) error {
	candidate := maps.Clone(store.studies)
	candidate[study.ID] = study
	if err := persistDocument(store.path, candidate, production); err != nil {
		return err
	}
	store.studies = candidate
	store.production = production
	return nil
}

func persistDocument(path string, studies map[string]Study, production Parameters) error {
	ordered := make([]Study, 0, len(studies))
	for _, study := range studies {
		ordered = append(ordered, study)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].ID < ordered[right].ID })
	document := Document{Version: uint64(time.Now().UnixNano()), Studies: ordered, Production: production}
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".next"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	file, err := os.Open(temporary)
	if err != nil {
		return err
	}
	if err = file.Sync(); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
