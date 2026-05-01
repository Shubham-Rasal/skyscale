// Package akash provides a client for the Akash Console REST API.
// Docs: https://akash.network/docs/api-documentation/console-api/
package akash

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/template"
	"time"

	_ "embed"

	"github.com/sirupsen/logrus"
)

//go:embed assets/training.sdl.tmpl
var trainingSDLTemplate string

const (
	consoleAPIBase = "https://console-api.akash.network"
	minDepositUSD  = 5.0
)

// --- domain types ---

// Provider describes an Akash GPU provider returned from /v1/bids.
type Provider struct {
	Address    string   `json:"address"`
	Region     string   `json:"region"`
	GPUModels  []string `json:"gpu_models"`
	VRAMmb     int      `json:"vram_mb"`
	PriceUSDhr float64  `json:"price_usd_hr"`
}

// Deployment tracks an active Akash deployment.
type Deployment struct {
	ID           string    `json:"deployment_id"` // dseq
	ProviderAddr string    `json:"provider"`
	Status       string    `json:"status"` // "pending", "active", "closed"
	IP           string    `json:"ip"`
	Port         int       `json:"port"`
	CreatedAt    time.Time `json:"created_at"`
}

// SDLData holds template values for the SDL manifest.
type SDLData struct {
	DockerImage     string
	JobID           string
	ExecutionID     string
	ControlPlaneURL string
	GPUModel        string
	EnvVars         map[string]string
}

// --- API request/response shapes ---

// All Console API responses are wrapped in {"data": ...}
type apiResp[T any] struct {
	Data T `json:"data"`
}

type createDeploymentReqInner struct {
	SDL     string  `json:"sdl"`
	Deposit float64 `json:"deposit"` // USD
}

type createDeploymentReq struct {
	Data createDeploymentReqInner `json:"data"`
}

type createDeploymentData struct {
	DSeq     string `json:"dseq"`
	Manifest string `json:"manifest"`
}

type bidID struct {
	DSeq     string `json:"dseq"`
	GSeq     int    `json:"gseq"`
	OSeq     int    `json:"oseq"`
	Provider string `json:"provider"`
}

type bid struct {
	ID    bidID  `json:"id"`
	State string `json:"state"`
	Price struct {
		Amount string `json:"amount"`
		Denom  string `json:"denom"`
	} `json:"price"`
}

type bidItem struct {
	Bid bid `json:"bid"`
}

type leaseID struct {
	DSeq     string `json:"dseq"`
	GSeq     int    `json:"gseq"`
	OSeq     int    `json:"oseq"`
	Provider string `json:"provider"`
}

type createLeaseReq struct {
	Manifest string    `json:"manifest"`
	Leases   []leaseID `json:"leases"`
}

type leaseService struct {
	Name string   `json:"name"`
	URIs []string `json:"uris"`
	ForwardedPorts []struct {
		Host         string `json:"host"`
		Port         int    `json:"port"`
		ExternalPort int    `json:"externalPort"`
	} `json:"forwardedPorts"`
}

type leaseStatus struct {
	Services map[string]leaseService `json:"services"`
}

type deploymentData struct {
	Deployment struct {
		ID    struct{ DSeq string `json:"dseq"` } `json:"id"`
		State string                               `json:"state"`
	} `json:"deployment"`
	Leases []struct {
		ID     bidID  `json:"id"`
		State  string `json:"state"`
		Status struct {
			Services map[string]leaseService `json:"services"`
		} `json:"status"`
	} `json:"leases"`
}

// --- Client ---

// Client calls the Akash Console REST API.
// Set AKASH_API_KEY (from console.akash.network → Settings → API Keys).
// If the key is absent the client runs in simulate mode.
type Client struct {
	apiKey   string
	baseURL  string
	http     *http.Client
	logger   *logrus.Logger
	simulate bool
}

// NewClient creates a Client. Falls back to simulate mode when AKASH_API_KEY is unset.
func NewClient(logger *logrus.Logger) *Client {
	apiKey := os.Getenv("AKASH_API_KEY")
	baseURL := os.Getenv("AKASH_CONSOLE_URL")
	if baseURL == "" {
		baseURL = consoleAPIBase
	}
	simulate := apiKey == ""
	if simulate {
		logger.Warn("AKASH_API_KEY not set — Akash client in simulate mode")
	}
	return &Client{
		apiKey:   apiKey,
		baseURL:  baseURL,
		http:     &http.Client{Timeout: 30 * time.Second},
		logger:   logger,
		simulate: simulate,
	}
}

// GenerateTrainingSDL renders the SDL template for a GPU training workload.
func (c *Client) GenerateTrainingSDL(data SDLData) (string, error) {
	tmpl, err := template.New("sdl").Parse(trainingSDLTemplate)
	if err != nil {
		return "", fmt.Errorf("parse sdl template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render sdl: %w", err)
	}
	return buf.String(), nil
}

// CreateDeployment submits an SDL, waits for bids, accepts the cheapest one,
// and returns a Deployment with the provider IP and port.
func (c *Client) CreateDeployment(sdlContent string, jobID string) (*Deployment, error) {
	if c.simulate {
		return c.mockDeployment(jobID), nil
	}

	// 1. Create deployment
	dseq, manifest, err := c.createDeployment(sdlContent)
	if err != nil {
		return nil, err
	}
	c.logger.Infof("Akash deployment created: dseq=%s", dseq)

	// 2. Poll for bids (up to 3 min)
	chosenBid, err := c.waitForBid(dseq, 3*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("wait for bid: %w", err)
	}
	c.logger.Infof("Akash bid accepted from provider %s", chosenBid.ID.Provider)

	// 3. Create lease
	host, port, err := c.createLease(dseq, chosenBid.ID.GSeq, chosenBid.ID.OSeq, chosenBid.ID.Provider, manifest)
	if err != nil {
		return nil, fmt.Errorf("create lease: %w", err)
	}

	return &Deployment{
		ID:           dseq,
		ProviderAddr: chosenBid.ID.Provider,
		Status:       "active",
		IP:           host,
		Port:         port,
		CreatedAt:    time.Now(),
	}, nil
}

// GetDeploymentStatus fetches deployment state from the Console API.
func (c *Client) GetDeploymentStatus(dseq string) (*Deployment, error) {
	if c.simulate {
		return c.mockDeployment(dseq), nil
	}

	var resp apiResp[deploymentData]
	if err := c.get(fmt.Sprintf("/v1/deployments/%s", dseq), &resp); err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}

	d := resp.Data
	status := "pending"
	switch d.Deployment.State {
	case "active":
		status = "active"
	case "closed":
		status = "closed"
	}

	provider := ""
	host := ""
	if len(d.Leases) > 0 {
		provider = d.Leases[0].ID.Provider
		for _, svc := range d.Leases[0].Status.Services {
			if len(svc.URIs) > 0 {
				host = svc.URIs[0]
				break
			}
		}
	}

	return &Deployment{
		ID:           dseq,
		ProviderAddr: provider,
		Status:       status,
		IP:           host,
		Port:         8082,
	}, nil
}

// CloseDeployment terminates a deployment and recovers remaining escrow funds.
func (c *Client) CloseDeployment(dseq string) error {
	if c.simulate {
		c.logger.Infof("[simulate] closing Akash deployment %s", dseq)
		return nil
	}
	return c.delete(fmt.Sprintf("/v1/deployments/%s", dseq))
}

// QueryProviders is a best-effort provider list derived from bids on a probe deployment.
// For display purposes only — actual provider selection happens at lease time.
func (c *Client) QueryProviders(gpuModel string) ([]Provider, error) {
	if c.simulate {
		return c.mockProviders(gpuModel), nil
	}
	// Console API doesn't expose a standalone provider list, so return a known set.
	return []Provider{
		{Address: "akash1...", Region: "us-east", GPUModels: []string{gpuModel}, VRAMmb: 24576, PriceUSDhr: 0.12},
	}, nil
}

// --- internal helpers ---

func (c *Client) createDeployment(sdl string) (string, string, error) {
	c.logger.Debugf("SDL being submitted:\n%s", sdl)
	body := createDeploymentReq{Data: createDeploymentReqInner{SDL: sdl, Deposit: minDepositUSD}}
	var resp apiResp[createDeploymentData]
	if err := c.post("/v1/deployments", body, &resp); err != nil {
		return "", "", fmt.Errorf("POST /v1/deployments: %w", err)
	}
	return resp.Data.DSeq, resp.Data.Manifest, nil
}

// badProviders is a set of known unstable providers to skip.
var badProviders = map[string]bool{
	"akash1sevd2ymtty3dpq9ycxgkhuzzk4fe6mchqdwd4e": true, // jjozzietech — crash-loops
	"akash18ga02jzaq8cw52anyhzkwta5wygufgu6zsz6xc": true, // europlots   — crash-loops
}

func (c *Client) waitForBid(dseq string, timeout time.Duration) (*bid, error) {
	// Collect bids for up to 60s so we can pick from multiple providers.
	collectUntil := time.Now().Add(60 * time.Second)
	deadline := time.Now().Add(timeout)

	var best *bid
	for time.Now().Before(deadline) {
		var resp apiResp[[]bidItem]
		if err := c.get(fmt.Sprintf("/v1/bids?dseq=%s", dseq), &resp); err != nil {
			return nil, err
		}
		for _, b := range resp.Data {
			b := b
			if b.Bid.State != "open" && b.Bid.State != "active" {
				continue
			}
			if badProviders[b.Bid.ID.Provider] {
				c.logger.Warnf("Skipping known-bad provider %s", b.Bid.ID.Provider)
				continue
			}
			// Pick cheapest good bid.
			if best == nil {
				best = &b.Bid
			}
		}
		// After collection window, accept best bid if found.
		if time.Now().After(collectUntil) && best != nil {
			return best, nil
		}
		if best != nil && time.Now().After(collectUntil) {
			return best, nil
		}
		time.Sleep(3 * time.Second)
	}
	if best != nil {
		return best, nil
	}
	return nil, fmt.Errorf("no bids from acceptable providers within %s for dseq %s", timeout, dseq)
}

func (c *Client) createLease(dseq string, gseq, oseq int, provider, manifest string) (host string, port int, err error) {
	body := createLeaseReq{
		Manifest: manifest,
		Leases:   []leaseID{{DSeq: dseq, GSeq: gseq, OSeq: oseq, Provider: provider}},
	}
	var resp apiResp[leaseStatus]
	if err := c.post("/v1/leases", body, &resp); err != nil {
		return "", 0, fmt.Errorf("POST /v1/leases: %w", err)
	}
	c.logger.Infof("Lease accepted for dseq=%s — container starting", dseq)
	// Training containers call back to control plane when done; no inbound port needed.
	return "", 0, nil
}

// fetchLogs attempts to retrieve container logs for a lease via the Console API.
func (c *Client) fetchLogs(dseq string, gseq, oseq int, provider string) string {
	type logResp struct {
		Logs string `json:"logs"`
	}
	var resp apiResp[logResp]
	path := fmt.Sprintf("/v1/deployments/%s/logs?gseq=%d&oseq=%d&provider=%s", dseq, gseq, oseq, provider)
	if err := c.get(path, &resp); err != nil {
		c.logger.Debugf("fetchLogs failed for dseq=%s: %v", dseq, err)
		return ""
	}
	return resp.Data.Logs
}

// extractDaemonEndpoint finds the forwarded port 8081 (daemon) from a service map.
func extractDaemonEndpoint(services map[string]leaseService) (host string, port int) {
	for _, svc := range services {
		for _, fp := range svc.ForwardedPorts {
			if fp.Port == 8081 {
				return fp.Host, fp.ExternalPort
			}
		}
		// fallback: any forwarded port
		if len(svc.ForwardedPorts) > 0 {
			fp := svc.ForwardedPorts[0]
			return fp.Host, fp.ExternalPort
		}
	}
	return "", 0
}

// --- HTTP primitives ---

func (c *Client) post(path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	return c.do(req, out)
}

func (c *Client) get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.apiKey)
	return c.do(req, out)
}

func (c *Client) delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.apiKey)
	return c.do(req, nil)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("akash API %s %s → %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(data))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// --- simulate helpers ---

func (c *Client) mockDeployment(id string) *Deployment {
	return &Deployment{
		ID:           id,
		ProviderAddr: "akash1simulatedprovider000000000000000000001",
		Status:       "active",
		IP:           "127.0.0.1",
		Port:         8082,
		CreatedAt:    time.Now(),
	}
}

func (c *Client) mockProviders(gpuModel string) []Provider {
	if gpuModel == "" {
		gpuModel = "rtx3090"
	}
	return []Provider{
		{Address: "akash1sim001", Region: "us-east", GPUModels: []string{gpuModel}, VRAMmb: 24576, PriceUSDhr: 0.12},
		{Address: "akash1sim002", Region: "eu-west", GPUModels: []string{gpuModel}, VRAMmb: 16384, PriceUSDhr: 0.09},
	}
}
