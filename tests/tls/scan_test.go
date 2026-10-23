package tls_test

import (
	"log"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/tlsscanner"
)

// scanResults holds the parsed output from the tls-scanner Job.
// Populated once by scanOnce and shared across all It blocks.
var (
	scanResults *tlsscanner.ScanResults
	scanOnce    sync.Once
	scanErr     error
)

// ensureScanned runs the tls-scanner Job exactly once per test process.
// Subsequent calls return the cached results immediately.
func ensureScanned() {
	scanOnce.Do(func() {
		lastNamespace = tlsscanner.ScannerNamespace

		tlsscanner.SetupScannerNamespace()
		tlsscanner.SetupScannerRBAC()
		// Product NPs only allow openshift-console → console-plugin:8443.
		// Temporarily allow the scanner namespace so the probe can negotiate TLS.
		tlsscanner.AllowScannerIngressToConsolePlugin()
		DeferCleanup(func() {
			tlsscanner.TeardownScannerIngressToConsolePlugin()
			tlsscanner.TeardownScannerRBAC()
		})

		podName := tlsscanner.RunScannerJob(sharedClients, config.TargetNamespace)

		var resultsPath string
		resultsPath, scanErr = config.TempFile("tls-results.json")
		if scanErr != nil {
			return
		}

		tlsscanner.ExtractResults(podName, resultsPath)
		scanResults = tlsscanner.ParseResultsJSON(resultsPath)
		log.Printf("Loaded scan results: %d IPs scanned", scanResults.ScannedIPs)
	})
	Expect(scanErr).NotTo(HaveOccurred(), "tls-scanner setup failed")
	Expect(scanResults).NotTo(BeNil(), "tls-scanner produced no results")
}

// ── TC01: cluster TLS profile check ──────────────────────────────────────────
//
// Separate, non-cascading Describe so that a "Default" or unexpected profile
// value does NOT skip TC02–TC09 (the actual component compliance tests).
var _ = Describe("SRVKP-11926: TLS scanner — cluster TLS profile check: PIPELINES-40",
	Serial, Ordered, Label("e2e", "tls-scan", "admin", "pqc", "cluster-config"), func() {

		BeforeAll(ensureScanned)

		// Accepts "Default" (fresh install), "Intermediate" (OCP 4.16), "Modern" (OCP 4.17+).
		// Only "Old" (TLS 1.0) is rejected — it is a security regression vs PQC requirements.
		It("SRVKP-11926-TC01: cluster APIServer TLS security config reports a PQC-capable profile: PIPELINES-40-TC01",
			func() {
				tlsscanner.AssertClusterTLSSecurityConfig(scanResults,
					config.TLSProfileDefault, config.TLSProfileModern, config.TLSProfileIntermediate)
			})
	})

// ── TC02–TC09: per-component PQC compliance ───────────────────────────────────
//
// NOT Ordered — each It block is fully independent. A failure in one component
// does not skip the others. All blocks share scanResults from ensureScanned().
var _ = Describe("SRVKP-11926: TLS scanner PQC compliance — all Pipelines components: PIPELINES-40",
	Serial, Label("e2e", "tls-scan", "admin", "pqc"), func() {

		BeforeEach(ensureScanned)

		// ── Go-based webhook components ──────────────────────────────────────

		It("SRVKP-11926-TC02: tekton-pipelines-webhook port 8443 is fully PQC compliant: PIPELINES-40-TC02",
			Label("webhook"), func() {
				tlsscanner.AssertComponentPQCCompliant(scanResults, config.PipelineWebhookName)
			})

		It("SRVKP-11926-TC03: tekton-triggers-webhook port 8443 is fully PQC compliant: PIPELINES-40-TC03",
			Label("webhook"), func() {
				tlsscanner.AssertComponentPQCCompliant(scanResults, config.TriggerWebhookName)
			})

		It("SRVKP-11926-TC04: tekton-triggers-core-interceptors port 8443 is fully PQC compliant: PIPELINES-40-TC04",
			Label("webhook"), func() {
				tlsscanner.AssertComponentPQCCompliant(scanResults, config.TriggersInterceptorsName)
			})

		It("SRVKP-11926-TC05: pipelines-as-code-webhook port 8443 is fully PQC compliant: PIPELINES-40-TC05",
			Label("webhook"), func() {
				tlsscanner.AssertComponentPQCCompliant(scanResults, config.PacWebhookName)
			})

		It("SRVKP-11926-TC06: tekton-operator-proxy-webhook port 8443 is fully PQC compliant: PIPELINES-40-TC06",
			Label("webhook"), func() {
				tlsscanner.AssertComponentPQCCompliant(scanResults, config.OperatorProxyWebhookName)
			})

		// TC07: tekton-results-api does not declare container ports in its pod spec,
		// so the scanner may report NO_PORTS. When that happens the test is skipped
		// with an informational message rather than failing. When the scanner does
		// find a TLS port, full PQC assertions are applied.
		It("SRVKP-11926-TC07: tekton-results-api is fully PQC compliant: PIPELINES-40-TC07",
			Label("results"), func() {
				tlsscanner.AssertComponentPQCCompliantOrNoPort(scanResults, config.ResultsAPIName)
			})

		// ── Non-Go TLS components ─────────────────────────────────────────────

		It("SRVKP-11926-TC08: pipelines-console-plugin (nginx) port 8443 is fully PQC compliant: PIPELINES-40-TC08",
			Label("nginx"), func() {
				tlsscanner.AssertComponentPQCCompliant(scanResults, config.ConsolePluginDeployment)
			})

		// TC09: tkn-cli-serve (Apache httpd) uses OpenShift edge TLS termination.
		// The pod serves plain HTTP on 8080; TLS is handled at the Route. Assert
		// that the scanner sees only NO_TLS (no spurious pod-level SSL on 8443).
		It("SRVKP-11926-TC09: tkn-cli-serve (apache httpd) uses edge TLS termination: PIPELINES-40-TC09",
			Label("httpd"), func() {
				tlsscanner.AssertComponentEdgeTerminated(scanResults, config.TknDeployment)
			})
	})
