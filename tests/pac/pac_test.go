package pac_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

var _ = Describe("Pipelines As Code TektonConfig tests: PIPELINES-20", func() {

	// =========================================================================
	// =========================================================================
	Describe("Enable/Disable PAC: PIPELINES-20-TC01", Ordered, Serial, ContinueOnFailure, Label("pac", "sanity"), func() {

		BeforeAll(func() {
			lastNamespace = config.TargetNamespace
		})

		// setPACEnabled sets the pipelinesAsCode.enable field in the TektonConfig CR.
		setPACEnabled := func(enabled bool) {
			tc, err := sharedClients.TektonConfig().Get(context.Background(), "config", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "failed to get TektonConfig")

			specBytes, err := json.Marshal(tc.Spec)
			Expect(err).NotTo(HaveOccurred())

			var specMap map[string]any
			err = json.Unmarshal(specBytes, &specMap)
			Expect(err).NotTo(HaveOccurred())

			pacSection, ok := specMap["platforms"].(map[string]any)
			if !ok {
				pacSection = make(map[string]any)
				specMap["platforms"] = pacSection
			}
			openshiftSection, ok := pacSection["openshift"].(map[string]any)
			if !ok {
				openshiftSection = make(map[string]any)
				pacSection["openshift"] = openshiftSection
			}
			pacConfig, ok := openshiftSection["pipelinesAsCode"].(map[string]any)
			if !ok {
				pacConfig = make(map[string]any)
				openshiftSection["pipelinesAsCode"] = pacConfig
			}

			pacConfig["enable"] = enabled

			updatedSpec, err := json.Marshal(specMap)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(updatedSpec, &tc.Spec)
			Expect(err).NotTo(HaveOccurred())

			_, err = sharedClients.TektonConfig().Update(context.Background(), tc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to update TektonConfig pipelinesAsCode.enable to %v", enabled))

			log.Printf("Set pipelinesAsCode.enable to %v", enabled)
		}

		It("should disable pipelinesAsCode", func() {
			setPACEnabled(false)
		})

		It("should verify PAC installersets are not present when disabled", func() {
			Eventually(func() bool {
				tis, err := sharedClients.Operator.TektonInstallerSets().List(context.Background(), metav1.ListOptions{})
				if err != nil {
					return false
				}
				for _, is := range tis.Items {
					if strings.HasPrefix(is.Name, "openshiftpipelinesascode") {
						return false
					}
				}
				return true
			}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(BeTrue(),
				"PAC installersets should not be present when disabled")
		})

		It("should verify PAC pods are not present when disabled", func() {
			pacPodLabels := []string{config.PacControllerName, config.PacWatcherName, config.PacWebhookName}
			Eventually(func() bool {
				pods, err := sharedClients.KubeClient.Kube.CoreV1().Pods(config.TargetNamespace).List(
					context.Background(), metav1.ListOptions{})
				if err != nil {
					return false
				}
				for _, pod := range pods.Items {
					for _, name := range pacPodLabels {
						if strings.Contains(pod.Name, name) {
							return false
						}
					}
				}
				return true
			}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(BeTrue(),
				"PAC pods should not be present when disabled")
		})

		It("should verify pipelines-as-code CR is removed when disabled", func() {
			Eventually(func() bool {
				_, err := sharedClients.PipelinesAsCode().Get(context.Background(), "pipelines-as-code", metav1.GetOptions{})
				return err != nil
			}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(BeTrue(),
				"pipelines-as-code CR should be removed when PAC is disabled")
		})

		It("should re-enable pipelinesAsCode", func() {
			setPACEnabled(true)

			DeferCleanup(func() {
				setPACEnabled(true)
			})
		})

		It("should verify PAC installersets are present when enabled", func() {
			Eventually(func() bool {
				tis, err := sharedClients.Operator.TektonInstallerSets().List(context.Background(), metav1.ListOptions{})
				if err != nil {
					return false
				}
				for _, is := range tis.Items {
					if strings.HasPrefix(is.Name, "openshiftpipelinesascode") {
						return true
					}
				}
				return false
			}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(BeTrue(),
				"PAC installersets should be present when enabled")
		})

		It("should verify PAC pods are present when enabled", func() {
			pacPodNames := []string{config.PacControllerName, config.PacWatcherName, config.PacWebhookName}
			Eventually(func() int {
				foundCount := 0
				pods, err := sharedClients.KubeClient.Kube.CoreV1().Pods(config.TargetNamespace).List(
					context.Background(), metav1.ListOptions{})
				if err != nil {
					return 0
				}
				for _, pod := range pods.Items {
					for _, name := range pacPodNames {
						if strings.Contains(pod.Name, name) {
							foundCount++
							break
						}
					}
				}
				return foundCount
			}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(BeNumerically(">=", len(pacPodNames)),
				"PAC pods should be present when enabled")
		})

		It("should verify pipelines-as-code CR state after re-enable", func() {
			Eventually(func() error {
				_, err := sharedClients.PipelinesAsCode().Get(context.Background(), "pipelines-as-code", metav1.GetOptions{})
				return err
			}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(Succeed(),
				"pipelines-as-code CR should exist after re-enable")
		})
	})

	// =========================================================================
	// =========================================================================
	PDescribe("Application name change visible in GitHub UI: PIPELINES-20-TC02", Label("pac", "sanity"), func() {
		// TODO: Requires GitHub App configuration not available in current test infrastructure
		// Steps from Gauge spec:
		// - Change application-name in tektonconfig
		// - Configure PAC using GitHub app
		// - Create repo, repo CRD, configure push event
		// - Verify pipelinerun created
		// - Verify pipelinerun status in GitHub
		// - Verify application name shown in GitHub UI
		It("should reflect application name change in GitHub UI", func() {})
	})

	// =========================================================================
	// =========================================================================
	PDescribe("Auto-configure new GitHub repo: PIPELINES-20-TC03", Label("pac", "sanity"), func() {
		// TODO: Requires GitHub App configuration not available in current test infrastructure
		// Steps from Gauge spec:
		// - Set auto-configure-new-github-repo to true
		// - Configure PAC using GitHub app
		// - Create new repo in GitHub
		// - Verify repo CR created
		// - Set auto-configure-new-github-repo to false
		// - Create another repo
		// - Verify repo CR is not created
		It("should auto-create repo CR when enabled", func() {})
		It("should not create repo CR when disabled", func() {})
	})

	// =========================================================================
	// =========================================================================
	PDescribe("Error log snippet visibility: PIPELINES-20-TC04", Label("pac", "sanity"), func() {
		// TODO: Requires GitHub App configuration not available in current test infrastructure
		// Steps from Gauge spec:
		// - Set error-log-snippet to false
		// - Configure PAC using GitHub app
		// - Create repo and failing pipelinerun
		// - Verify error log NOT shown in GitHub UI
		// - Set error-log-snippet to true
		// - Trigger push and verify error log IS shown
		It("should not show error log when disabled", func() {})
		It("should show error log when enabled", func() {})
	})
})
