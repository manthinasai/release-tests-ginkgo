// Package openshift provides helpers for interacting with OpenShift-specific resources.
package openshift

import (
	"log"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega
	imageStream "github.com/openshift/client-go/image/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
)

// IsHostedCluster returns true when the cluster runs a hosted control plane
// (HyperShift / ROSA HCP, controlPlaneTopology == "External"). On such clusters
// the control-plane configuration (APIServer/cluster, OAuth identity providers,
// ...) is owned by the HostedCluster on the management cluster and cannot be
// changed from inside the guest cluster.
//
// Uses the oc CLI rather than a Go API client so that it works in environments
// where the test process network is restricted (e.g. sandboxed runners).
func IsHostedCluster() bool {
	result := cmd.Run("oc", "get", "infrastructure", "cluster",
		"-o", "jsonpath={.status.controlPlaneTopology}")
	if result.ExitCode != 0 {
		log.Printf("Warning: could not get infrastructure/cluster topology: %s", result.Combined())
		return false
	}
	topology := strings.TrimSpace(result.Stdout())
	log.Printf("Cluster controlPlaneTopology: %q", topology)
	return topology == "External"
}

// GetImageStreamTags returns the list of tag names for the given imagestream.
func GetImageStreamTags(c *clients.Clients, namespace, name string) []string {
	log.Printf("Getting imagestream %s from namespace %s", name, namespace)
	is := imageStream.NewForConfigOrDie(c.KubeConfig)
	isRequired, err := is.ImageV1().ImageStreams(namespace).Get(c.Ctx, name, metav1.GetOptions{})
	if err != nil {
		Fail("failed to get imagestream " + name + ": " + err.Error())
	}
	tags := isRequired.Spec.Tags
	tagNames := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagNames = append(tagNames, tag.Name)
	}
	return tagNames
}

// IsCapabilityEnabled checks whether a given OpenShift capability (e.g. "Console")
// is enabled on the cluster.
func IsCapabilityEnabled(c *clients.Clients, name string) bool {
	cv, err := c.ClusterVersion.Get(c.Ctx, "version", metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), "failed to get ClusterVersion")

	for _, capability := range cv.Status.Capabilities.EnabledCapabilities {
		if string(capability) == name {
			return true
		}
	}
	return false
}

// GetOpenShiftVersion returns the desired OpenShift version string (e.g. "4.16.3").
func GetOpenShiftVersion(c *clients.Clients) string {
	cv, err := c.ClusterVersion.Get(c.Ctx, "version", metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), "failed to get ClusterVersion")
	return cv.Status.Desired.Version
}

// Ensure unused imports don't fire.
var _ = GinkgoWriter
