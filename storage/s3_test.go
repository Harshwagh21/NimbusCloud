package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/Harshwagh21/NimbusCloud/config"
)

// openMinIO connects to the S3 backend named by the S3_* variables (.env locally, the CI
// service container in CI). Each test gets its own key prefix and cleans up after itself.
func openMinIO(t *testing.T) (*S3Storage, string) {
	t.Helper()
	_ = godotenv.Load("../.env")
	cfg := config.Storage{
		Endpoint:       os.Getenv("S3_ENDPOINT"),
		Region:         "us-east-1",
		Bucket:         os.Getenv("S3_BUCKET"),
		AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		SecretKey:      os.Getenv("S3_SECRET_KEY"),
		ForcePathStyle: true,
		PresignTTL:     time.Minute,
	}
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		t.Fatal("S3_ENDPOINT and S3_BUCKET are required for storage integration tests (run make up)")
	}
	store := NewS3(cfg)
	if err := store.Check(context.Background()); err != nil {
		t.Fatalf("storage not reachable (is MinIO up?): %v", err)
	}
	prefix := "test/" + uuid.NewString() + "/"
	t.Cleanup(func() { deletePrefix(t, store, prefix) })
	return store, prefix
}

func deletePrefix(t *testing.T, store *S3Storage, prefix string) {
	ctx := context.Background()
	out, err := store.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: &store.bucket, Prefix: &prefix})
	if err != nil {
		t.Logf("cleanup list: %v", err)
		return
	}
	for _, obj := range out.Contents {
		_, _ = store.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &store.bucket, Key: obj.Key})
	}
}

// put sends body to a presigned request the way a browser would: the signed headers, and
// a Content-Length the HTTP client sets from the body.
func put(t *testing.T, req PresignedRequest, body []byte) int {
	t.Helper()
	httpReq, err := http.NewRequest(req.Method, req.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(httpReq.Header, req.Headers)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestS3PresignedUploadRoundTrip(t *testing.T) {
	store, prefix := openMinIO(t)
	ctx := context.Background()
	key := prefix + "hello.png"
	body := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 600)...)

	req, err := store.PresignPut(ctx, key, "image/png", int64(len(body)))
	if err != nil {
		t.Fatalf("presign put: %v", err)
	}
	if req.Headers.Get("Host") != "" || req.Headers.Get("Content-Length") != "" {
		t.Fatalf("browser-forbidden headers leaked: %v", req.Headers)
	}
	if status := put(t, req, body); status != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", status)
	}

	info, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if info.Size != int64(len(body)) {
		t.Fatalf("size = %d, want %d", info.Size, len(body))
	}

	prefixBytes, err := store.ReadPrefix(ctx, key, 512)
	if err != nil {
		t.Fatalf("read prefix: %v", err)
	}
	if len(prefixBytes) != 512 || http.DetectContentType(prefixBytes) != "image/png" {
		t.Fatalf("prefix len %d sniffed %q", len(prefixBytes), http.DetectContentType(prefixBytes))
	}

	url, err := store.PresignGet(ctx, key, `my "photo".png`)
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, body) {
		t.Fatalf("GET status %d, %d bytes", resp.StatusCode, len(got))
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "filename*=UTF-8''my%20%22photo%22.png") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
}

func TestS3PresignedPutRejectsOtherSize(t *testing.T) {
	store, prefix := openMinIO(t)
	key := prefix + "liar.bin"

	req, err := store.PresignPut(context.Background(), key, "application/octet-stream", 10)
	if err != nil {
		t.Fatal(err)
	}
	// The URL was signed for 10 bytes; sending 11 must fail the signature check.
	if status := put(t, req, bytes.Repeat([]byte("x"), 11)); status == http.StatusOK {
		t.Fatal("storage accepted a body larger than the signed Content-Length")
	}
	if _, err := store.Head(context.Background(), key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("head after rejected PUT: err = %v, want ErrNotFound", err)
	}
}

func TestS3MissingKeyIsErrNotFound(t *testing.T) {
	store, prefix := openMinIO(t)
	ctx := context.Background()

	if _, err := store.Head(ctx, prefix+"never-uploaded"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Head err = %v, want ErrNotFound", err)
	}
	if _, err := store.ReadPrefix(ctx, prefix+"never-uploaded", 512); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ReadPrefix err = %v, want ErrNotFound", err)
	}
}
