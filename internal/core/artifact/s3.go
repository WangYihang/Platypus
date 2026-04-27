package artifact

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config carries the fields the YAML config file exposes for the
// object-store backend. Works against AWS S3, MinIO, Ceph RGW, or any
// other S3-compatible endpoint.
type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	Prefix          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
}

// S3Store is the concrete Store backed by an S3-compatible object store.
type S3Store struct {
	client *minio.Client
	bucket string
	prefix string
}

// NewS3Store connects to the configured endpoint. It does not verify
// that the bucket exists — the release pipeline is expected to have
// created it — so a typo here surfaces as a 404 on the first request.
func NewS3Store(cfg S3Config) (*S3Store, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("artifact: s3 endpoint is empty")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("artifact: s3 bucket is empty")
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("artifact: init minio client: %w", err)
	}
	return &S3Store{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.TrimSuffix(cfg.Prefix, "/"),
	}, nil
}

// Prefix returns the trimmed bucket-local prefix.
func (s *S3Store) Prefix() string { return s.prefix }

// fullKey joins the configured prefix with the caller-supplied sub-key.
func (s *S3Store) fullKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return path.Join(s.prefix, key)
}

// GetObject downloads a full object into memory. Only used for small
// files (manifest JSON + signature), never for agent binaries.
func (s *S3Store) GetObject(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.fullKey(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("artifact: get %s: %w", key, err)
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("artifact: read %s: %w", key, err)
	}
	return data, nil
}

// GetObjectReader returns a streaming reader for key, plus size and
// content type from the object stat. The caller must close the returned
// ReadCloser. Used by the Distributor to proxy agent binary downloads
// through the server instead of issuing presigned redirect URLs.
func (s *S3Store) GetObjectReader(ctx context.Context, key string) (io.ReadCloser, int64, string, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.fullKey(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, "", fmt.Errorf("artifact: get reader %s: %w", key, err)
	}
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, 0, "", fmt.Errorf("artifact: stat %s: %w", key, err)
	}
	return obj, info.Size, info.ContentType, nil
}
