package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	RayJobs         = schema.GroupVersionResource{Group: "ray.io", Version: "v1", Resource: "rayjobs"}
	Deployments     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	Services        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	ConfigMaps      = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	Secrets         = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	Jobs            = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	PVCs            = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}
	Namespaces      = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	ResourceQuotas  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "resourcequotas"}
	LimitRanges     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "limitranges"}
	NetworkPolicies = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	ServiceAccounts = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}
)

type Client interface {
	Apply(context.Context, schema.GroupVersionResource, *unstructured.Unstructured) (*unstructured.Unstructured, error)
	Get(context.Context, schema.GroupVersionResource, string, string) (*unstructured.Unstructured, error)
	List(context.Context, schema.GroupVersionResource, string, metav1.ListOptions) (*unstructured.UnstructuredList, error)
	Delete(context.Context, schema.GroupVersionResource, string, string, metav1.DeleteOptions) error
}

type DynamicClient struct {
	client dynamic.Interface
}

func NewDynamicClient() (*DynamicClient, error) {
	var cfg *rest.Config
	var err error
	if path := os.Getenv("KUBECONFIG"); path != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", path)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("load kubernetes configuration: %w", err)
	}
	d, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic kubernetes client: %w", err)
	}
	return &DynamicClient{client: d}, nil
}

func (c *DynamicClient) Apply(ctx context.Context, gvr schema.GroupVersionResource, object *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	raw, err := json.Marshal(object.Object)
	if err != nil {
		return nil, err
	}
	force := true
	return c.client.Resource(gvr).Namespace(object.GetNamespace()).Patch(
		ctx, object.GetName(), types.ApplyPatchType, raw,
		metav1.PatchOptions{FieldManager: "skyscale-rl-controller", Force: &force},
	)
}

func (c *DynamicClient) Get(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	return c.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *DynamicClient) List(ctx context.Context, gvr schema.GroupVersionResource, namespace string, options metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return c.client.Resource(gvr).Namespace(namespace).List(ctx, options)
}

func (c *DynamicClient) Delete(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, options metav1.DeleteOptions) error {
	return c.client.Resource(gvr).Namespace(namespace).Delete(ctx, name, options)
}
