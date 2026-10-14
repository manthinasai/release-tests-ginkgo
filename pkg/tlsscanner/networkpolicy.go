package tlsscanner

import (
	"fmt"
	"log"
	"strings"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

const (
	// consolePluginAllowPolicy is a temporary NetworkPolicy that lets the
	// tls-scanner namespace probe pipelines-console-plugin:8443. The product
	// policies only allow ingress from openshift-console; without this allow
	// rule the scanner reports NO_TLS even though nginx serves TLS.
	consolePluginAllowPolicy = "tls-scanner-console-plugin-allow"
)

// AllowScannerIngressToConsolePlugin creates a temporary NetworkPolicy in the
// Pipelines target namespace that permits TCP/8443 from ScannerNamespace to
// pods labeled app=pipelines-console-plugin. Safe alongside the product
// deny/allow policies (NetworkPolicy ingress rules are additive).
func AllowScannerIngressToConsolePlugin() {
	yaml := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/part-of: tls-scanner
spec:
  podSelector:
    matchLabels:
      app: %s
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: %s
    ports:
    - protocol: TCP
      port: 8443
`, consolePluginAllowPolicy, config.TargetNamespace, config.ConsolePluginDeployment, ScannerNamespace)

	log.Printf("Allowing scanner ingress to %s/%s:8443 via NetworkPolicy %q",
		config.TargetNamespace, config.ConsolePluginDeployment, consolePluginAllowPolicy)
	cmd.MustSucceedWithStdin(strings.NewReader(yaml), "oc", "apply", "-f", "-")
}

// TeardownScannerIngressToConsolePlugin deletes the temporary allow NetworkPolicy.
// Errors are ignored so DeferCleanup can always complete.
func TeardownScannerIngressToConsolePlugin() {
	log.Printf("Deleting temporary NetworkPolicy %s/%s",
		config.TargetNamespace, consolePluginAllowPolicy)
	_ = cmd.Run("oc", "delete", "networkpolicy", consolePluginAllowPolicy,
		"-n", config.TargetNamespace, "--ignore-not-found=true")
}
