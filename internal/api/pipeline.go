package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
)

// PipelineServicer is the subset of *service.PipelineService consumed by
// PipelineHandler.
type PipelineServicer interface {
	ListProspects(ctx context.Context, in service.ListProspectsInput) (store.ProspectsPage, error)
	GetProspectWithDetails(ctx context.Context, prospectID, callerOrgID uuid.UUID) (models.ProspectWithDetails, error)
	CreateProspect(ctx context.Context, in service.CreateProspectInput) (models.Prospect, error)
	UpdateProspect(ctx context.Context, in service.UpdateProspectInput) (models.Prospect, error)
	AdvanceProspect(ctx context.Context, callerUserSub string, in service.AdvanceProspectInput) (models.Prospect, error)
	LoseProspect(ctx context.Context, callerUserSub string, in service.LoseProspectInput) (models.Prospect, error)
	CreateEstimate(ctx context.Context, in service.CreateEstimateInput) (models.PipelineEstimate, error)
	UpdateEstimateStatus(ctx context.Context, in service.UpdateEstimateStatusInput) (models.PipelineEstimate, error)
	CreatePermit(ctx context.Context, in service.CreatePermitInput) (models.Permit, error)
	UpdatePermit(ctx context.Context, in service.UpdatePermitInput) (models.Permit, error)
	GetPipelineAnalytics(ctx context.Context, orgID uuid.UUID) ([]models.PipelineAnalyticsRow, error)
}

// PipelineHandler handles /api/v1/org/{orgID}/pipeline/* endpoints.
type PipelineHandler struct {
	svc PipelineServicer
}

// NewPipelineHandler creates a handler bound to the given service.
func NewPipelineHandler(svc PipelineServicer) *PipelineHandler {
	return &PipelineHandler{svc: svc}
}

// ---------- Prospect reads ----------

// ListProspects returns a paginated page of prospects for an org.
// GET /api/v1/org/{orgID}/pipeline/prospects[?stage=LEAD&page=1&per_page=50]
func (h *PipelineHandler) ListProspects(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	page, perPage := parsePagination(r)

	result, err := h.svc.ListProspects(r.Context(), service.ListProspectsInput{
		OrgID:   orgID,
		Stage:   r.URL.Query().Get("stage"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSONWithPagination(w, r, http.StatusOK,
		map[string]any{"prospects": result.Prospects},
		paginationMeta{Page: result.Page, PerPage: result.PerPage, Total: result.Total, TotalPages: result.TotalPages})
}

// GetProspect returns a prospect with its estimates and permits.
// GET /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
func (h *PipelineHandler) GetProspect(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	prospectID, ok := parseUUIDFromURL(w, r, "prospectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetProspectWithDetails(r.Context(), prospectID, callerOrg)
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, out)
}

// ---------- Prospect writes ----------

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

// CreateProspect adds a new prospect at stage LEAD.
// POST /api/v1/org/{orgID}/pipeline/prospects
func (h *PipelineHandler) CreateProspect(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	var body createProspectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	prospect, err := h.svc.CreateProspect(r.Context(), service.CreateProspectInput{
		OrgID:       orgID,
		Name:        body.Name,
		ClientName:  body.ClientName,
		ClientEmail: body.ClientEmail,
		ClientPhone: body.ClientPhone,
		Address:     body.Address,
		GSF:         body.GSF,
		Source:      body.Source,
		Notes:       body.Notes,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"prospect": prospect})
}

type updateProspectRequest struct {
	Name        *string `json:"name,omitempty"`
	ClientName  *string `json:"client_name,omitempty"`
	ClientEmail *string `json:"client_email,omitempty"`
	ClientPhone *string `json:"client_phone,omitempty"`
	Address     *string `json:"address,omitempty"`
	GSF         *int    `json:"gsf,omitempty"`
	Source      *string `json:"source,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

// UpdateProspect modifies prospect details (no stage transition here).
// PUT /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
func (h *PipelineHandler) UpdateProspect(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	prospectID, ok := parseUUIDFromURL(w, r, "prospectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body updateProspectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	prospect, err := h.svc.UpdateProspect(r.Context(), service.UpdateProspectInput{
		ProspectID:  prospectID,
		OrgID:       callerOrg,
		Name:        body.Name,
		ClientName:  body.ClientName,
		ClientEmail: body.ClientEmail,
		ClientPhone: body.ClientPhone,
		Address:     body.Address,
		GSF:         body.GSF,
		Source:      body.Source,
		Notes:       body.Notes,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"prospect": prospect})
}

type advanceProspectRequest struct {
	TargetStage      string  `json:"target_stage"`
	PermitIssuedDate *string `json:"permit_issued_date,omitempty"`
}

// AdvanceProspect transitions a prospect to the next pipeline stage.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/advance
func (h *PipelineHandler) AdvanceProspect(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	prospectID, ok := parseUUIDFromURL(w, r, "prospectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body advanceProspectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if body.TargetStage == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "target_stage is required")
		return
	}
	permitDate, err := parseOptionalDate(body.PermitIssuedDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "permit_issued_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	prospect, err := h.svc.AdvanceProspect(r.Context(), claims.Sub, service.AdvanceProspectInput{
		ProspectID:       prospectID,
		OrgID:            callerOrg,
		Target:           models.PipelineStage(body.TargetStage),
		PermitIssuedDate: permitDate,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"prospect": prospect})
}

type loseProspectRequest struct {
	Reason string `json:"reason"`
}

// LoseProspect marks a prospect as lost.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/lose
func (h *PipelineHandler) LoseProspect(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	prospectID, ok := parseUUIDFromURL(w, r, "prospectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body loseProspectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	prospect, err := h.svc.LoseProspect(r.Context(), claims.Sub, service.LoseProspectInput{
		ProspectID: prospectID,
		OrgID:      callerOrg,
		Reason:     body.Reason,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"prospect": prospect})
}

// ---------- Estimates ----------

type createEstimateRequest struct {
	LineItems    models.PipelineEstimateLineItems `json:"line_items"`
	MarginPct    int                              `json:"margin_pct"`
	CurrencyCode string                           `json:"currency_code"`
}

// CreateEstimate adds a new estimate version for a prospect.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/estimates
func (h *PipelineHandler) CreateEstimate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	prospectID, ok := parseUUIDFromURL(w, r, "prospectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body createEstimateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	estimate, err := h.svc.CreateEstimate(r.Context(), service.CreateEstimateInput{
		ProspectID:   prospectID,
		OrgID:        callerOrg,
		CurrencyCode: body.CurrencyCode,
		LineItems:    body.LineItems,
		MarginPct:    body.MarginPct,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"estimate": estimate})
}

// UpdateEstimate modifies an existing estimate's status. Estimates are
// versioned; line-item edits create a new estimate via CreateEstimate.
// PUT /api/v1/org/{orgID}/pipeline/estimates/{estimateID}
//
// The route is org-scoped without a {prospectID} path segment per
// API_CONTRACT, so the body MUST carry prospect_id for the service to
// verify the (estimate, prospect, org) chain.
func (h *PipelineHandler) UpdateEstimate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	estimateID, ok := parseUUIDFromURL(w, r, "estimateID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body struct {
		ProspectID string  `json:"prospect_id"`
		Status     *string `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if body.Status == nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "status is required")
		return
	}
	prospectID, err := uuid.Parse(body.ProspectID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "prospect_id is required and must be a UUID")
		return
	}
	estimate, err := h.svc.UpdateEstimateStatus(r.Context(), service.UpdateEstimateStatusInput{
		EstimateID: estimateID,
		ProspectID: prospectID,
		OrgID:      callerOrg,
		NewStatus:  *body.Status,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"estimate": estimate})
}

// ---------- Permits ----------

type createPermitRequest struct {
	PermitType        string  `json:"permit_type"`
	Jurisdiction      string  `json:"jurisdiction"`
	ApplicationNumber *string `json:"application_number,omitempty"`
	SubmittedDate     *string `json:"submitted_date,omitempty"`
	ExpectedIssueDate *string `json:"expected_issue_date,omitempty"`
	FeeCents          int64   `json:"fee_cents"`
	FeeCurrencyCode   string  `json:"fee_currency_code"`
	Notes             *string `json:"notes,omitempty"`
}

// CreatePermit adds a permit for a prospect.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/permits
func (h *PipelineHandler) CreatePermit(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	prospectID, ok := parseUUIDFromURL(w, r, "prospectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body createPermitRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	submittedDate, err := parseOptionalDate(body.SubmittedDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "submitted_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	expectedIssue, err := parseOptionalDate(body.ExpectedIssueDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "expected_issue_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	permit, err := h.svc.CreatePermit(r.Context(), service.CreatePermitInput{
		ProspectID:        prospectID,
		OrgID:             callerOrg,
		PermitType:        body.PermitType,
		Jurisdiction:      body.Jurisdiction,
		ApplicationNumber: body.ApplicationNumber,
		SubmittedDate:     submittedDate,
		ExpectedIssueDate: expectedIssue,
		FeeCents:          body.FeeCents,
		FeeCurrencyCode:   body.FeeCurrencyCode,
		Notes:             body.Notes,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"permit": permit})
}

type updatePermitRequest struct {
	ProspectID        string  `json:"prospect_id"`
	ApplicationNumber *string `json:"application_number,omitempty"`
	SubmittedDate     *string `json:"submitted_date,omitempty"`
	ExpectedIssueDate *string `json:"expected_issue_date,omitempty"`
	ActualIssueDate   *string `json:"actual_issue_date,omitempty"`
	FeeCents          *int64  `json:"fee_cents,omitempty"`
	Status            *string `json:"status,omitempty"`
	Notes             *string `json:"notes,omitempty"`
}

// UpdatePermit modifies an existing permit.
// PUT /api/v1/org/{orgID}/pipeline/permits/{permitID}
func (h *PipelineHandler) UpdatePermit(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireOrgIDFromURL(w, r); !ok {
		return
	}
	permitID, ok := parseUUIDFromURL(w, r, "permitID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body updatePermitRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	prospectID, err := uuid.Parse(body.ProspectID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "prospect_id is required and must be a UUID")
		return
	}
	submitted, err := parseOptionalDate(body.SubmittedDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "submitted_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	expected, err := parseOptionalDate(body.ExpectedIssueDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "expected_issue_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	actual, err := parseOptionalDate(body.ActualIssueDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "actual_issue_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	permit, err := h.svc.UpdatePermit(r.Context(), service.UpdatePermitInput{
		PermitID:          permitID,
		ProspectID:        prospectID,
		OrgID:             callerOrg,
		ApplicationNumber: body.ApplicationNumber,
		SubmittedDate:     submitted,
		ExpectedIssueDate: expected,
		ActualIssueDate:   actual,
		FeeCents:          body.FeeCents,
		NewStatus:         body.Status,
		Notes:             body.Notes,
	})
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"permit": permit})
}

// ---------- Analytics ----------

// Analytics returns per-currency pipeline rollups: total estimated,
// weighted revenue, and prospect count. Computed on demand from a
// single SQL aggregation in the service layer; no cache.
// GET /api/v1/org/{orgID}/pipeline/analytics
func (h *PipelineHandler) Analytics(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.GetPipelineAnalytics(r.Context(), orgID)
	if err != nil {
		writePipelineError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"analytics": rows})
}

// ---------- helpers ----------

// writePipelineError maps pipeline-service sentinel errors to HTTP codes.
// Falls through to the shared budget-error mapping for ErrNotFound /
// ErrInvalidInput / ErrCrossCurrency, then 500 for anything else.
func writePipelineError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidTransition):
		writeErrorResponse(w, r, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, service.ErrTerminalStage):
		writeErrorResponse(w, r, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, service.ErrNotImplemented):
		writeErrorResponse(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// requireOrgIDFromURL extracts {orgID} from the URL and verifies it
// matches the JWT caller's claim. Shared with FinancialsHandler.
// (Eventual home: a shared api/auth_helpers.go file once a third
// handler adopts it.)
func requireOrgIDFromURL(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	urlOrg, ok := parseUUIDFromURL(w, r, "orgID")
	if !ok {
		return uuid.Nil, false
	}
	caller, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return uuid.Nil, false
	}
	if urlOrg != caller {
		writeErrorResponse(w, r, http.StatusForbidden, "FORBIDDEN", "org_id mismatch")
		return uuid.Nil, false
	}
	return urlOrg, true
}

// callerOrgIDFromClaims parses the caller's org_id claim from JWT.
// Writes 401 on parse failure (claim is malformed). Shared by all
// authenticated handlers.
func callerOrgIDFromClaims(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	claims := mw.MustClaimsFromContext(r.Context())
	parsed, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid org_id claim")
		return uuid.Nil, false
	}
	return parsed, true
}

// parsePagination reads ?page= and ?per_page= with sane defaults.
func parsePagination(r *http.Request) (page, perPage int) {
	page = 1
	perPage = 50
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
		}
	}
	return page, perPage
}

// paginationMeta is included in the standard envelope for list endpoints.
type paginationMeta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// writeJSONWithPagination wraps a list response in the envelope plus
// pagination meta. Mirrors writeJSON but adds a "pagination" sub-object
// to the meta field per API_CONTRACT §2.3.
func writeJSONWithPagination(w http.ResponseWriter, r *http.Request, status int, data any, p paginationMeta) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	wrapped := struct {
		Data       any            `json:"data,omitempty"`
		Meta       meta           `json:"meta"`
		Pagination paginationMeta `json:"pagination"`
	}{
		Data:       data,
		Meta:       buildMeta(r),
		Pagination: p,
	}
	_ = json.NewEncoder(w).Encode(wrapped)
}
