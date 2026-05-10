package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ArtifactStore handles S3-compatible artifact storage.
type ArtifactStore struct {
	client  *minio.Client
	bucket  string
	enabled bool
}

// NewArtifactStore creates an ArtifactStore from env vars.
// Requires S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY, S3_SECRET_KEY.
func NewArtifactStore() *ArtifactStore {
	endpoint := os.Getenv("S3_ENDPOINT")
	bucket := os.Getenv("S3_BUCKET")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")

	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return &ArtifactStore{enabled: false}
	}

	useSSL := os.Getenv("S3_USE_SSL") != "false"
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return &ArtifactStore{enabled: false}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exists, _ := client.BucketExists(ctx, bucket)
	if !exists {
		_ = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	}

	return &ArtifactStore{client: client, bucket: bucket, enabled: true}
}

func (s *ArtifactStore) objectKey(executionID, filename string) string {
	return "executions/" + executionID + "/" + filename
}

// uploadArtifactHandler handles POST /api/executions/{id}/artifacts (multipart from container).
func (h *APIHandler) uploadArtifactHandler(w http.ResponseWriter, r *http.Request) {
	if !h.artifactStore.enabled {
		http.Error(w, "artifact storage not configured", http.StatusServiceUnavailable)
		return
	}
	executionID := mux.Vars(r)["id"]
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		http.Error(w, "multipart parse error", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	key := h.artifactStore.objectKey(executionID, header.Filename)
	_, err = h.artifactStore.client.PutObject(
		r.Context(), h.artifactStore.bucket, key, file, header.Size,
		minio.PutObjectOptions{ContentType: "application/octet-stream"},
	)
	if err != nil {
		h.logger.Errorf("artifact upload %s/%s: %v", executionID, header.Filename, err)
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}
	h.logger.Infof("artifact stored: %s/%s", executionID, header.Filename)
	w.WriteHeader(http.StatusCreated)
}

// listArtifactsHandler handles GET /api/executions/{id}/artifacts.
func (h *APIHandler) listArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	if !h.artifactStore.enabled {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	executionID := mux.Vars(r)["id"]
	prefix := "executions/" + executionID + "/"
	var names []string
	for obj := range h.artifactStore.client.ListObjects(r.Context(), h.artifactStore.bucket, minio.ListObjectsOptions{Prefix: prefix}) {
		if obj.Err != nil {
			continue
		}
		names = append(names, obj.Key[len(prefix):])
	}
	if names == nil {
		names = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

// downloadArtifactHandler handles GET /api/executions/{id}/artifacts/{filename}
// by redirecting to a presigned URL valid for 1 hour.
func (h *APIHandler) downloadArtifactHandler(w http.ResponseWriter, r *http.Request) {
	if !h.artifactStore.enabled {
		http.Error(w, "artifact storage not configured", http.StatusServiceUnavailable)
		return
	}
	vars := mux.Vars(r)
	key := h.artifactStore.objectKey(vars["id"], vars["filename"])
	url, err := h.artifactStore.client.PresignedGetObject(r.Context(), h.artifactStore.bucket, key, time.Hour, nil)
	if err != nil {
		http.Error(w, "presign failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, url.String(), http.StatusTemporaryRedirect)
}
