package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/model"
	"github.com/wyw14/solo-038/internal/service"
	"github.com/wyw14/solo-038/internal/trial"
)

type createTrialRequest struct {
	Name     string         `json:"name"`
	Criteria trial.Criteria `json:"criteria"`
}

type addGroupRequest struct {
	Kind     string         `json:"kind"`
	Name     string         `json:"name"`
	Settings trial.Settings `json:"settings"`
}

type recordResultRequest struct {
	Speed               float64       `json:"speed"`
	Profile             dryer.Profile `json:"profile"`
	RegisterDeviationMM []float64     `json:"register_deviation_mm"`
	WasteSheets         int           `json:"waste_sheets"`
}

type approveTrialRequest struct {
	GroupID    string `json:"group_id"`
	ApprovedBy string `json:"approved_by"`
}

func (handler *Handler) listTrials(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, handler.runtime.ListTrials())
}

func (handler *Handler) createTrial(writer http.ResponseWriter, request *http.Request) {
	var input createTrialRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	created, err := handler.runtime.CreateTrial(service.CreateTrialInput{Name: input.Name, Criteria: input.Criteria})
	if err != nil {
		writeTrialError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, created)
}

func (handler *Handler) getTrial(writer http.ResponseWriter, request *http.Request) {
	trialID, err := model.ParseTrialID(chi.URLParam(request, "trial"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	found, ok := handler.runtime.GetTrial(trialID)
	if !ok {
		writeTrialError(writer, trial.ErrTrialNotFound)
		return
	}
	writeJSON(writer, http.StatusOK, found)
}

func (handler *Handler) addTrialGroup(writer http.ResponseWriter, request *http.Request) {
	trialID, err := model.ParseTrialID(chi.URLParam(request, "trial"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	var input addGroupRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	_, group, err := handler.runtime.AddTrialGroup(trialID, service.AddGroupInput{Kind: input.Kind, Name: input.Name, Settings: input.Settings})
	if err != nil {
		writeTrialError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, group)
}

func (handler *Handler) recordTrialResult(writer http.ResponseWriter, request *http.Request) {
	trialID, err := model.ParseTrialID(chi.URLParam(request, "trial"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	groupID, err := model.ParseGroupID(chi.URLParam(request, "group"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	var input recordResultRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	updated, err := handler.runtime.RecordTrialResult(trialID, groupID, service.RecordResultInput{Speed: input.Speed, Profile: input.Profile, RegisterDeviationMM: input.RegisterDeviationMM, WasteSheets: input.WasteSheets})
	if err != nil {
		writeTrialError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, updated)
}

func (handler *Handler) trialComparison(writer http.ResponseWriter, request *http.Request) {
	trialID, err := model.ParseTrialID(chi.URLParam(request, "trial"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	report, err := handler.runtime.TrialComparison(trialID)
	if err != nil {
		writeTrialError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, report)
}

func (handler *Handler) approveTrial(writer http.ResponseWriter, request *http.Request) {
	trialID, err := model.ParseTrialID(chi.URLParam(request, "trial"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	var input approveTrialRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	groupID, err := model.ParseGroupID(input.GroupID)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	updated, settings, err := handler.runtime.ApproveTrial(trialID, groupID, input.ApprovedBy)
	if err != nil {
		writeTrialError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"trial": updated, "production_settings": settings})
}

func (handler *Handler) productionConfig(writer http.ResponseWriter, request *http.Request) {
	settings, ok := handler.runtime.ProductionConfig()
	if !ok {
		writeJSON(writer, http.StatusOK, map[string]any{"configured": false})
		return
	}
	writeJSON(writer, http.StatusOK, settings)
}

func writeTrialError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trial.ErrTrialNotFound), errors.Is(err, trial.ErrGroupNotFound):
		writeError(writer, http.StatusNotFound, err)
	case errors.Is(err, trial.ErrTrialOpen), errors.Is(err, trial.ErrTrialDecided),
		errors.Is(err, trial.ErrBaselineExists), errors.Is(err, trial.ErrGroupOpen),
		errors.Is(err, trial.ErrNoResults), errors.Is(err, trial.ErrNotEnoughGroups),
		errors.Is(err, trial.ErrGroupNotRecordable):
		writeError(writer, http.StatusConflict, err)
	default:
		writeError(writer, http.StatusBadRequest, err)
	}
}
