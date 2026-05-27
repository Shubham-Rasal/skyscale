package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const defaultHuggingFaceBaseURL = "https://huggingface.co"

var hfFlavorByModel = map[string]string{
	"a100":       "a100-large",
	"a100-large": "a100-large",
	"h200":       "h200",
	"t4":         "t4-medium",
	"t4-small":   "t4-small",
	"t4-medium":  "t4-medium",
	"l4":         "l4x1",
	"l4x1":       "l4x1",
	"l4x4":       "l4x4",
	"a10g":       "a10g-small",
	"a10g-small": "a10g-small",
	"a10g-large": "a10g-large",
}

var errUnsupportedHFFlavor = errors.New("unsupported Hugging Face GPU flavor")

// HuggingFaceProvider launches GPU jobs on Hugging Face Jobs.
type HuggingFaceProvider struct {
	token     string
	namespace string
	baseURL   string
	http      *http.Client
	simulate  bool
	logger    *logrus.Logger
}

func NewHuggingFaceProvider(logger *logrus.Logger) *HuggingFaceProvider {
	token := firstNonEmpty(os.Getenv("HF_TOKEN"), os.Getenv("HUGGINGFACE_TOKEN"))
	namespace := firstNonEmpty(os.Getenv("HF_NAMESPACE"), os.Getenv("HF_ORG"), os.Getenv("HF_USER"))
	baseURL := os.Getenv("HF_ENDPOINT")
	if baseURL == "" {
		baseURL = defaultHuggingFaceBaseURL
	}

	simulate := token == ""
	if simulate {
		logger.Warn("HF_TOKEN not set - Hugging Face provider in simulate mode")
	}

	return &HuggingFaceProvider{
		token:     token,
		namespace: namespace,
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      &http.Client{Timeout: 30 * time.Second},
		simulate:  simulate,
		logger:    logger,
	}
}

func (p *HuggingFaceProvider) Name() string { return "huggingface" }

func (p *HuggingFaceProvider) Deploy(ctx context.Context, spec DeploySpec) (DeployResult, error) {
	flavor, err := huggingFaceFlavor(spec.GPUModel)
	if err != nil {
		return DeployResult{}, err
	}
	if p.simulate {
		id := fmt.Sprintf("hf-sim-%s-%d", sanitizeLabelValue(spec.JobID), time.Now().UnixMilli())
		return DeployResult{
			DeploymentID: id,
			ProviderName: p.Name(),
			ProviderAddr: "huggingface://simulate/" + id,
		}, nil
	}
	if p.namespace == "" {
		return DeployResult{}, errors.New("HF_NAMESPACE, HF_ORG, or HF_USER is required for Hugging Face Jobs")
	}
	if spec.DockerImage == "" {
		return DeployResult{}, errors.New("docker image is required for Hugging Face Jobs")
	}
	controlPlaneURL := huggingFaceCallbackURL(spec.ControlPlaneURL)
	if controlPlaneURL == "" || isLocalControlPlaneURL(controlPlaneURL) {
		return DeployResult{}, errors.New("huggingface jobs require a public CONTROL_PLANE_URL; set SKYSCALE_PUBLIC_BASE or pass control_plane_url")
	}

	env := copyEnv(spec.EnvVars)
	env["SKYSCALE_JOB_ID"] = spec.JobID
	env["JOB_ID"] = spec.JobID
	env["EXECUTION_ID"] = spec.ExecutionID
	env["CONTROL_PLANE_URL"] = controlPlaneURL

	body := hfStartJobRequest{
		DockerImage:    spec.DockerImage,
		Environment:    env,
		Flavor:         flavor,
		Arch:           "amd64",
		TimeoutSeconds: hfJobTimeoutSeconds(),
		Attempts:       1,
		Labels: map[string]string{
			"skyscale_job_id":       sanitizeLabelValue(spec.JobID),
			"skyscale_execution_id": sanitizeLabelValue(spec.ExecutionID),
		},
	}

	var resp map[string]any
	path := fmt.Sprintf("/api/jobs/%s", p.namespace)
	if err := p.doJSON(ctx, http.MethodPost, path, body, &resp); err != nil {
		return DeployResult{}, err
	}

	id := firstJSONField(resp, "id", "jobId", "job_id")
	if id == "" {
		return DeployResult{}, fmt.Errorf("huggingface: start job response missing id: %v", resp)
	}
	url := firstJSONField(resp, "url")
	if url == "" {
		url = fmt.Sprintf("%s/jobs/%s/%s", p.baseURL, p.namespace, id)
	}

	return DeployResult{
		DeploymentID: id,
		ProviderName: p.Name(),
		ProviderAddr: url,
	}, nil
}

func (p *HuggingFaceProvider) Terminate(ctx context.Context, deploymentID string) error {
	if p.simulate {
		p.logger.Infof("[simulate] canceling Hugging Face job %s", deploymentID)
		return nil
	}
	if p.namespace == "" {
		return errors.New("HF_NAMESPACE, HF_ORG, or HF_USER is required for Hugging Face Jobs")
	}
	path := fmt.Sprintf("/api/jobs/%s/%s/cancel", p.namespace, deploymentID)
	return p.doJSON(ctx, http.MethodPost, path, nil, nil)
}

func (p *HuggingFaceProvider) Available(_ context.Context, gpuModel string) (int, error) {
	if _, err := huggingFaceFlavor(gpuModel); err != nil {
		return 0, err
	}
	// Hugging Face Jobs exposes available flavors, not real-time free capacity.
	return 1, nil
}

type hfStartJobRequest struct {
	DockerImage    string            `json:"dockerImage"`
	Environment    map[string]string `json:"environment,omitempty"`
	Flavor         string            `json:"flavor"`
	Arch           string            `json:"arch,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
	Attempts       int               `json:"attempts,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

func (p *HuggingFaceProvider) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body *bytes.Reader
	if in == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("huggingface: %s %s returned %s", method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func huggingFaceFlavor(gpuModel string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(gpuModel))
	if key == "" {
		key = "a10g"
	}
	if override := os.Getenv("HF_FLAVOR_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))); override != "" {
		return override, nil
	}
	flavor, ok := hfFlavorByModel[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", errUnsupportedHFFlavor, gpuModel)
	}
	return flavor, nil
}

func hfJobTimeoutSeconds() int {
	value := os.Getenv("HF_JOB_TIMEOUT_SECONDS")
	if value == "" {
		return 6 * 60 * 60
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 6 * 60 * 60
	}
	return n
}

func huggingFaceCallbackURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value != "" && !isLocalControlPlaneURL(value) {
		return value
	}
	return strings.TrimRight(firstNonEmpty(
		os.Getenv("HF_CALLBACK_BASE"),
		os.Getenv("HUGGINGFACE_CALLBACK_BASE"),
		os.Getenv("SKYSCALE_PUBLIC_BASE"),
	), "/")
}

func isLocalControlPlaneURL(value string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	u, err := url.Parse(value)
	if err != nil {
		return true
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()
}

func copyEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+3)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstJSONField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok {
			return value
		}
	}
	return ""
}

func sanitizeLabelValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	if len(out) > 100 {
		return out[:100]
	}
	return out
}
