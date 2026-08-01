package pkg

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesClient wraps the kubernetes client for easier testing
type KubernetesClient struct {
	clientset kubernetes.Interface
}

// NewKubernetesClient creates a new Kubernetes client
func NewKubernetesClient(config *rest.Config) (*KubernetesClient, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &KubernetesClient{clientset: clientset}, nil
}

// listIngressesInNamespace lists ingress resources in a specific namespace
func (c *KubernetesClient) listIngressesInNamespace(ctx context.Context, namespace string) ([]runtime.Object, error) {
	ingressList, err := c.clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ingresses in namespace %s: %w", namespace, err)
	}

	objects := make([]runtime.Object, len(ingressList.Items))
	for i, ingress := range ingressList.Items {
		objects[i] = toPartialObjectMetadata(ingress.ObjectMeta)
	}

	return objects, nil
}

// listIngressesInAllNamespaces lists ingress resources across all namespaces
func (c *KubernetesClient) listIngressesInAllNamespaces(ctx context.Context) ([]runtime.Object, error) {
	ingressList, err := c.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ingresses in all namespaces: %w", err)
	}

	objects := make([]runtime.Object, len(ingressList.Items))
	for i, ingress := range ingressList.Items {
		objects[i] = toPartialObjectMetadata(ingress.ObjectMeta)
	}

	return objects, nil
}

// LoadContextConfigs resolves a rest.Config per kubeconfig context so the
// analyzer can enumerate Ingress resources "across all contexts" as required
// by PLAN.md, not just the current one. If kubeconfigPath is empty (no
// kubeconfig file present), it falls back to in-cluster config as a single
// pseudo-context named "in-cluster". If only is non-empty, it restricts the
// result to those context names instead of every context in the file.
func LoadContextConfigs(kubeconfigPath string, only []string) (map[string]*rest.Config, error) {
	if kubeconfigPath == "" {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("no kubeconfig file found and not running in-cluster: %w", err)
		}
		return map[string]*rest.Config{"in-cluster": cfg}, nil
	}

	rawConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig %s: %w", kubeconfigPath, err)
	}

	names := only
	if len(names) == 0 {
		for name := range rawConfig.Contexts {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	if len(names) == 0 {
		return nil, fmt.Errorf("kubeconfig %s defines no contexts", kubeconfigPath)
	}

	configs := make(map[string]*rest.Config, len(names))
	for _, name := range names {
		if _, ok := rawConfig.Contexts[name]; !ok {
			return nil, fmt.Errorf("context %q not found in kubeconfig %s", name, kubeconfigPath)
		}

		clientConfig := clientcmd.NewNonInteractiveClientConfig(
			*rawConfig,
			name,
			&clientcmd.ConfigOverrides{CurrentContext: name},
			clientcmd.NewDefaultClientConfigLoadingRules(),
		)

		restConfig, err := clientConfig.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build client config for context %q: %w", name, err)
		}
		configs[name] = restConfig
	}

	return configs, nil
}

// toPartialObjectMetadata extracts the metadata analysis needs (name, namespace, annotations)
// so callers work against a single concrete type regardless of the underlying resource.
func toPartialObjectMetadata(meta metav1.ObjectMeta) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		ObjectMeta: meta,
	}
}
