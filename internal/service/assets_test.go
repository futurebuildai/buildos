package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// fakeObjectStore is an in-memory ObjectStore for unit tests (no network).
type fakeObjectStore struct {
	getBody io.ReadCloser
	getCT   string
}

func (f *fakeObjectStore) PresignPut(_ context.Context, key, ct string, _ int64, _ time.Duration) (string, map[string]string, error) {
	return "https://example.test/" + key, map[string]string{"Content-Type": ct}, nil
}
func (f *fakeObjectStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://example.test/get/" + key, nil
}
func (f *fakeObjectStore) Get(_ context.Context, _ string) (io.ReadCloser, string, error) {
	return f.getBody, f.getCT, nil
}
func (f *fakeObjectStore) Delete(_ context.Context, _ string) error { return nil }

// configuredResolver returns a resolver yielding the given fake store.
func configuredResolver(f ObjectStore) ObjectStoreResolver {
	return func(context.Context, uuid.UUID) (ObjectStore, error) { return f, nil }
}

// newAssetSvc builds an AssetService with the given resolver and a nil pool —
// safe ONLY for paths that reject before touching the DB (validation / 503).
func newAssetSvc(resolver ObjectStoreResolver) *AssetService {
	return NewAssetService(nil, store.NewAssetStore(), nil, resolver, nil, nil, nil)
}

func TestAssetService_RequestUpload_Unconfigured503(t *testing.T) {
	// nil resolver => storage unconfigured => ErrStorageUnavailable before any DB.
	svc := newAssetSvc(nil)
	pid := uuid.New()
	_, err := svc.RequestUpload(context.Background(), uuid.New(), "sub", RequestUploadInput{
		ProjectID:   &pid,
		ContentType: "image/jpeg",
		SizeBytes:   1024,
	})
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("err = %v, want ErrStorageUnavailable", err)
	}
}

func TestAssetService_SignedGetURL_Unconfigured503(t *testing.T) {
	svc := newAssetSvc(nil)
	_, err := svc.SignedGetURL(context.Background(), uuid.New(), uuid.New(), 0)
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("err = %v, want ErrStorageUnavailable", err)
	}
}

func TestAssetService_RequestUpload_RejectsBadContentType(t *testing.T) {
	svc := newAssetSvc(configuredResolver(&fakeObjectStore{}))
	pid := uuid.New()
	cases := []string{"application/pdf", "image/gif", "text/plain", "", "image/svg+xml"}
	for _, ct := range cases {
		_, err := svc.RequestUpload(context.Background(), uuid.New(), "sub", RequestUploadInput{
			ProjectID: &pid, ContentType: ct, SizeBytes: 1024,
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("content_type %q: err = %v, want ErrInvalidInput", ct, err)
		}
	}
}

func TestAssetService_RequestUpload_AcceptsAllowedContentTypes(t *testing.T) {
	// Allowed types must pass VALIDATION (they then reach the DB tx, which a nil
	// pool will panic on — so we only assert they did NOT fail validation by
	// confirming the error, if any, is NOT ErrInvalidInput. We recover the
	// nil-pool panic to keep the test pure.)
	for ct := range allowedAssetContentTypes {
		func() {
			defer func() { _ = recover() }() // nil pool panics in the tx; expected
			svc := newAssetSvc(configuredResolver(&fakeObjectStore{}))
			pid := uuid.New()
			_, err := svc.RequestUpload(context.Background(), uuid.New(), "sub", RequestUploadInput{
				ProjectID: &pid, ContentType: ct, SizeBytes: 1024,
			})
			if errors.Is(err, ErrInvalidInput) {
				t.Errorf("allowed content_type %q rejected as invalid", ct)
			}
		}()
	}
}

func TestAssetService_RequestUpload_RejectsBadSize(t *testing.T) {
	svc := newAssetSvc(configuredResolver(&fakeObjectStore{}))
	pid := uuid.New()
	cases := []int64{0, -1, MaxAssetSizeBytes + 1, 1 << 40}
	for _, sz := range cases {
		_, err := svc.RequestUpload(context.Background(), uuid.New(), "sub", RequestUploadInput{
			ProjectID: &pid, ContentType: "image/png", SizeBytes: sz,
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("size %d: err = %v, want ErrInvalidInput", sz, err)
		}
	}
}

func TestAssetService_RequestUpload_RejectsNilOrg(t *testing.T) {
	svc := newAssetSvc(configuredResolver(&fakeObjectStore{}))
	_, err := svc.RequestUpload(context.Background(), uuid.Nil, "sub", RequestUploadInput{
		ContentType: "image/jpeg", SizeBytes: 10,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// TestAsset_StorageKeyNeverSerialized asserts the opaque object key is never
// emitted in the JSON wire shape (it would leak the bucket layout).
func TestAsset_StorageKeyNeverSerialized(t *testing.T) {
	a := models.Asset{
		ID:          uuid.New(),
		OrgID:       uuid.New(),
		StorageKey:  "org/abc/project/def/secret-key.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   1234,
		Status:      models.AssetStatusReady,
		UploadedBy:  "sub-123",
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte("secret-key")) || bytes.Contains(raw, []byte("storage_key")) {
		t.Fatalf("storage_key leaked into JSON: %s", raw)
	}
	// Sanity: a non-secret field IS present.
	if !bytes.Contains(raw, []byte("image/jpeg")) {
		t.Fatalf("expected content_type in JSON: %s", raw)
	}
}

// TestStripImageMetadata_DropsEXIF builds a JPEG carrying a fabricated EXIF-like
// APP1 marker, runs it through stripImageMetadata, and asserts the re-encoded
// output no longer contains the marker bytes (the decode→encode round trip drops
// all metadata, incl. GPS EXIF — D-4 redaction-leak guard).
func TestStripImageMetadata_DropsEXIF(t *testing.T) {
	// A real EXIF segment starts with the APP1 marker 0xFFE1 then "Exif\0\0".
	// We construct a valid baseline JPEG, then splice an APP1 "Exif" segment
	// with a recognizable GPS sentinel right after SOI so a naive byte scan of
	// the ORIGINAL finds it and of the STRIPPED output does not.
	var orig bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 100, A: 255})
		}
	}
	if err := jpeg.Encode(&orig, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode base jpeg: %v", err)
	}

	exifPayload := []byte("Exif\x00\x00GPSLATITUDE-SENTINEL-12.345")
	// APP1 marker (0xFF 0xE1) + 2-byte length + payload.
	app1 := append([]byte{0xFF, 0xE1, 0x00, byte(len(exifPayload) + 2)}, exifPayload...)
	withEXIF := append([]byte{}, orig.Bytes()[:2]...) // SOI (FFD8)
	withEXIF = append(withEXIF, app1...)
	withEXIF = append(withEXIF, orig.Bytes()[2:]...)

	if !bytes.Contains(withEXIF, []byte("GPSLATITUDE-SENTINEL")) {
		t.Fatal("test setup: sentinel not present in pre-strip bytes")
	}

	stripped, ct, ok := stripImageMetadata(bytes.NewReader(withEXIF), "image/jpeg")
	if !ok {
		t.Fatal("stripImageMetadata returned ok=false for image/jpeg")
	}
	if ct != "image/jpeg" {
		t.Errorf("content-type = %q", ct)
	}
	if bytes.Contains(stripped, []byte("GPSLATITUDE-SENTINEL")) {
		t.Fatal("EXIF GPS sentinel survived the strip — metadata leaked")
	}
	// The stripped output must still be a decodable JPEG.
	if _, err := jpeg.Decode(bytes.NewReader(stripped)); err != nil {
		t.Fatalf("stripped output is not a valid jpeg: %v", err)
	}
}

func TestStripImageMetadata_UnsupportedFormat(t *testing.T) {
	if _, _, ok := stripImageMetadata(bytes.NewReader([]byte("RIFF....WEBP")), "image/webp"); ok {
		t.Fatal("webp should not be stripped by the stdlib path (ok must be false)")
	}
	if _, _, ok := stripImageMetadata(bytes.NewReader([]byte("....")), "image/heic"); ok {
		t.Fatal("heic should not be stripped (ok must be false)")
	}
}

// TestBuildStorageKey covers the org/project key layout (Internal UUIDs only).
func TestBuildStorageKey(t *testing.T) {
	org := uuid.New()
	proj := uuid.New()
	withProj := buildStorageKey(org, &proj, ".jpg")
	if got := "org/" + org.String() + "/project/" + proj.String() + "/"; !contains(withProj, got) {
		t.Errorf("project key = %q, want prefix %q", withProj, got)
	}
	orgLevel := buildStorageKey(org, nil, ".png")
	if contains(orgLevel, "/project/") {
		t.Errorf("org-level key should have no /project/ segment: %q", orgLevel)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

// --- Chunk B: daily-log photo linking ---

// TestDedupeUUIDs covers de-dup + nil-drop + order preservation.
func TestDedupeUUIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	out := dedupeUUIDs([]uuid.UUID{a, b, a, uuid.Nil, b})
	if len(out) != 2 || out[0] != a || out[1] != b {
		t.Fatalf("dedupeUUIDs = %v, want [%v %v]", out, a, b)
	}
	if dedupeUUIDs(nil) != nil {
		t.Fatal("dedupeUUIDs(nil) should be nil")
	}
	if len(dedupeUUIDs([]uuid.UUID{uuid.Nil})) != 0 {
		t.Fatal("dedupeUUIDs of only-nil should be empty")
	}
}

// TestLinkPhotosToDailyLog_ValidationGates exercises the argument gates that
// reject before any DB access (nil pool/store stay safe).
func TestLinkPhotosToDailyLog_ValidationGates(t *testing.T) {
	svc := newAssetSvc(configuredResolver(&fakeObjectStore{}))
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	good := uuid.New()
	cases := []struct {
		name string
		org  uuid.UUID
		proj uuid.UUID
		day  time.Time
		ids  []uuid.UUID
		want error
	}{
		{"nil org", uuid.Nil, uuid.New(), day, []uuid.UUID{good}, ErrInvalidInput},
		{"nil project", uuid.New(), uuid.Nil, day, []uuid.UUID{good}, ErrInvalidInput},
		{"zero day", uuid.New(), uuid.New(), time.Time{}, []uuid.UUID{good}, ErrInvalidInput},
		{"empty ids", uuid.New(), uuid.New(), day, nil, ErrInvalidInput},
		{"only-nil ids", uuid.New(), uuid.New(), day, []uuid.UUID{uuid.Nil}, ErrInvalidInput},
		{"over cap", uuid.New(), uuid.New(), day, manyUUIDs(MaxAssetsPerDailyLog + 1), ErrInvalidInput},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.LinkPhotosToDailyLog(context.Background(), c.org, "sub", c.proj, c.day, c.ids)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func manyUUIDs(n int) []uuid.UUID {
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = uuid.New()
	}
	return out
}
