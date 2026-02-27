// Package objstore provides a [flush.Storage] implementation that uploads
// coverage files to a remote object store (S3, GCS, Azure Blob, etc.).
//
// The caller provides an [Uploader] function that performs the actual upload,
// keeping cloud SDK dependencies out of this module.
package objstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yag13s/goreach/flush"
)

// Uploader uploads the contents of body to the given key.
// The caller is responsible for closing body after Uploader returns.
type Uploader func(ctx context.Context, key string, body io.Reader) error

// KeyFunc generates an object key for a given file and metadata.
type KeyFunc func(prefix string, meta flush.Metadata, filename string) string

// Storage uploads coverage files using the provided [Uploader].
type Storage struct {
	Upload  Uploader
	Prefix  string  // key prefix (default "goreach")
	KeyFunc KeyFunc // custom key generator (nil uses defaultKey)
}

// compile-time check
var _ flush.Storage = (*Storage)(nil)

// Store uploads each file in files via the configured [Uploader].
func (s *Storage) Store(ctx context.Context, files []string, meta flush.Metadata) error {
	prefix := s.Prefix
	if prefix == "" {
		prefix = "goreach"
	}
	keyFn := s.KeyFunc
	if keyFn == nil {
		keyFn = defaultKey
	}

	for _, f := range files {
		body, err := os.Open(f)
		if err != nil {
			return fmt.Errorf("goreach/flush: open %s: %w", filepath.Base(f), err)
		}

		key := keyFn(prefix, meta, filepath.Base(f))
		uploadErr := s.Upload(ctx, key, body)
		body.Close()

		if uploadErr != nil {
			return fmt.Errorf("goreach/flush: upload %s: %w", filepath.Base(f), uploadErr)
		}
	}
	return nil
}

// defaultKey produces keys in the form: <prefix>/<service>/<version>/<pod>/<filename>.
func defaultKey(prefix string, meta flush.Metadata, filename string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		prefix, meta.ServiceName, meta.BuildVersion, meta.PodName, filename)
}
