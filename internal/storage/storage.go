package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/azuradara/bobr/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("not found")

type Object struct {
	Body         io.ReadSeekCloser
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

type Driver interface {
	Fetch(ctx context.Context, path string) (*Object, error)
}

type S3Driver struct {
	client *minio.Client
	bucket string
}

func NewS3Driver(conf map[string]string) (*S3Driver, error) {
	endpoint := conf["endpoint"]
	accessKey := conf["access_key"]
	secretKey := conf["secret_key"]
	bucket := conf["bucket"]

	endpointClean := strings.TrimPrefix(endpoint, "http://")
	endpointClean = strings.TrimPrefix(endpointClean, "https://")
	endpointClean = strings.TrimSuffix(endpointClean, "/")
	useSSL := strings.HasPrefix(endpoint, "https://")

	client, err := minio.New(endpointClean, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: conf["region"],
	})
	if err != nil {
		return nil, err
	}

	return &S3Driver{
		client: client,
		bucket: bucket,
	}, nil
}

func (s *S3Driver) Fetch(ctx context.Context, path string) (*Object, error) {
	obj, err := s.client.GetObject(
		ctx,
		s.bucket,
		strings.TrimLeft(path, "/"),
		minio.GetObjectOptions{},
	)
	if err != nil {
		return nil, err
	}

	stat, err := obj.Stat()
	if err != nil {
		_ = obj.Close()

		if isNotFound(err) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &Object{
		Body:         obj,
		Size:         stat.Size,
		ContentType:  stat.ContentType,
		ETag:         stat.ETag,
		LastModified: stat.LastModified,
	}, nil
}

func isNotFound(err error) bool {
	var errResp minio.ErrorResponse
	if !errors.As(err, &errResp) {
		return false
	}

	return errResp.StatusCode == http.StatusNotFound ||
		errResp.Code == "NoSuchKey" ||
		errResp.Code == "NoSuchBucket"
}

func NewDriver(cfg config.OriginConfig) (Driver, error) {
	switch cfg.Type {
	case "", "s3":
		return NewS3Driver(cfg.Config)
	default:
		return nil, fmt.Errorf("unknown origin type %q for origin %s", cfg.Type, cfg.Name)
	}
}
