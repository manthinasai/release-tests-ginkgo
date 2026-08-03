package triggers_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/triggers"
)

var _ = Describe("Verify eventlisteners spec: PIPELINES-05", Label("triggers"), func() {

	// TC01-TC06: Pending — steps and testdata not yet implemented in Gauge either.
	// TODO: create testdata/triggers/eventlisteners/{triggertemplate,triggerbinding,role-resources,custom-interceptor}/
	// TODO: implement helper functions for AddEventListener variants and resource/label verification
	Describe("EventListener Basic (to-do)", func() {
		It("Create Eventlistener: PIPELINES-05-TC01", Label("to-do"), Pending, func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/eventlisteners/triggertemplate/template.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding-message.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/role-resources/rbac.yaml", ns)
			// TODO: oc.Create("testdata/triggers/eventlisteners/<el-yaml>", ns)
			elName := "basic-eventlistener" // TODO: confirm from EL YAML
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, elName, ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, elName, ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, elName, ns)
			// TODO: verify openshift-pipeline-resources created
			// TODO: verify resources created with labels & event-id
			// TODO: pipelines.ValidatePipelineRun(sharedClients, "<pipeline-run-name>", "successful", ns)
		})

		It("Create Eventlistener with github interceptor: PIPELINES-05-TC02", Label("to-do"), Pending, func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/eventlisteners/triggertemplate/template.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding-message.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/role-resources/rbac.yaml", ns)
			// TODO: oc.Create("testdata/triggers/eventlisteners/<github-interceptor-el-yaml>", ns)
			elName := "github-interceptor-listener" // TODO: confirm from EL YAML
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, elName, ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, elName, ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, elName, ns)
			// TODO: verify openshift-pipeline-resources created
			// TODO: verify resources created with labels & event-id
			// TODO: pipelines.ValidatePipelineRun(sharedClients, "<pipeline-run-name>", "successful", ns)
		})

		It("Create EventListener with custom interceptor: PIPELINES-05-TC03", Label("to-do"), Pending, func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/eventlisteners/triggertemplate/template.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding-message.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/role-resources/rbac.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/custom-interceptor/gh-validate-service.yaml", ns)
			// TODO: oc.Create("testdata/triggers/eventlisteners/<custom-interceptor-el-yaml>", ns)
			elName := "custom-interceptor-listener" // TODO: confirm from EL YAML
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, elName, ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, elName, ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, elName, ns)
			// TODO: verify openshift-pipeline-resources created
			// TODO: verify resources created with labels & event-id
			// TODO: pipelines.ValidatePipelineRun(sharedClients, "<pipeline-run-name>", "successful", ns)
		})

		It("Create EventListener with CEL interceptor with filter: PIPELINES-05-TC04", Label("to-do"), Pending, func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/eventlisteners/triggertemplate/template.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/role-resources/rbac.yaml", ns)
			// TODO: oc.Create("testdata/triggers/eventlisteners/<cel-filter-el-yaml>", ns)
			elName := "cel-filter-listener" // TODO: confirm from EL YAML
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, elName, ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, elName, ns)
			// TODO: send CEL push/pr events with appropriate CEL payload
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, elName, ns)
			// TODO: verify openshift-pipeline-resources created
			// TODO: verify resources created with labels & event-id
			// TODO: pipelines.ValidatePipelineRun(sharedClients, "<pipeline-run-name>", "successful", ns)
		})

		It("Create EventListener with CEL interceptor without filter: PIPELINES-05-TC05", Label("to-do"), Pending, func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/eventlisteners/triggertemplate/template.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/role-resources/rbac.yaml", ns)
			// TODO: oc.Create("testdata/triggers/eventlisteners/<cel-no-filter-el-yaml>", ns)
			elName := "cel-no-filter-listener" // TODO: confirm from EL YAML
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, elName, ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, elName, ns)
			// TODO: send CEL push/pr events with appropriate CEL payload
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, elName, ns)
			// TODO: verify openshift-pipeline-resources created
			// TODO: verify resources created with labels & event-id
			// TODO: pipelines.ValidatePipelineRun(sharedClients, "<pipeline-run-name>", "successful", ns)
		})

		It("Create EventListener with multiple interceptors: PIPELINES-05-TC06", Label("to-do"), Pending, func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/eventlisteners/triggertemplate/template.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/triggerbinding/binding.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/role-resources/rbac.yaml", ns)
			// TODO: oc.Create("testdata/triggers/eventlisteners/<multi-interceptor-el-yaml>", ns)
			elName := "multi-interceptor-listener" // TODO: confirm from EL YAML
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, elName, ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, elName, ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, elName, ns)
			// TODO: verify openshift-pipeline-resources created
			// TODO: verify resources created with labels & event-id
			// TODO: pipelines.ValidatePipelineRun(sharedClients, "<pipeline-run-name>", "successful", ns)
		})
	})

	Describe("Create Eventlistener with TLS enabled: PIPELINES-05-TC07", func() {
		It("Create Eventlistener with TLS enabled", Label("tls", "admin", "e2e"), func() {
			ns := lastNamespace
			oc.EnableTLSConfigForEventlisteners(ns)

			oc.Create("testdata/triggers/sample-pipeline.yaml", ns)
			oc.Create("testdata/triggers/triggerbindings/triggerbinding.yaml", ns)
			oc.Create("testdata/triggers/triggertemplate/triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/eventlistener-embeded-binding.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "pipeline-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-embed-binding", ns)
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-embed-binding", ns) })

			routeURL := triggers.ExposeEventListenerForTLS(sharedClients, "listener-embed-binding", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", true)
			triggers.AssertElResponse(sharedClients, resp, "listener-embed-binding", ns)

			pipelines.ValidatePipelineRun(sharedClients, "simple-pipeline-run", "successful", ns)
		})
	})

	Describe("Create Eventlistener embedded TriggersBindings specs: PIPELINES-05-TC08", func() {
		It("Create Eventlistener embedded TriggersBindings specs", Label("e2e", "non-admin", "sanity"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/sample-pipeline.yaml", ns)
			oc.Create("testdata/triggers/triggerbindings/triggerbinding.yaml", ns)
			oc.Create("testdata/triggers/triggertemplate/triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/eventlistener-embeded-binding.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "pipeline-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-embed-binding", ns)
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-embed-binding", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "listener-embed-binding", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, "listener-embed-binding", ns)

			pipelines.ValidatePipelineRun(sharedClients, "simple-pipeline-run", "successful", ns)
		})
	})

	Describe("Create embedded TriggersTemplate: PIPELINES-05-TC09", func() {
		It("Create embedded TriggersTemplate", Label("e2e", "non-admin", "sanity"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/triggerbindings/triggerbinding.yaml", ns)
			oc.Create("testdata/triggers/triggertemplate/embed-triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/eventlistener-embeded-binding.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "pipeline-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-embed-binding", ns)
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-embed-binding", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "listener-embed-binding", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, "listener-embed-binding", ns)

			pipelines.ValidatePipelineRun(sharedClients, "pipelinerun-with-taskspec-to-echo-message", "successful", ns)
		})
	})

	Describe("Create Eventlistener with gitlab interceptor: PIPELINES-05-TC10", func() {
		It("Create Eventlistener with gitlab interceptor", Label("e2e", "non-admin"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/gitlab/gitlab-push-listener.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "gitlab-push-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "gitlab-echo-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "gitlab-listener", ns)
			Expect(err).NotTo(HaveOccurred())

			oc.CreateSecretWithSecretToken("gitlab-secret", ns)
			oc.LinkSecretToSA("gitlab-secret", "pipeline", ns)

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "gitlab-listener", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "gitlab-listener", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "gitlab", "Push Hook", "testdata/triggers/gitlab/gitlab-push-event.json", false)
			triggers.AssertElResponse(sharedClients, resp, "gitlab-listener", ns)

			pipelines.ValidatePipelineRun(sharedClients, "gitlab-run", "successful", ns)
		})
	})

	Describe("Create Eventlistener with bitbucket interceptor: PIPELINES-05-TC11", func() {
		It("Create Eventlistener with bitbucket interceptor", Label("e2e", "non-admin"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/bitbucket/bitbucket-eventlistener-interceptor.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "bitbucket-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "bitbucket-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "bitbucket-listener", ns)
			Expect(err).NotTo(HaveOccurred())

			oc.CreateSecretWithSecretToken("bitbucket-secret", ns)
			oc.LinkSecretToSA("bitbucket-secret", "pipeline", ns)

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "bitbucket-listener", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "bitbucket-listener", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "bitbucket", "refs_changed", "testdata/triggers/bitbucket/refs-change-event.json", false)
			triggers.AssertElResponse(sharedClients, resp, "bitbucket-listener", ns)

			// Bitbucket test verifies TaskRun with Failure status (intentional, matching Gauge spec)
			pipelines.ValidateTaskRun(sharedClients, "bitbucket-run", "Failure", ns)
		})
	})

	Describe("Verify Github push event with Embedded TriggerTemplate using Github-CTB: PIPELINES-05-TC12", func() {
		It("Verify Github push event with Embedded TriggerTemplate using Github-CTB", Label("e2e", "non-admin", "sanity"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/github-ctb/Embeddedtriggertemplate-git-push.yaml", ns)
			oc.Create("testdata/triggers/github-ctb/eventlistener-ctb-git-push.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template-git-push", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-ctb-github-push", ns)
			Expect(err).NotTo(HaveOccurred())

			oc.CreateSecretWithSecretToken("github-secret", ns)
			oc.LinkSecretToSA("github-secret", "pipeline", ns)

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-ctb-github-push", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "listener-ctb-github-push", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/triggers/github-ctb/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, "listener-ctb-github-push", ns)

			pipelines.ValidatePipelineRun(sharedClients, "pipelinerun-git-push-ctb", "successful", ns)
		})
	})

	Describe("Verify Github pull_request event with Embedded TriggerTemplate using Github-CTB: PIPELINES-05-TC13", func() {
		It("Verify Github pull_request event with Embedded TriggerTemplate using Github-CTB", Label("e2e", "non-admin", "sanity"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/github-ctb/Embeddedtriggertemplate-git-pr.yaml", ns)
			oc.Create("testdata/triggers/github-ctb/eventlistener-ctb-git-pr.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template-git-pr", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-clustertriggerbinding-github-pr", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("clustertriggerbinding", "github-pullreq", ns)
			Expect(err).NotTo(HaveOccurred())

			oc.CreateSecretWithSecretToken("github-secret", ns)
			oc.LinkSecretToSA("github-secret", "pipeline", ns)

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-clustertriggerbinding-github-pr", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "listener-clustertriggerbinding-github-pr", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "pull_request", "testdata/triggers/github-ctb/pr.json", false)
			triggers.AssertElResponse(sharedClients, resp, "listener-clustertriggerbinding-github-pr", ns)

			pipelines.ValidatePipelineRun(sharedClients, "pipelinerun-git-pr-ctb", "successful", ns)
		})
	})

	Describe("Verify Github pr_review event with Embedded TriggerTemplate using Github-CTB: PIPELINES-05-TC14", func() {
		It("Verify Github pr_review event with Embedded TriggerTemplate using Github-CTB", Label("e2e", "non-admin"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/github-ctb/Embeddedtriggertemplate-git-pr-review.yaml", ns)
			oc.Create("testdata/triggers/github-ctb/eventlistener-ctb-git-pr-review.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template-git-pr-review", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-ctb-github-pr-review", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("clustertriggerbinding", "github-pullreq", ns)
			Expect(err).NotTo(HaveOccurred())

			oc.CreateSecretWithSecretToken("github-secret", ns)
			oc.LinkSecretToSA("github-secret", "pipeline", ns)

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-ctb-github-pr-review", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "listener-ctb-github-pr-review", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "issue_comment", "testdata/triggers/github-ctb/issue-comment.json", false)
			triggers.AssertElResponse(sharedClients, resp, "listener-ctb-github-pr-review", ns)

			pipelines.ValidatePipelineRun(sharedClients, "pipelinerun-git-pr-review-ctb", "successful", ns)
		})
	})

	Describe("Create TriggersCRD resource with CEL interceptors (overlays): PIPELINES-05-TC15", func() {
		It("Create TriggersCRD resource with CEL interceptors (overlays)", Label("e2e", "non-admin", "sanity"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/triggersCRD/eventlistener-triggerref.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/trigger.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/triggerbindings.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/pipeline.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "github-pr-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "github-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-triggerref", ns)
			Expect(err).NotTo(HaveOccurred())

			oc.CreateSecretWithSecretToken("github-secret", ns)
			oc.LinkSecretToSA("github-secret", "pipeline", ns)

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-triggerref", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "listener-triggerref", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "pull_request", "testdata/triggers/triggersCRD/pull-request.json", false)
			triggers.AssertElResponse(sharedClients, resp, "listener-triggerref", ns)

			pipelines.ValidatePipelineRun(sharedClients, "parallel-pipelinerun", "successful", ns)
		})
	})

	Describe("Create multiple Eventlistener with TLS enabled: PIPELINES-05-TC16", func() {
		It("Create multiple Eventlistener with TLS enabled", Label("e2e", "tls", "admin", "sanity"), func() {
			ns := lastNamespace
			oc.EnableTLSConfigForEventlisteners(ns)

			// First EventListener
			oc.Create("testdata/triggers/sample-pipeline.yaml", ns)
			oc.Create("testdata/triggers/triggerbindings/triggerbinding.yaml", ns)
			oc.Create("testdata/triggers/triggertemplate/triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/eventlistener-embeded-binding.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "pipeline-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-embed-binding", ns)
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-embed-binding", ns) })

			routeURL := triggers.ExposeEventListenerForTLS(sharedClients, "listener-embed-binding", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", true)
			triggers.AssertElResponse(sharedClients, resp, "listener-embed-binding", ns)

			pipelines.ValidatePipelineRun(sharedClients, "simple-pipeline-run", "successful", ns)

			// Second EventListener
			oc.Create("testdata/triggers/triggertemplate/triggertemplate-2.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/eventlistener-embeded-binding-2.yaml", ns)
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-embed-binding-2", ns) })

			routeURL2 := triggers.ExposeEventListenerForTLS(sharedClients, "listener-embed-binding-2", ns)
			resp2, _ := triggers.MockPostEvent(routeURL2, "github", "push", "testdata/push.json", true)
			triggers.AssertElResponse(sharedClients, resp2, "listener-embed-binding-2", ns)

			pipelines.ValidatePipelineRun(sharedClients, "simple-pipeline-run-2", "successful", ns)
		})
	})

	Describe("Create Eventlistener with github interceptor And verify Kubernetes Events: PIPELINES-05-TC17", func() {
		It("Create Eventlistener with github interceptor And verify Kubernetes Events", Label("e2e", "events", "admin", "sanity"), func() {
			ns := lastNamespace

			oc.Create("testdata/triggers/sample-pipeline.yaml", ns)
			oc.Create("testdata/triggers/triggerbindings/triggerbinding.yaml", ns)
			oc.Create("testdata/triggers/triggertemplate/triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/eventlistener-embeded-binding.yaml", ns)

			_, err := opc.VerifyResourceListMatchesName("triggerbinding", "pipeline-binding", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("triggertemplate", "pipeline-template", ns)
			Expect(err).NotTo(HaveOccurred())
			_, err = opc.VerifyResourceListMatchesName("eventlistener", "listener-embed-binding", ns)
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "listener-embed-binding", ns) })

			routeURL := triggers.ExposeEventListener(sharedClients, "listener-embed-binding", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, "listener-embed-binding", ns)

			oc.VerifyKubernetesEventsForEventListener(ns)

			pipelines.ValidatePipelineRun(sharedClients, "simple-pipeline-run", "successful", ns)
		})
	})
})
