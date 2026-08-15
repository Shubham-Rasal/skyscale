package weights

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
)

type MinioArtifactStore struct {
	client *minio.Client
	bucket string
}

func NewMinioArtifactStore(client *minio.Client, bucket string) (*MinioArtifactStore, error) {
	if client == nil || bucket == "" {
		return nil, errors.New("MinIO client and bucket are required")
	}
	return &MinioArtifactStore{client: client, bucket: bucket}, nil
}

func (s *MinioArtifactStore) key(uri string) string {
	prefix := "s3://" + s.bucket + "/"
	return strings.TrimPrefix(uri, prefix)
}

func (s *MinioArtifactStore) Fetch(ctx context.Context, uri string) ([]byte, error) {
	object, err := s.client.GetObject(ctx, s.bucket, s.key(uri), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	return io.ReadAll(object)
}

func (s *MinioArtifactStore) PutImmutable(ctx context.Context, uri string, payload []byte, expectedChecksum string) error {
	key := s.key(uri)
	if stat, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err == nil {
		if stat.Metadata.Get("X-Amz-Meta-Sha256") != expectedChecksum {
			return errors.New("immutable artifact conflict")
		}
		return nil
	} else {
		response := minio.ToErrorResponse(err)
		if response.Code != "NoSuchKey" && response.Code != "NoSuchObject" && response.StatusCode != 404 {
			return err
		}
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{
		ContentType: "application/octet-stream", UserMetadata: map[string]string{"sha256": expectedChecksum},
	})
	return err
}

func (s *MinioArtifactStore) Delete(ctx context.Context, uri string) error {
	return s.client.RemoveObject(ctx, s.bucket, s.key(uri), minio.RemoveObjectOptions{})
}
