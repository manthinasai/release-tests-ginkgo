package operator

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

// tlsProfilePatch is the JSON merge-patch shape for APIServer/cluster.
type tlsProfilePatch struct {
	Spec tlsProfilePatchSpec `json:"spec"`
}

type tlsProfilePatchSpec struct {
	TLSSecurityProfile *configv1.TLSSecurityProfile `json:"tlsSecurityProfile"`
}

// GetClusterTLSProfileType reads the current tlsSecurityProfile type from
// APIServer/cluster. Returns "Intermediate" when no explicit profile is set
// (cluster default).
func GetClusterTLSProfileType(cs *clients.Clients) string {
	apiServer, err := cs.ProxyConfig.APIServers().Get(
		context.TODO(), "cluster", metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), "failed to get APIServer/cluster")
	if apiServer.Spec.TLSSecurityProfile == nil {
		return config.TLSProfileIntermediate
	}
	return string(apiServer.Spec.TLSSecurityProfile.Type)
}

// PatchClusterTLSProfile patches APIServer/cluster to the given profileType
// ("Modern" or "Intermediate"). Any other value resets the profile to nil
// (cluster default).
func PatchClusterTLSProfile(cs *clients.Clients, profileType string) {
	var profile *configv1.TLSSecurityProfile
	switch profileType {
	case config.TLSProfileModern:
		profile = &configv1.TLSSecurityProfile{
			Type:   configv1.TLSProfileModernType,
			Modern: &configv1.ModernTLSProfile{},
		}
	case config.TLSProfileIntermediate:
		profile = &configv1.TLSSecurityProfile{
			Type:         configv1.TLSProfileIntermediateType,
			Intermediate: &configv1.IntermediateTLSProfile{},
		}
	case config.TLSProfileOld:
		profile = &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileOldType,
			Old:  &configv1.OldTLSProfile{},
		}
	default:
		profile = nil
	}

	patch := tlsProfilePatch{}
	patch.Spec.TLSSecurityProfile = profile
	patchBytes, err := json.Marshal(patch)
	Expect(err).NotTo(HaveOccurred(), "failed to marshal TLS profile patch")

	log.Printf("Patching APIServer/cluster TLS profile to %q", profileType)
	_, err = cs.ProxyConfig.APIServers().Patch(
		context.TODO(), "cluster",
		types.MergePatchType, patchBytes,
		metav1.PatchOptions{})
	Expect(err).NotTo(HaveOccurred(),
		"failed to patch APIServer/cluster TLS profile to %q", profileType)
}

// AssertDeploymentHasEnvVar polls a Deployment until all containers carry the
// env var envName=expectedValue, or the APITimeout is reached.
// This is the primary assertion for Go-based components that receive TLS
// settings via environment variables injected by the Tekton Operator.
func AssertDeploymentHasEnvVar(cs *clients.Clients, ns, deploymentName, envName, expectedValue string) {
	err := wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, true,
		func(context.Context) (bool, error) {
			deployment, err := cs.KubeClient.Kube.AppsV1().Deployments(ns).Get(
				context.TODO(), deploymentName, metav1.GetOptions{})
			if err != nil {
				log.Printf("Waiting for deployment %s: %v", deploymentName, err)
				return false, nil
			}
			for _, container := range deployment.Spec.Template.Spec.Containers {
				found := false
				for _, env := range container.Env {
					if env.Name == envName {
						if env.Value == expectedValue {
							found = true
						} else {
							log.Printf("Deployment %s container %s: %s=%s (want %s)",
								deploymentName, container.Name, envName, env.Value, expectedValue)
							return false, nil
						}
					}
				}
				if !found {
					log.Printf("Deployment %s container %s: env var %s not yet present",
						deploymentName, container.Name, envName)
					return false, nil
				}
			}
			log.Printf("Deployment %s: confirmed %s=%s on all containers", deploymentName, envName, expectedValue)
			return true, nil
		})
	Expect(err).NotTo(HaveOccurred(),
		"deployment %s/%s does not have %s=%s after waiting",
		ns, deploymentName, envName, expectedValue)
}

// AssertNginxConfigMapHasTLSProfile polls a ConfigMap that holds an nginx
// configuration and checks that the ssl_protocols directive matches the
// expected TLS profile.
//
// Modern      → ssl_protocols must include "TLSv1.3" exclusively.
// Intermediate → ssl_protocols must include both "TLSv1.2" and "TLSv1.3".
//
// NOTE: update config.NginxConsolePluginConfigMap with the exact ConfigMap
// name once confirmed from a live cluster.
func AssertNginxConfigMapHasTLSProfile(cs *clients.Clients, ns, configMapName, profileType string) {
	var requiredProtocols []string
	switch profileType {
	case config.TLSProfileModern:
		requiredProtocols = []string{"TLSv1.3"}
	case config.TLSProfileIntermediate:
		requiredProtocols = []string{"TLSv1.2", "TLSv1.3"}
	case config.TLSProfileOld:
		requiredProtocols = []string{"TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"}
	default:
		requiredProtocols = []string{"TLSv1.2", "TLSv1.3"}
	}

	err := wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, true,
		func(context.Context) (bool, error) {
			cm, err := cs.KubeClient.Kube.CoreV1().ConfigMaps(ns).Get(
				context.TODO(), configMapName, metav1.GetOptions{})
			if err != nil {
				log.Printf("Waiting for ConfigMap %s: %v", configMapName, err)
				return false, nil
			}
			for key, value := range cm.Data {
				allFound := true
				for _, proto := range requiredProtocols {
					if !strings.Contains(value, proto) {
						allFound = false
						break
					}
				}
				if allFound {
					log.Printf("ConfigMap %s key %q: ssl_protocols contains required protocols %v",
						configMapName, key, requiredProtocols)
					return true, nil
				}
			}
			log.Printf("Waiting for ConfigMap %s to contain ssl_protocols %v for profile %s",
				configMapName, requiredProtocols, profileType)
			return false, nil
		})
	Expect(err).NotTo(HaveOccurred(),
		"nginx ConfigMap %s/%s does not reflect TLS profile %s (required ssl_protocols: %v)",
		ns, configMapName, profileType, requiredProtocols)
}
