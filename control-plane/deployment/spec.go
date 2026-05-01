package deployment

// DeploymentSpec is the JSON body from the Python SDK / CLI for one web endpoint.
type DeploymentSpec struct {
	AppName          string `json:"app_name"`
	EndpointName     string `json:"endpoint_name"`
	EntryType        string `json:"entry_type"`
	EntryClass       string `json:"entry_class"`
	EntryMethod      string `json:"entry_method"`
	EnterMethod      string `json:"enter_method"`
	Code             string `json:"code"`
	Requirements     string `json:"requirements"`
	SetupScript      string `json:"setup_script"`
	Hardware         specHW `json:"hardware"`
	Web              specWeb `json:"web"`
	ScaledownWindow  int    `json:"scaledown_window"`
}

type specHW struct {
	Type      string `json:"type"`
	GPUModel  string `json:"gpu_model"`
	Memory    int    `json:"memory"`
	CPUTarget string `json:"cpu_target"`
}

type specWeb struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// ServeRequest is the JSON body for POST /serve on the workload daemon.
type ServeRequest struct {
	Code         string `json:"code"`
	Requirements string `json:"requirements"`
	SetupScript  string `json:"setup_script"`
	EntryType    string `json:"entry_type"`
	EntryClass   string `json:"entry_class"`
	EntryMethod  string `json:"entry_method"`
	EnterMethod  string `json:"enter_method"`
	Port         int    `json:"port"`
	WebMethod    string `json:"web_method"`
	WebPath      string `json:"web_path"`
}
