package pkg

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// toPartialObjectMetadata extracts the metadata analysis needs (name, namespace, annotations)
// so callers work against a single concrete type regardless of the underlying resource.
func toPartialObjectMetadata(meta metav1.ObjectMeta) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		ObjectMeta: meta,
	}
}
