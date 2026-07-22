// Package operator provides helpers for validating and configuring the Tekton Operator.
package operator

import (
	"log"
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/tektoncd/operator/test/utils"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// WaitForTektonConfigCR ensures a TektonConfig CR exists.
func WaitForTektonConfigCR(cs *clients.Clients, rnames utils.ResourceNames) {
	_, err := EnsureTektonConfigExists(cs.TektonConfig(), rnames)
	Expect(err).NotTo(HaveOccurred(), "TektonConfig doesn't exist")
}

// ValidateRBAC verifies that RBAC resources are auto-created successfully.
func ValidateRBAC(cs *clients.Clients, rnames utils.ResourceNames) {
	log.Printf("Verifying that TektonConfig status is \"installed\"\n")
	EnsureTektonConfigStatusInstalled(cs.TektonConfig(), rnames)

	AssertServiceAccountPresent(cs, store.Namespace(), "pipeline")
	AssertClusterRolePresent(cs, "pipelines-scc-clusterrole")
	AssertConfigMapPresent(cs, store.Namespace(), "config-service-cabundle")
	AssertConfigMapPresent(cs, store.Namespace(), "config-trusted-cabundle")
	AssertRoleBindingPresent(cs, store.Namespace(), "openshift-pipelines-edit")
	AssertRoleBindingPresent(cs, store.Namespace(), "pipelines-scc-rolebinding")
	AssertSCCPresent(cs, "pipelines-scc")
}

// ValidateRBACAfterDisable verifies RBAC resources are properly disabled.
func ValidateRBACAfterDisable(cs *clients.Clients, rnames utils.ResourceNames) {
	EnsureTektonConfigStatusInstalled(cs.TektonConfig(), rnames)
	AssertServiceAccountPresent(cs, store.Namespace(), "pipeline")
	AssertClusterRoleNotPresent(cs, "pipelines-scc-clusterrole")
	AssertRoleBindingNotPresent(cs, store.Namespace(), "edit")
	AssertRoleBindingNotPresent(cs, store.Namespace(), "pipelines-scc-rolebinding")
	AssertSCCNotPresent(cs, "pipelines-scc")
}

// ValidateCABundleConfigMaps verifies CA Bundle ConfigMaps are created.
func ValidateCABundleConfigMaps(cs *clients.Clients, rnames utils.ResourceNames) {
	log.Printf("Verifying that TektonConfig status is \"installed\"\n")
	EnsureTektonConfigStatusInstalled(cs.TektonConfig(), rnames)
	AssertConfigMapPresent(cs, store.Namespace(), "config-service-cabundle")
	AssertConfigMapPresent(cs, store.Namespace(), "config-trusted-cabundle")
}

// ValidateOperatorInstallStatus verifies the operator is installed and running.
func ValidateOperatorInstallStatus(cs *clients.Clients, rnames utils.ResourceNames) {
	operatorVersion := opc.GetOPCServerVersion("operator")
	Expect(operatorVersion).NotTo(ContainSubstring("unknown"),
		"Operator is not installed")
	log.Printf("Waiting for operator to be up and running....\n")
	EnsureTektonConfigStatusInstalled(cs.TektonConfig(), rnames)
	log.Printf("Operator is up\n")
}

// AssertTaskPresence polls until the named task is present or absent in namespace.
func AssertTaskPresence(namespace, taskName string, shouldBePresent bool) {
	log.Printf("Verifying task %s is present=%v in namespace %s", taskName, shouldBePresent, namespace)
	Eventually(func(g Gomega) {
		result := cmd.Run("oc", "get", "task", taskName, "-n", namespace)
		if shouldBePresent {
			g.Expect(result.ExitCode).To(Equal(0),
				"expected task %s to be present in namespace %s", taskName, namespace)
		} else {
			g.Expect(result.ExitCode).NotTo(Equal(0),
				"expected task %s to NOT be present in namespace %s", taskName, namespace)
		}
	}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(Succeed())
}

// AssertPipelinesPresence polls until pipelines exist (or don't) in namespace.
func AssertPipelinesPresence(namespace string, shouldBePresent bool) {
	log.Printf("Verifying pipelines present=%v in namespace %s", shouldBePresent, namespace)
	Eventually(func(g Gomega) {
		result := cmd.Run("oc", "get", "pipeline", "-n", namespace, "-o", "name")
		output := strings.TrimSpace(result.Stdout())
		if shouldBePresent {
			g.Expect(output).NotTo(BeEmpty(),
				"expected pipelines to exist in namespace %s", namespace)
		} else {
			g.Expect(output).To(BeEmpty(),
				"expected no pipelines in namespace %s, got: %s", namespace, output)
		}
	}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(Succeed())
}

// AssertCronjobPresence polls until a cronjob with the given prefix is present or absent.
func AssertCronjobPresence(namespace, prefix string, shouldBePresent bool) {
	log.Printf("Verifying cronjob prefix %s present=%v in namespace %s", prefix, shouldBePresent, namespace)
	if shouldBePresent {
		Eventually(func(g Gomega) {
			output := cmd.MustSucceed("oc", "get", "cronjob", "-n", namespace, "-o", "name").Stdout()
			g.Expect(output).To(ContainSubstring(prefix),
				"expected cronjob with prefix %s to be present in namespace %s", prefix, namespace)
		}).WithTimeout(2 * time.Minute).WithPolling(config.APIRetry).Should(Succeed())
	} else {
		Eventually(func(g Gomega) {
			output := cmd.Run("oc", "get", "cronjob", "-n", namespace, "-o", "name").Stdout()
			g.Expect(output).NotTo(ContainSubstring(prefix),
				"expected cronjob with prefix %s to NOT be present in namespace %s", prefix, namespace)
		}).WithTimeout(2 * time.Minute).WithPolling(config.APIRetry).Should(Succeed())
	}
}

// GetCronjobName returns the name of the first cronjob with the given schedule in namespace.
func GetCronjobName(namespace, schedule string) string {
	output := cmd.MustSucceed("oc", "get", "cronjob", "-n", namespace,
		"-o", "jsonpath={range .items[?(@.spec.schedule==\""+schedule+"\")]}{.metadata.name}{end}").Stdout()
	Expect(strings.TrimSpace(output)).NotTo(BeEmpty(),
		"expected to find cronjob with schedule %s in namespace %s", schedule, namespace)
	return strings.TrimSpace(output)
}
