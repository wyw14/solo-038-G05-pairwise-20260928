package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/solo-038/internal/trial"
)

type createTrialStudyRequest struct {
	Criteria trial.Criteria `json:"criteria"`
}

type createTrialGroupRequest struct {
	Label      string           `json:"label"`
	Parameters trial.Parameters `json:"parameters"`
}

type recordTrialResultRequest struct {
	RegisterOffsets []float64 `json:"register_offsets"`
	WasteSheets     int       `json:"waste_sheets"`
}

type approveTrialStudyRequest struct {
	GroupID string `json:"group_id"`
}

func (handler *Handler) listTrialStudies(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, handler.runtime.TrialStudies())
}

func (handler *Handler) createTrialStudy(writer http.ResponseWriter, request *http.Request) {
	var input createTrialStudyRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	study, err := handler.runtime.CreateTrialStudy(request.Context(), input.Criteria)
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeJSON(writer, http.StatusCreated, study)
}

func (handler *Handler) getTrialStudy(writer http.ResponseWriter, request *http.Request) {
	study, ok := handler.runtime.TrialStudy(chi.URLParam(request, "studyID"))
	if !ok {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trial study was not found"})
		return
	}
	writeJSON(writer, http.StatusOK, study)
}

func (handler *Handler) createTrialGroup(writer http.ResponseWriter, request *http.Request) {
	studyID := chi.URLParam(request, "studyID")
	if _, ok := handler.runtime.TrialStudy(studyID); !ok {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trial study was not found"})
		return
	}
	var input createTrialGroupRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	group, err := handler.runtime.AddTrialGroup(studyID, input.Label, input.Parameters)
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeJSON(writer, http.StatusCreated, group)
}

func (handler *Handler) recordTrialResult(writer http.ResponseWriter, request *http.Request) {
	studyID := chi.URLParam(request, "studyID")
	study, ok := handler.runtime.TrialStudy(studyID)
	if !ok {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trial study was not found"})
		return
	}
	groupID := chi.URLParam(request, "groupID")
	if _, ok := study.Group(groupID); !ok {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trial group was not found"})
		return
	}
	var input recordTrialResultRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	updated, err := handler.runtime.RecordTrialResult(studyID, groupID, trial.Result{RegisterOffsets: input.RegisterOffsets, WasteSheets: input.WasteSheets})
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeJSON(writer, http.StatusOK, updated)
}

func (handler *Handler) evaluateTrialStudy(writer http.ResponseWriter, request *http.Request) {
	comparison, err := handler.runtime.EvaluateTrialStudy(chi.URLParam(request, "studyID"))
	if err != nil {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trial study was not found"})
		return
	}
	writeJSON(writer, http.StatusOK, comparison)
}

func (handler *Handler) approveTrialStudy(writer http.ResponseWriter, request *http.Request) {
	studyID := chi.URLParam(request, "studyID")
	if _, ok := handler.runtime.TrialStudy(studyID); !ok {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trial study was not found"})
		return
	}
	var input approveTrialStudyRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	study, err := handler.runtime.ApproveTrialGroup(studyID, input.GroupID)
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeJSON(writer, http.StatusOK, study)
}

func (handler *Handler) applyTrialStudy(writer http.ResponseWriter, request *http.Request) {
	studyID := chi.URLParam(request, "studyID")
	if _, ok := handler.runtime.TrialStudy(studyID); !ok {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "trial study was not found"})
		return
	}
	study, err := handler.runtime.ApplyTrialStudy(request.Context(), studyID)
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeJSON(writer, http.StatusOK, study)
}

func (handler *Handler) productionParameters(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, handler.runtime.ProductionParameters())
}
