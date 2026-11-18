package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Harshwagh21/NimbusCloud/config"
)

// S3Storage implements ObjectStorage for any S3-compatible backend.
type S3Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

var _ ObjectStorage = (*S3Storage)(nil)

func NewS3(cfg config.Storage) *S3Storage {
	client := s3.New(s3.Options{
		Region:       cfg.Region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		// MinIO serves buckets as a path (host/bucket/key), not a subdomain (bucket.host).
		UsePathStyle: cfg.ForcePathStyle,
		// The SDK's default checksum would sign a CRC of an empty body into presigned PUT
		// URLs, so every browser upload would fail verification. R2 rejects it too.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &S3Storage{
		client:  client,
		presign: s3.NewPresignClient(client, s3.WithPresignExpires(cfg.PresignTTL)),
		bucket:  cfg.Bucket,
	}
}

func (s *S3Storage) PresignPut(ctx context.Context, key, contentType string, size int64) (PresignedRequest, error) {
	// ContentLength and ContentType are signed, so storage rejects any other size or type.
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &key,
		ContentType:   &contentType,
		ContentLength: &size,
	})
	if err != nil {
		return PresignedRequest{}, fmt.Errorf("presign put: %w", err)
	}
	headers := req.SignedHeader.Clone()
	// Browsers set these two themselves and refuse to let a script set them.
	headers.Del("Host")
	headers.Del("Content-Length")
	return PresignedRequest{Method: req.Method, URL: req.URL, Headers: headers}, nil
}

func (s *S3Storage) PresignGet(ctx context.Context, key, downloadName string) (string, error) {
	// RFC 6266 filename*: percent-encoded UTF-8, so quotes or newlines in a name cannot
	// break out of the header.
	disposition := "attachment; filename*=UTF-8''" + url.PathEscape(downloadName)
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     &s.bucket,
		Key:                        &key,
		ResponseContentDisposition: &disposition,
	})
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return req.URL, nil
}

func (s *S3Storage) Head(ctx context.Context, key string) (ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return ObjectInfo{}, mapErr("head", err)
	}
	return ObjectInfo{Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType)}, nil
}

func (s *S3Storage) ReadPrefix(ctx context.Context, key string, n int64) ([]byte, error) {
	byteRange := fmt.Sprintf("bytes=0-%d", n-1)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key, Range: &byteRange})
	if err != nil {
		return nil, mapErr("read prefix", err)
	}
	defer func() { _ = out.Body.Close() }()
	// LimitReader guards against a backend that ignores Range and streams the whole object.
	data, err := io.ReadAll(io.LimitReader(out.Body, n))
	if err != nil {
		return nil, fmt.Errorf("read prefix body: %w", err)
	}
	return data, nil
}

// Name and Check make S3Storage a health.Checker, so /readyz reports storage.
func (s *S3Storage) Name() string { return "storage" }

func (s *S3Storage) Check(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket})
	return err
}

// mapErr turns any 404 (HeadObject's bare NotFound or GetObject's NoSuchKey) into ErrNotFound.
func mapErr(op string, err error) error {
	var resp *awshttp.ResponseError
	if errors.As(err, &resp) && resp.HTTPStatusCode() == http.StatusNotFound {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
