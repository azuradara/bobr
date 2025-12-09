package storage

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/azuradara/bobr/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("not found")

type Driver interface {
	Fetch(ctx context.Context, path string) (io.ReadCloser, int64, string, error)
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
	useSSL := strings.HasPrefix(endpoint, "https://")

	client, err := minio.New(endpointClean, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	return &S3Driver{
		client: client,
		bucket: bucket,
	}, nil
}

func (s *S3Driver) Fetch(ctx context.Context, path string) (io.ReadCloser, int64, string, error) {
	objectName := strings.TrimLeft(path, "/")

	obj, err := s.client.GetObject(ctx, s.bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, "", err
	}

	stat, err := obj.Stat()
	if err != nil {
		var errResp minio.ErrorResponse
		if errors.As(err, &errResp) {
			if errResp.Code == "NoSuchKey" {
				return nil, 0, "", ErrNotFound
			}
		}
		return nil, 0, "", err
	}

	return obj, stat.Size, stat.ContentType, nil
}

func NewDriver(cfg config.OriginConfig) (Driver, error) {
	return NewS3Driver(cfg.Config)
}
