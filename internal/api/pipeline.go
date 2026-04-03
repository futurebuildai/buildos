package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// PipelineHandler handles /api/v1/org/{orgID}/pipeline/* endpoints.
type PipelineHandler struct {
	svc *service.PipelineService
}

// NewPipelineHandler creates a handler with service dependencies.
func NewPipelineHandler(svc *service.PipelineService) *PipelineHandler {
	return &PipelineHandler{svc: svc}
}

// ListProspects returns all pipeline prospects for an org.
// GET /api/v1/org/{orgID}/pipeline/prospects
func (h *PipelineHandler) ListProspects(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseOrgID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG_ID", err.Error())
		return
	}

	prospects, err := h.svc.ListProspects(r.Context(), orgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list prospects")
		return
	}

	writeJSON(w, r, http.StatusOK, prospects)
}

// createProspectRequest is the JSON body for CreateProspect.
type createProspectRequest struct {
	Name        string  `json:"name"`
	ClientName  string  `json:"client_name"`
	ClientEmail *string `json:"client_email,omitempty"`
	ClientPhone *string `json:"client_phone,omitempty"`
	Address     *string `json:"address,omitempty"`
	GSF         *int    `json:"gsf,omitempty"`
	Source      *string `json:"source,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

// CreateProspect adds a new prospect to the pipeline.
// POST /api/v1/org/{orgID}/pipeline/prospects
func (h *PipelineHandler) CreateProspect(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseOrgID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG_ID", err.Error())
		return
	}

	var req createProspectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	prospect := &models.Prospect{
		OrgID:       orgID,
		Name:        req.Name,
		ClientName:  req.ClientName,
		ClientEmail: req.ClientEmail,
		ClientPhone: req.ClientPhone,
		Address:     req.Address,
		GSF:         req.GSF,
		Source:      req.Source,
		Notes:       req.Notes,
	}

	id, err := h.svc.CreateProspect(r.Context(), prospect)
	if err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id})
}

// GetProspect returns a prospect with its estimates and permits.
// GET /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
func (h *PipelineHandler) GetProspect(w http.ResponseWriter, r *http.Request) {
	prospectID, err := uuid.Parse(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROSPECT_ID", "invalid prospect ID")
		return
	}

	detail, err := h.svc.GetProspectDetail(r.Context(), prospectID)
	if err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, detail)
}

// updateProspectRequest is the JSON body for UpdateProspect.
type updateProspectRequest struct {
	Name        string  `json:"name"`
	ClientName  string  `json:"client_name"`
	ClientEmail *string `json:"client_email,omitempty"`
	ClientPhone *string `json:"client_phone,omitempty"`
	Address     *string `json:"address,omitempty"`
	GSF         *int    `json:"gsf,omitempty"`
	Source      *string `json:"source,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

// UpdateProspect modifies prospect details.
// PUT /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
func (h *PipelineHandler) UpdateProspect(w http.ResponseWriter, r *http.Request) {
	prospectID, err := uuid.Parse(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROSPECT_ID", "invalid prospect ID")
		return
	}

	var req updateProspectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	prospect := &models.Prospect{
		ID:          prospectID,
		Name:        req.Name,
		ClientName:  req.ClientName,
		ClientEmail: req.ClientEmail,
		ClientPhone: req.ClientPhone,
		Address:     req.Address,
		GSF:         req.GSF,
		Source:      req.Source,
		Notes:       req.Notes,
	}

	if err := h.svc.UpdateProspect(r.Context(), prospect); err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "updated"})
}

// AdvanceProspect transitions a prospect to the next pipeline stage.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/advance
func (h *PipelineHandler) AdvanceProspect(w http.ResponseWriter, r *http.Request) {
	prospectID, err := uuid.Parse(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROSPECT_ID", "invalid prospect ID")
		return
	}

	prospect, err := h.svc.AdvanceProspect(r.Context(), prospectID)
	if err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, prospect)
}

// loseProspectRequest is the JSON body for LoseProspect.
type loseProspectRequest struct {
	Reason string `json:"reason"`
}

// LoseProspect marks a prospect as lost.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/lose
func (h *PipelineHandler) LoseProspect(w http.ResponseWriter, r *http.Request) {
	prospectID, err := uuid.Parse(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROSPECT_ID", "invalid prospect ID")
		return
	}

	var req loseProspectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	if err := h.svc.LoseProspect(r.Context(), prospectID, req.Reason); err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "lost"})
}

// createEstimateRequest is the JSON body for CreateEstimate.
type createEstimateRequest struct {
	TotalEstimatedCents int64           `json:"total_estimated_cents"`
	CurrencyCode        string          `json:"currency_code"`
	LineItems           json.RawMessage `json:"line_items,omitempty"`
	MarginPct           int             `json:"margin_pct,omitempty"`
	Status              string          `json:"status,omitempty"`
}

// CreateEstimate adds a new estimate for a prospect.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/estimates
func (h *PipelineHandler) CreateEstimate(w http.ResponseWriter, r *http.Request) {
	prospectID, err := uuid.Parse(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROSPECT_ID", "invalid prospect ID")
		return
	}

	var req createEstimateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	estimate := &models.PipelineEstimate{
		ProspectID:          prospectID,
		TotalEstimatedCents: req.TotalEstimatedCents,
		CurrencyCode:        req.CurrencyCode,
		LineItems:           req.LineItems,
		MarginPct:           req.MarginPct,
		Status:              req.Status,
	}

	id, err := h.svc.CreateEstimate(r.Context(), estimate)
	if err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id})
}

// updateEstimateRequest is the JSON body for UpdateEstimate.
type updateEstimateRequest struct {
	TotalEstimatedCents int64           `json:"total_estimated_cents"`
	CurrencyCode        string          `json:"currency_code"`
	LineItems           json.RawMessage `json:"line_items,omitempty"`
	MarginPct           int             `json:"margin_pct"`
	Status              string          `json:"status"`
}

// UpdateEstimate modifies an existing estimate.
// PUT /api/v1/org/{orgID}/pipeline/estimates/{estimateID}
func (h *PipelineHandler) UpdateEstimate(w http.ResponseWriter, r *http.Request) {
	estimateID, err := uuid.Parse(chi.URLParam(r, "estimateID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ESTIMATE_ID", "invalid estimate ID")
		return
	}

	var req updateEstimateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	var sentAt *time.Time
	if req.Status == models.EstimateStatusSent {
		now := time.Now().UTC()
		sentAt = &now
	}

	estimate := &models.PipelineEstimate{
		ID:                  estimateID,
		TotalEstimatedCents: req.TotalEstimatedCents,
		CurrencyCode:        req.CurrencyCode,
		LineItems:           req.LineItems,
		MarginPct:           req.MarginPct,
		Status:              req.Status,
		SentAt:              sentAt,
	}

	if err := h.svc.UpdateEstimate(r.Context(), estimate); err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "updated"})
}

// createPermitRequest is the JSON body for CreatePermit.
type createPermitRequest struct {
	PermitType        string  `json:"permit_type"`
	Jurisdiction      string  `json:"jurisdiction"`
	ApplicationNumber *string `json:"application_number,omitempty"`
	SubmittedDate     *string `json:"submitted_date,omitempty"`
	ExpectedIssueDate *string `json:"expected_issue_date,omitempty"`
	FeeCents          int64   `json:"fee_cents"`
	FeeCurrencyCode   string  `json:"fee_currency_code"`
	Status            string  `json:"status,omitempty"`
	Notes             *string `json:"notes,omitempty"`
}

// CreatePermit adds a permit for a prospect.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/permits
func (h *PipelineHandler) CreatePermit(w http.ResponseWriter, r *http.Request) {
	prospectID, err := uuid.Parse(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROSPECT_ID", "invalid prospect ID")
		return
	}

	_ = mw.MustClaimsFromContext(r.Context()) // ensure auth

	var req createPermitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	permit := &models.Permit{
		ProspectID:        prospectID,
		PermitType:        req.PermitType,
		Jurisdiction:      req.Jurisdiction,
		ApplicationNumber: req.ApplicationNumber,
		FeeCents:          req.FeeCents,
		FeeCurrencyCode:   req.FeeCurrencyCode,
		Status:            req.Status,
		Notes:             req.Notes,
	}

	if req.SubmittedDate != nil {
		t, err := time.Parse("2006-01-02", *req.SubmittedDate)
		if err == nil {
			permit.SubmittedDate = &t
		}
	}
	if req.ExpectedIssueDate != nil {
		t, err := time.Parse("2006-01-02", *req.ExpectedIssueDate)
		if err == nil {
			permit.ExpectedIssueDate = &t
		}
	}

	id, err := h.svc.CreatePermit(r.Context(), permit)
	if err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id})
}

// updatePermitRequest is the JSON body for UpdatePermit.
type updatePermitRequest struct {
	PermitType        string  `json:"permit_type"`
	Jurisdiction      string  `json:"jurisdiction"`
	ApplicationNumber *string `json:"application_number,omitempty"`
	SubmittedDate     *string `json:"submitted_date,omitempty"`
	ExpectedIssueDate *string `json:"expected_issue_date,omitempty"`
	ActualIssueDate   *string `json:"actual_issue_date,omitempty"`
	FeeCents          int64   `json:"fee_cents"`
	FeeCurrencyCode   string  `json:"fee_currency_code"`
	Status            string  `json:"status"`
	Notes             *string `json:"notes,omitempty"`
}

// UpdatePermit modifies an existing permit.
// PUT /api/v1/org/{orgID}/pipeline/permits/{permitID}
func (h *PipelineHandler) UpdatePermit(w http.ResponseWriter, r *http.Request) {
	permitID, err := uuid.Parse(chi.URLParam(r, "permitID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PERMIT_ID", "invalid permit ID")
		return
	}

	var req updatePermitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	permit := &models.Permit{
		ID:                permitID,
		PermitType:        req.PermitType,
		Jurisdiction:      req.Jurisdiction,
		ApplicationNumber: req.ApplicationNumber,
		FeeCents:          req.FeeCents,
		FeeCurrencyCode:   req.FeeCurrencyCode,
		Status:            req.Status,
		Notes:             req.Notes,
	}

	if req.SubmittedDate != nil {
		t, err := time.Parse("2006-01-02", *req.SubmittedDate)
		if err == nil {
			permit.SubmittedDate = &t
		}
	}
	if req.ExpectedIssueDate != nil {
		t, err := time.Parse("2006-01-02", *req.ExpectedIssueDate)
		if err == nil {
			permit.ExpectedIssueDate = &t
		}
	}
	if req.ActualIssueDate != nil {
		t, err := time.Parse("2006-01-02", *req.ActualIssueDate)
		if err == nil {
			permit.ActualIssueDate = &t
		}
	}

	if err := h.svc.UpdatePermit(r.Context(), permit); err != nil {
		handlePipelineError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "updated"})
}

// Analytics returns weighted pipeline revenue grouped by currency.
// GET /api/v1/org/{orgID}/pipeline/analytics
func (h *PipelineHandler) Analytics(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseOrgID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG_ID", err.Error())
		return
	}

	analytics, err := h.svc.Analytics(r.Context(), orgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute analytics")
		return
	}

	writeJSON(w, r, http.StatusOK, analytics)
}

// handlePipelineError maps pipeline service errors to HTTP responses.
func handlePipelineError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidTransition):
		writeErrorResponse(w, r, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, service.ErrProspectLost):
		writeErrorResponse(w, r, http.StatusConflict, "PROSPECT_LOST", err.Error())
	case errors.Is(err, service.ErrAlreadyIssued):
		writeErrorResponse(w, r, http.StatusConflict, "ALREADY_ISSUED", err.Error())
	case errors.Is(err, service.ErrMissingName):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrMissingClientName):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrInvalidCurrency):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "INVALID_CURRENCY", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
	}
}
