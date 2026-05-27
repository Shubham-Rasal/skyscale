package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ArtifactStore handles S3-compatible or local filesystem artifact storage.
type ArtifactStore struct {
	client    *minio.Client
	bucket    string
	s3Enabled bool
	localRoot string
	localOn   bool
}

// NewArtifactStore creates an ArtifactStore from env vars.
// Uses S3 when configured, otherwise falls back to ARTIFACT_LOCAL_DIR (default /opt/skyscale/artifacts).
func NewArtifactStore() *ArtifactStore {
	endpoint := os.Getenv("S3_ENDPOINT")
	bucket := os.Getenv("S3_BUCKET")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")

	if endpoint != "" && bucket != "" && accessKey != "" && secretKey != "" {
		useSSL := os.Getenv("S3_USE_SSL") != "false"
		client, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: useSSL,
		})
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			exists, _ := client.BucketExists(ctx, bucket)
			if !exists {
				_ = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
			}
			return &ArtifactStore{client: client, bucket: bucket, s3Enabled: true}
		}
	}

	localRoot := os.Getenv("ARTIFACT_LOCAL_DIR")
	if localRoot == "" {
		localRoot = "/opt/skyscale/artifacts"
	}
	_ = os.MkdirAll(localRoot, 0o755)
	return &ArtifactStore{localRoot: localRoot, localOn: true}
}

func (s *ArtifactStore) enabled() bool { return s.s3Enabled || s.localOn }

func (s *ArtifactStore) objectKey(executionID, filename string) string {
	return "executions/" + executionID + "/" + filename
}

func (s *ArtifactStore) localPath(executionID, filename string) string {
	return filepath.Join(s.localRoot, executionID, filename)
}

// uploadArtifactHandler handles POST /api/executions/{id}/artifacts (multipart from container).
func (h *APIHandler) uploadArtifactHandler(w http.ResponseWriter, r *http.Request) {
	if !h.artifactStore.enabled() {
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

	if h.artifactStore.s3Enabled {
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
	} else {
		dir := filepath.Join(h.artifactStore.localRoot, executionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			http.Error(w, "mkdir failed", http.StatusInternalServerError)
			return
		}
		dest := filepath.Join(dir, header.Filename)
		out, err := os.Create(dest)
		if err != nil {
			http.Error(w, "create failed", http.StatusInternalServerError)
			return
		}
		if _, err := io.Copy(out, file); err != nil {
			out.Close()
			http.Error(w, "write failed", http.StatusInternalServerError)
			return
		}
		out.Close()
	}

	h.logger.Infof("artifact stored: %s/%s", executionID, header.Filename)
	w.WriteHeader(http.StatusCreated)
}

// listArtifactsHandler handles GET /api/executions/{id}/artifacts.
func (h *APIHandler) listArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := mux.Vars(r)["id"]
	var names []string

	if h.artifactStore.s3Enabled {
		prefix := "executions/" + executionID + "/"
		for obj := range h.artifactStore.client.ListObjects(r.Context(), h.artifactStore.bucket, minio.ListObjectsOptions{Prefix: prefix}) {
			if obj.Err != nil {
				continue
			}
			names = append(names, obj.Key[len(prefix):])
		}
	} else if h.artifactStore.localOn {
		dir := filepath.Join(h.artifactStore.localRoot, executionID)
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					names = append(names, e.Name())
				}
			}
		}
	}

	if names == nil {
		names = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

// downloadArtifactHandler handles GET /api/executions/{id}/artifacts/{filename}
func (h *APIHandler) downloadArtifactHandler(w http.ResponseWriter, r *http.Request) {
	if !h.artifactStore.enabled() {
		http.Error(w, "artifact storage not configured", http.StatusServiceUnavailable)
		return
	}
	vars := mux.Vars(r)
	executionID := vars["id"]
	filename := vars["filename"]

	if h.artifactStore.s3Enabled {
		key := h.artifactStore.objectKey(executionID, filename)
		url, err := h.artifactStore.client.PresignedGetObject(r.Context(), h.artifactStore.bucket, key, time.Hour, nil)
		if err != nil {
			http.Error(w, "presign failed", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, url.String(), http.StatusTemporaryRedirect)
		return
	}

	path := h.artifactStore.localPath(executionID, filename)
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	io.Copy(w, f)
}
