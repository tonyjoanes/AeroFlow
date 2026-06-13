// Package k8s wraps client-go with the small set of read and write operations
// the AeroFlow platform API needs. It detects whether it's running in-cluster
// or out-of-cluster automatically so the same binary works locally (with a
// kubeconfig) and inside Kubernetes.
package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps the Kubernetes clientset.
type Client struct {
	cs kubernetes.Interface
}

// New returns a Client using in-cluster config when running inside a pod, or
// the default kubeconfig (~/.kube/config) when running locally.
func New() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig for local development.
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build kubeconfig: %w", err)
		}
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &Client{cs: cs}, nil
}

// ServiceSummary holds the rolled-up state of a namespace for the UI and API.
type ServiceSummary struct {
	Namespace   string           `json:"namespace"`
	Deployments []DeploymentInfo `json:"deployments"`
}

// DeploymentInfo is a trimmed view of a Kubernetes Deployment.
type DeploymentInfo struct {
	Name      string `json:"name"`
	Image     string `json:"image"`
	Ready     int32  `json:"ready"`
	Desired   int32  `json:"desired"`
	Available bool   `json:"available"`
}

// AeroFlowNamespaces is the set of namespaces we own.
var AeroFlowNamespaces = []string{
	"flights", "gates", "baggage", "ground-ops", "notifications", "platform",
}

// ListServices returns a deployment summary for every AeroFlow namespace.
func (c *Client) ListServices(ctx context.Context) ([]ServiceSummary, error) {
	summaries := make([]ServiceSummary, 0, len(AeroFlowNamespaces))

	for _, ns := range AeroFlowNamespaces {
		deps, err := c.cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list deployments in %s: %w", ns, err)
		}

		infos := make([]DeploymentInfo, 0, len(deps.Items))
		for _, d := range deps.Items {
			infos = append(infos, deploymentInfo(d))
		}

		summaries = append(summaries, ServiceSummary{
			Namespace:   ns,
			Deployments: infos,
		})
	}

	return summaries, nil
}

// AggregateHealth returns "healthy" if every deployment in every AeroFlow
// namespace has all desired replicas available, otherwise "degraded".
func (c *Client) AggregateHealth(ctx context.Context) (string, error) {
	summaries, err := c.ListServices(ctx)
	if err != nil {
		return "", err
	}

	for _, s := range summaries {
		for _, d := range s.Deployments {
			if !d.Available {
				return "degraded", nil
			}
		}
	}
	return "healthy", nil
}

// ListPods returns all pods in namespace.
func (c *Client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	list, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods in %s: %w", namespace, err)
	}
	return list.Items, nil
}

// RolloutRequest describes a deployment image update.
type RolloutRequest struct {
	Namespace  string `json:"namespace"`
	Deployment string `json:"deployment"`
	Image      string `json:"image"`
}

// PatchImage updates the first container image on a Deployment using a
// strategic merge patch — the minimal, safe way to trigger a rollout.
func (c *Client) PatchImage(ctx context.Context, req RolloutRequest) error {
	patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":%q,"image":%q}]}}}}`,
		req.Deployment, req.Image)

	_, err := c.cs.AppsV1().Deployments(req.Namespace).Patch(
		ctx,
		req.Deployment,
		types.StrategicMergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patch deployment %s/%s: %w", req.Namespace, req.Deployment, err)
	}
	return nil
}

func deploymentInfo(d appsv1.Deployment) DeploymentInfo {
	image := ""
	if len(d.Spec.Template.Spec.Containers) > 0 {
		image = d.Spec.Template.Spec.Containers[0].Image
	}
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	return DeploymentInfo{
		Name:      d.Name,
		Image:     image,
		Ready:     d.Status.ReadyReplicas,
		Desired:   desired,
		Available: d.Status.ReadyReplicas == desired,
	}
}
