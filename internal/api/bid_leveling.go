package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/agents"
)

// BidLevelingHandler handles POST /api/v1/procurement/bids/analyze.
type BidLevelingHandler struct {
	agent *agents.BidLevelingAgent
}

// NewBidLevelingHandler creates a new BidLevelingHandler.
// Returns nil if agent is nil (AI not configured).
func NewBidLevelingHandler(agent *agents.BidLevelingAgent) *BidLevelingHandler {
	if agent == nil {
		return nil
	}
	return &BidLevelingHandler{agent: agent}
}

// analyzeBidsRequest is the request body for POST /api/v1/procurement/bids/analyze.
type analyzeBidsRequest struct {
	OrgID     string            `json:"org_id"`
	ProjectID string            `json:"project_id"`
	ItemID    *string           `json:"item_id,omitempty"`
	Bids      []agents.BidInput `json:"bids"`
}

// AnalyzeBids performs Claude-powered bid leveling on multiple vendor bids.
// POST /api/v1/procurement/bids/analyze
func (h *BidLevelingHandler) AnalyzeBids(w http.ResponseWriter, r *http.Request) {
	var body analyzeBidsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	// Validate required fields
	orgID, err := uuid.Parse(body.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid org_id")
		return
	}

	projectID, err := uuid.Parse(body.ProjectID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid project_id")
		return
	}

	var itemID *uuid.UUID
	if body.ItemID != nil {
		parsed, err := uuid.Parse(*body.ItemID)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid item_id")
			return
		}
		itemID = &parsed
	}

	if len(body.Bids) < 2 {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "at least 2 bids required for comparison")
		return
	}

	// Validate each bid has line items
	for i, bid := range body.Bids {
		if bid.Vendor == "" {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR",
				"bid at index "+itoa(i)+" is missing vendor name")
			return
		}
		if len(bid.LineItems) == 0 {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR",
				"bid from "+bid.Vendor+" has no line items")
			return
		}
		for _, li := range bid.LineItems {
			if li.CurrencyCode == "" {
				writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR",
					"line item in bid from "+bid.Vendor+" is missing currency_code")
				return
			}
			if li.UnitPriceCents < 0 {
				writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR",
					"line item in bid from "+bid.Vendor+" has negative unit_price_cents")
				return
			}
		}
	}

	analysis, err := h.agent.AnalyzeBids(r.Context(), orgID, projectID, itemID, body.Bids)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "ANALYSIS_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusOK, analysis)
}

// itoa converts an int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
