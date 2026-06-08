package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/currency"
	"github.com/futurebuildai/buildos/internal/models"
)

// fakeExtractor is the test double for invoiceExtractorAI. Captures the
// last request and replays a scripted response/error so we can exercise
// the deterministic gate + soft-fail without an HTTP server or a DB.
// Mirrors fakeBriefer/fakeAdjuster in agents_test.go.
type fakeExtractor struct {
	lastReq ai.InvoiceExtractRequest
	resp    *ai.InvoiceExtractResponse
	err     error
}

func (f *fakeExtractor) InvoiceExtract(_ context.Context, req ai.InvoiceExtractRequest) (*ai.InvoiceExtractResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// ingestWithFake builds an IngestionService whose AI seam is the supplied
// fake (bypassing the typed-nil constructor guard so we can drive the
// pre-tx phases directly). pool/budget/stores stay nil — every test here
// either short-circuits before BeginTxFunc (soft-fail, 422 rejection) or
// exercises validateExtraction directly, so the DB is never touched.
func ingestWithFake(f invoiceExtractorAI, tol int64) *IngestionService {
	return &IngestionService{
		ai:                     f,
		audit:                  NewNoopAuditRecorder(),
		mismatchToleranceCents: tol,
	}
}

func validResp() *ai.InvoiceExtractResponse {
	return &ai.InvoiceExtractResponse{
		VendorName:   "Acme Lumber",
		InvoiceNo:    "INV-1001",
		IssuedDate:   "2026-06-01",
		TotalCents:   150000,
		CurrencyCode: currency.USD,
		LineItems: []ai.InvoiceExtractLineItem{
			{Description: "2x4 studs", Quantity: 100, UnitCents: 1000, AmountCents: 100000},
			{Description: "plywood", Quantity: 25, UnitCents: 2000, AmountCents: 50000},
		},
	}
}

// --- soft-fail: nil AI client and ai.ErrUnconfigured -------------------

func TestIngest_SoftFail_NilClient(t *testing.T) {
	// Constructed with a nil *ai.Client → s.ai stays nil → soft-fail.
	svc := NewIngestionService(nil, nil, nil, nil, nil, nil)
	_, err := svc.IngestInvoiceFromDocument(context.Background(), uuid.New(), uuid.New().String(),
		IngestInvoiceInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), Text: "anything"})
	if !errors.Is(err, ai.ErrUnconfigured) {
		t.Fatalf("nil client: err = %v, want ai.ErrUnconfigured", err)
	}
}

func TestIngest_SoftFail_ExtractorUnconfigured(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{err: ai.ErrUnconfigured}, 0)
	_, err := svc.IngestInvoiceFromDocument(context.Background(), uuid.New(), uuid.New().String(),
		IngestInvoiceInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), Text: "x"})
	if !errors.Is(err, ai.ErrUnconfigured) {
		t.Fatalf("unconfigured extractor: err = %v, want ai.ErrUnconfigured", err)
	}
}

// A nil pool would panic on BeginTxFunc; the soft-fail tests above must
// return BEFORE the tx opens. (No panic = the guard fired pre-tx.)

// --- transport errors map to 422 (media) vs propagate ------------------

func TestIngest_UnsupportedMediaType_Is422(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{err: ai.ErrUnsupportedMediaType}, 0)
	_, err := svc.IngestInvoiceFromDocument(context.Background(), uuid.New(), uuid.New().String(),
		IngestInvoiceInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), DocumentURL: "https://x/y.bmp"})
	if !errors.Is(err, ErrInvoiceExtractionInvalid) {
		t.Fatalf("unsupported media: err = %v, want ErrInvoiceExtractionInvalid", err)
	}
}

func TestIngest_TransportError_Propagates(t *testing.T) {
	boom := errors.New("upstream 503")
	svc := ingestWithFake(&fakeExtractor{err: boom}, 0)
	_, err := svc.IngestInvoiceFromDocument(context.Background(), uuid.New(), uuid.New().String(),
		IngestInvoiceInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), Text: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("transport error: err = %v, want wrapped boom", err)
	}
	if errors.Is(err, ai.ErrUnconfigured) || errors.Is(err, ErrInvoiceExtractionInvalid) {
		t.Fatalf("transport error must not map to soft-fail/422: %v", err)
	}
}

// --- deterministic gate: 422 rejections (pre-tx, nil pool safe) --------

func TestIngest_RejectsBadFuzzyOutput(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*ai.InvoiceExtractResponse)
		in   IngestInvoiceInput
	}{
		{"empty vendor", func(r *ai.InvoiceExtractResponse) { r.VendorName = "" }, IngestInvoiceInput{}},
		{"bad currency", func(r *ai.InvoiceExtractResponse) { r.CurrencyCode = "EUR" }, IngestInvoiceInput{}},
		{"zero total", func(r *ai.InvoiceExtractResponse) { r.TotalCents = 0 }, IngestInvoiceInput{}},
		{"negative total", func(r *ai.InvoiceExtractResponse) { r.TotalCents = -1 }, IngestInvoiceInput{}},
		{
			"line-sum mismatch beyond tolerance",
			func(r *ai.InvoiceExtractResponse) { r.TotalCents = 150001 }, // lines sum 150000
			IngestInvoiceInput{},
		},
		{
			"currency override mismatch",
			func(r *ai.InvoiceExtractResponse) {}, // resp stays USD
			IngestInvoiceInput{CurrencyOverride: ptrString(currency.CAD)},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := validResp()
			c.mut(resp)
			svc := ingestWithFake(&fakeExtractor{resp: resp}, 0)
			in := c.in
			in.ProjectID = uuid.New()
			in.IdempotencyKey = uuid.New()
			in.Text = "doc"
			_, err := svc.IngestInvoiceFromDocument(context.Background(), uuid.New(), uuid.New().String(), in)
			if !errors.Is(err, ErrInvoiceExtractionInvalid) {
				t.Fatalf("%s: err = %v, want ErrInvoiceExtractionInvalid", c.name, err)
			}
		})
	}
}

// --- validateExtraction: happy mapping + line-sum semantics ------------

func TestValidateExtraction_HappyMapping(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{}, 0)
	v, err := svc.validateExtraction(validResp(), nil)
	if err != nil {
		t.Fatalf("happy: unexpected err %v", err)
	}
	if v.vendorName != "Acme Lumber" {
		t.Errorf("vendor = %q, want Acme Lumber", v.vendorName)
	}
	if v.invoiceNumber == nil || *v.invoiceNumber != "INV-1001" {
		t.Errorf("invoice number = %v, want INV-1001", v.invoiceNumber)
	}
	// Persisted amount is the declared total (150000), NOT recomputed.
	if v.amountCents != 150000 {
		t.Errorf("amountCents = %d, want 150000 (declared total)", v.amountCents)
	}
	if v.currencyCode != currency.USD {
		t.Errorf("currency = %q, want USD", v.currencyCode)
	}
	if v.lineItemCount != 2 || v.lineSumCents != 150000 {
		t.Errorf("lines: count=%d sum=%d, want 2 / 150000", v.lineItemCount, v.lineSumCents)
	}
	if v.totalMismatch {
		t.Error("exact match must not flag totalMismatch")
	}
	if v.priority != models.FeedPriorityNormal {
		t.Errorf("priority = %q, want normal", v.priority)
	}
}

func TestValidateExtraction_EmptyLineItemsTrustsTotal(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{}, 0)
	resp := validResp()
	resp.LineItems = nil
	v, err := svc.validateExtraction(resp, nil)
	if err != nil {
		t.Fatalf("empty lines: unexpected err %v", err)
	}
	if v.amountCents != 150000 || v.totalMismatch || v.priority != models.FeedPriorityNormal {
		t.Errorf("empty lines must trust total: amount=%d mismatch=%v prio=%q", v.amountCents, v.totalMismatch, v.priority)
	}
}

func TestValidateExtraction_MismatchWithinTolerancePersistsUrgent(t *testing.T) {
	// Tolerance 5 cents; lines sum 150000, total declared 150003 (diff 3 ≤ 5).
	svc := ingestWithFake(&fakeExtractor{}, 5)
	resp := validResp()
	resp.TotalCents = 150003
	v, err := svc.validateExtraction(resp, nil)
	if err != nil {
		t.Fatalf("within-tolerance: unexpected err %v", err)
	}
	if v.amountCents != 150003 {
		t.Errorf("amountCents = %d, want 150003 (declared total persists)", v.amountCents)
	}
	if !v.totalMismatch {
		t.Error("within-tolerance mismatch must flag totalMismatch")
	}
	if v.priority != models.FeedPriorityUrgent {
		t.Errorf("priority = %q, want urgent", v.priority)
	}
}

func TestValidateExtraction_MismatchBeyondToleranceRejects(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{}, 5)
	resp := validResp()
	resp.TotalCents = 150006 // diff 6 > tolerance 5
	if _, err := svc.validateExtraction(resp, nil); !errors.Is(err, ErrInvoiceExtractionInvalid) {
		t.Fatalf("beyond tolerance: err = %v, want ErrInvoiceExtractionInvalid", err)
	}
}

func TestValidateExtraction_CurrencyOverrideMatchOK(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{}, 0)
	v, err := svc.validateExtraction(validResp(), ptrString(currency.USD))
	if err != nil {
		t.Fatalf("matching override: unexpected err %v", err)
	}
	if v.currencyCode != currency.USD {
		t.Errorf("currency = %q, want USD", v.currencyCode)
	}
}

func TestValidateExtraction_EmptyInvoiceNoStoresNil(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{}, 0)
	resp := validResp()
	resp.InvoiceNo = ""
	v, err := svc.validateExtraction(resp, nil)
	if err != nil {
		t.Fatalf("empty invoice no: unexpected err %v", err)
	}
	if v.invoiceNumber != nil {
		t.Errorf("empty invoice_no must map to nil, got %v", *v.invoiceNumber)
	}
}

func TestValidateExtraction_UnparsedIssuedDateFlagged(t *testing.T) {
	svc := ingestWithFake(&fakeExtractor{}, 0)
	resp := validResp()
	resp.IssuedDate = "not-a-date"
	v, err := svc.validateExtraction(resp, nil)
	if err != nil {
		t.Fatalf("bad issued date must NOT reject (it is dropped): %v", err)
	}
	if !v.issuedUnparsed {
		t.Error("unparseable issued_date must set issuedUnparsed")
	}
}

// --- integer-only money formatting (no float64) ------------------------

func TestFormatMoney_IntegerOnly(t *testing.T) {
	cases := []struct {
		cents int64
		code  string
		want  string
	}{
		{150000, "USD", "USD 1500.00"},
		{5, "CAD", "CAD 0.05"},
		{99, "USD", "USD 0.99"},
		{100, "USD", "USD 1.00"},
		{0, "USD", "USD 0.00"},
	}
	for _, c := range cases {
		if got := formatMoney(c.cents, c.code); got != c.want {
			t.Errorf("formatMoney(%d, %q) = %q, want %q", c.cents, c.code, got, c.want)
		}
	}
}

