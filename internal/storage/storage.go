package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage abstracts object storage (MinIO / S3 compatible).
type Storage struct {
	client   *minio.Client
	bucket   string
	publicURL string // public base URL for generating download URLs
}

// Config holds storage configuration.
type Config struct {
	Endpoint  string // e.g. "localhost:9000"
	AccessKey string
	SecretKey string
	Bucket    string // e.g. "openilink"
	UseSSL    bool
	PublicURL string // e.g. "https://s3.example.com/openilink"
}

// New creates a new Storage instance and ensures the bucket exists.
func New(cfg Config) (*Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: connect: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("storage: create bucket: %w", err)
		}
	}

	publicURL := cfg.PublicURL
	if publicURL == "" {
		scheme := "http"
		if cfg.UseSSL {
			scheme = "https"
		}
		publicURL = fmt.Sprintf("%s://%s/%s", scheme, cfg.Endpoint, cfg.Bucket)
	}

	return &Storage{client: client, bucket: cfg.Bucket, publicURL: publicURL}, nil
}

// Put stores data and returns the public URL.
// key is the object path, e.g. "media/bot-id/msg-123/0.jpg"
func (s *Storage) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return "", fmt.Errorf("storage: put %s: %w", key, err)
	}
	return s.URL(key), nil
}

// Get retrieves an object.
func (s *Storage) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

// URL returns the public URL for a key.
func (s *Storage) URL(key string) string {
	return s.publicURL + "/" + key
}
