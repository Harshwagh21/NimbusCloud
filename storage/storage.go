// Package storage is the only code that talks to object storage. Services depend on the
// ObjectStorage interface, so MinIO (local), Cloudflare R2 (production) and test fakes are
// interchangeable, and no service ever imports the AWS SDK.
package storage

import (
	"context"
	"errors"
	"net/http"
)

// ErrNotFound means the key has no object. A presigned PUT that the client never sent
// looks exactly like this, which is how an upload is proven to have happened.
var ErrNotFound = errors.New("storage: object not found")

// PresignedRequest is everything a browser needs to send bytes straight to storage.
// Headers must be sent exactly as given, because they are part of the signature.
type PresignedRequest struct {
	Method  string
	URL     string
	Headers http.Header
}

// ObjectInfo is what storage reports about a stored object. Size is the truth; the size
// a client declared is only a claim.
type ObjectInfo struct {
	Size        int64
	ContentType string
}

// ObjectStorage is the contract every backend implements.
type ObjectStorage interface {
	// PresignPut returns a short-lived URL that accepts exactly one object of this size and type.
	PresignPut(ctx context.Context, key, contentType string, size int64) (PresignedRequest, error)
	// PresignGet returns a short-lived download URL that saves the file as downloadName.
	PresignGet(ctx context.Context, key, downloadName string) (string, error)
	// Head reports the stored object's real size and type, or ErrNotFound.
	Head(ctx context.Context, key string) (ObjectInfo, error)
	// ReadPrefix returns up to the first n bytes of the object, for content sniffing.
	ReadPrefix(ctx context.Context, key string, n int64) ([]byte, error)
}
