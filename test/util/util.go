package util

import (
	"context"
	"github.com/go-logr/logr"

	"time"

	o "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	configclientv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	corev1 "k8s.io/api/core/v1"
	kapierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// LoadRestConfig loads Kube Config
func LoadRestConfig() (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
}

// IsTechPreviewNoUpgrade checks if a cluster is a TechPreviewNoUpgrade cluster
func IsTechPreviewNoUpgrade(c *configclientv1.ConfigV1Client) bool {
	featureGate, err := c.FeatureGates().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		if kapierrs.IsNotFound(err) {
			return false
		}
		o.Expect(err).NotTo(o.HaveOccurred(), "could not retrieve feature-gate: %v", err)
	}
	return featureGate.Spec.FeatureSet == configv1.TechPreviewNoUpgrade
}

// IsHypershift checks if a cluster is a Hypershift cluster
func IsHypershift(c *configclientv1.ConfigV1Client) (bool, error) {
	infrastructure, err := c.Infrastructures().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return infrastructure.Status.ControlPlaneTopology == configv1.ExternalTopologyMode, nil

}

// IsMicroShiftCluster returns "true" if a cluster is MicroShift,
// "false" otherwise. It needs kube-admin client as input.
func IsMicroShiftCluster(kubeClient kubernetes.Interface, logger logr.Logger) (bool, error) {
	ctx := context.Background()
	var cm *corev1.ConfigMap
	duration := 5 * time.Minute
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, duration, true, func(ctx context.Context) (bool, error) {
		// MicroShift cluster contains "microshift-version" configmap in "kube-public" namespace
		var err error
		cm, err = kubeClient.CoreV1().ConfigMaps("kube-public").Get(ctx, "microshift-version", metav1.GetOptions{})
		if err == nil {
			return true, nil
		}
		if kapierrs.IsNotFound(err) {
			cm = nil
			return true, nil
		}
		logger.Error(err, "error accessing microshift-version configmap")
		return false, nil
	}); err != nil {
		logger.WithValues("duration", duration).Error(err, "failed to find microshift-version configmap")
		return false, err
	}
	if cm == nil {
		logger.Info("microshift-version configmap not found")
		return false, nil
	}
	logger.WithValues("version", cm.Data["version"]).Info("MicroShift cluster has version in ConfigMap")
	return true, nil
}
