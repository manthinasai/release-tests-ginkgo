package operator_test

import (
	"log"

	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// knativeWebhookDeployments lists deployments that use the knative webhook library and
// therefore receive WEBHOOK_TLS_MIN_VERSION (not TLS_MIN_VERSION) when the cluster TLS
// profile changes (SRVKP-11926).
var knativeWebhookDeployments = []string{
	config.PipelineWebhookName, // tekton-pipelines-webhook
	config.TriggerWebhookName,  // tekton-triggers-webhook
	config.PacWebhookName,      // pipelines-as-code-webhook
}

// tlsEnvDeployments lists non-knative Go deployments that receive the TLS_MIN_VERSION
// env var (without the WEBHOOK_ prefix) from the Tekton Operator.
//
// Deliberately excluded from both lists:
//   - tekton-operator-proxy-webhook (openshift-pipelines): reads APIServer TLS profile
//     at process startup and sets WEBHOOK_TLS_MIN_VERSION via os.Setenv — this is a
//     runtime process env var, not a Deployment spec env var, so it cannot be asserted
//     here. Validated by the TLS scanner (Step 2).
//   - tekton-operator-webhook (openshift-operators namespace): same runtime os.Setenv
//     mechanism; also lives in a different namespace. Validated by the TLS scanner.
var tlsEnvDeployments = []string{
	config.TriggersInterceptorsName, // tekton-triggers-core-interceptors
	config.ResultsAPIName,           // tekton-results-api
}

var _ = Describe("SRVKP-11926: Central TLS profile propagation to Pipelines components",
	Serial, Ordered, ContinueOnFailure, Label("e2e", "operator", "admin", "tls-profile"), func() {

		var originalProfile string

		BeforeAll(func() {
			if operator.IsHostedCluster(sharedClients) {
				Skip("Skipping TLS profile propagation tests: APIServer/cluster is immutable on HyperShift hosted clusters")
			}

			lastNamespace = config.TargetNamespace
			operator.EnsureTektonConfigStatusInstalled(
				sharedClients.TektonConfig(), store.GetCRNames())

			originalProfile = operator.GetClusterTLSProfileType(sharedClients)
			log.Printf("Saved original cluster TLS profile: %q", originalProfile)

			DeferCleanup(func() {
				log.Printf("Restoring cluster TLS profile to %q", originalProfile)
				operator.PatchClusterTLSProfile(sharedClients, originalProfile)
				operator.EnsureTektonConfigStatusInstalled(
					sharedClients.TektonConfig(), store.GetCRNames())
			})
		})

		// ── Intermediate profile ──────────────────────────────────────────────
		// "Intermediate" is supported on all OCP versions (4.x+).
		// It sets TLS_MIN_VERSION=VersionTLS12 on Go-based components.

		It("SRVKP-11926-TC01: Intermediate profile propagates correct TLS env vars to all Go-based deployments",
			Label("tls-profile", "intermediate"), func() {

				operator.PatchClusterTLSProfile(sharedClients, config.TLSProfileIntermediate)
				operator.EnsureTektonConfigStatusInstalled(
					sharedClients.TektonConfig(), store.GetCRNames())

				for _, deployment := range knativeWebhookDeployments {
					log.Printf("Asserting %s has %s=%s",
						deployment, config.WebhookTLSMinVersionEnvVar, config.TLSVersionTLS12)
					operator.AssertDeploymentHasEnvVar(
						sharedClients,
						config.TargetNamespace,
						deployment,
						config.WebhookTLSMinVersionEnvVar,
						config.TLSVersionTLS12,
					)
				}
				for _, deployment := range tlsEnvDeployments {
					log.Printf("Asserting %s has %s=%s",
						deployment, config.TLSMinVersionEnvVar, config.TLSVersionTLS12)
					operator.AssertDeploymentHasEnvVar(
						sharedClients,
						config.TargetNamespace,
						deployment,
						config.TLSMinVersionEnvVar,
						config.TLSVersionTLS12,
					)
				}
			})

		It("SRVKP-11926-TC02: Intermediate profile propagates TLSv1.2+TLSv1.3 to nginx console plugin ConfigMap",
			Label("tls-profile", "intermediate", "nginx"), func() {

				// Cluster is already in Intermediate profile from TC01 (Ordered suite).
				operator.AssertNginxConfigMapHasTLSProfile(
					sharedClients,
					config.TargetNamespace,
					config.NginxConsolePluginConfigMap,
					config.TLSProfileIntermediate,
				)
			})

		// NOTE: "Modern" (TLS 1.3 only) profile tests are intentionally omitted here.
		// The "Modern" profile type is not supported on OCP 4.16 (added in OCP 4.17+).
		// The "Old" profile (TLS 1.0) is a security regression irrelevant to PQC readiness.
		// TC03/TC04 for Modern profile should be added when the cluster is OCP 4.17+.
	})
