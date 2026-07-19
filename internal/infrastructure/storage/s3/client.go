// Package s3 provides the S3-compatible object storage client (R2 in production, MinIO in dev).
package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps a minio S3-compatible client bound to a single bucket.
type Client struct {
	mc     *minio.Client
	bucket string
}

// NewClient constructs a Client from the S3_* environment configuration.
func NewClient(endpoint, region, bucket, accessKey, secretKey string, usePathStyle bool) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("s3: parse endpoint %q: %w", endpoint, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("s3: endpoint %q has no host — include the scheme, e.g. http://localhost:9000", endpoint)
	}

	lookup := minio.BucketLookupDNS
	if usePathStyle {
		lookup = minio.BucketLookupPath
	}

	mc, err := minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       u.Scheme == "https",
		Region:       region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: init client: %w", err)
	}

	return &Client{mc: mc, bucket: bucket}, nil
}

// Upload writes data to key and returns its stored URL — the same URL shape
// DeleteByURL/Download parse back out via keyFromURL, so a value returned
// here round-trips through either without a separate convention.
func (c *Client) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	_, err := c.mc.PutObject(ctx, c.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("s3: upload object %q: %w", key, err)
	}
	return c.mc.EndpointURL().String() + "/" + c.bucket + "/" + key, nil
}

// DeleteByURL removes the object a stored URL (e.g. TEST_RESULT.pdf_url) points to.
func (c *Client) DeleteByURL(ctx context.Context, rawURL string) error {
	key, err := c.keyFromURL(rawURL)
	if err != nil {
		return err
	}
	if err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("s3: delete object %q: %w", key, err)
	}
	return nil
}

// Download streams the object a stored URL (e.g. TEST_RESULT.pdf_url) points
// to — proxied through our own API rather than handed out as a signed URL, so
// the browser never needs a cross-origin redirect to the storage backend
// (which forces an opaque "null" Origin per the Fetch spec's redirect
// algorithm, something R2's CORS policy form won't let us allow-list). The
// caller owns closing the returned reader.
func (c *Client) Download(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	key, err := c.keyFromURL(rawURL)
	if err != nil {
		return nil, err
	}
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("s3: download object %q: %w", key, err)
	}
	// GetObject never errors eagerly (S3 is lazy) — the first Read() is what
	// actually surfaces a missing/inaccessible object, so probe it now
	// rather than handing the caller a reader that fails silently later.
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, fmt.Errorf("s3: stat object %q: %w", key, err)
	}
	return obj, nil
}

// keyFromURL extracts the object key from a stored object URL, handling both
// path-style (http://host/bucket/key) and virtual-hosted.
func (c *Client) keyFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("s3: parse object url %q: %w", rawURL, err)
	}
	key := strings.TrimPrefix(u.Path, "/")
	key = strings.TrimPrefix(key, c.bucket+"/")
	if key == "" {
		return "", fmt.Errorf("s3: object url %q has an empty key", rawURL)
	}
	return key, nil
}
