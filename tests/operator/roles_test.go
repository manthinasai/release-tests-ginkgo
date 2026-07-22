package operator_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Verify Roles for OSP",
	Label("e2e", "operator", "admin", "sanity", "roles"), func() {

		BeforeEach(func() {
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())
		})

		// PIPELINES-34-TC01
		It("Verify Roles in openshift-pipelines ns", func() {
			expectedRoles := []string{
				"manual-approval-gate-controller",
				"manual-approval-gate-info",
				"manual-approval-gate-webhook",
				"openshift-pipelines-read",
				"pipelines-as-code-controller-role",
				"pipelines-as-code-info",
				"pipelines-as-code-monitoring",
				"pipelines-as-code-watcher-role",
				"pipelines-as-code-webhook-role",
				"tekton-chains-info",
				"tekton-chains-leader-election",
				"tekton-default-openshift-pipelines-view",
				"tekton-ecosystem-stepaction-list-role",
				"tekton-ecosystem-task-list-role",
				"tekton-hub-info",
				"tekton-operators-proxy-admin",
				"tekton-pipelines-controller",
				"tekton-pipelines-events-controller",
				"tekton-pipelines-info",
				"tekton-pipelines-leader-election",
				"tekton-pipelines-resolvers-namespace-rbac",
				"tekton-pipelines-webhook",
				"tekton-results-info",
				"tekton-triggers-admin-webhook",
				"tekton-triggers-core-interceptors",
				"tekton-triggers-info",
			}

			// Verify each expected role is present
			for _, role := range expectedRoles {
				operator.VerifyRolesArePresent(sharedClients, role, "openshift-pipelines")
			}

			// Verify total count matches
			fullOutput := cmd.MustSucceed("oc", "get", "role", "-n", "openshift-pipelines", "-o", "name").Stdout()
			lines := strings.Split(strings.TrimSpace(fullOutput), "\n")
			actualCount := 0
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					actualCount++
				}
			}
			Expect(actualCount).To(BeNumerically(">=", len(expectedRoles)),
				"Too few roles in namespace openshift-pipelines. Expected at least %d, Actual: %d\nFull output:\n%s",
				len(expectedRoles), actualCount, fullOutput)
		})
	})
