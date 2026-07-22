package ecosystem_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

// PIPELINES-32
// ========================================================================
//
// These tests verify ecosystem tasks on different CPU architectures.
// Architecture-specific tests are skipped automatically when the cluster
// architecture doesn't match the required architecture.
//
// Test Cases:
//   PIPELINES-32-TC01: jib-maven pipelinerun (amd64)
//   PIPELINES-32-TC02: jib-maven P&Z pipelinerun (ppc64le, s390x, arm64)
//   PIPELINES-32-TC03: kn-apply pipelinerun (amd64)
//   PIPELINES-32-TC04: kn-apply p&z pipelinerun (ppc64le, s390x)
//   PIPELINES-32-TC05: kn pipelinerun (amd64)
//   PIPELINES-32-TC06: kn p&z pipelinerun (ppc64le, s390x)
// ========================================================================

// PIPELINES-32-TC01
var _ = Describe("jib-maven pipelinerun", Label("ecosystem", "e2e", "sanity", "jib-maven"), func() {
	It("should create jib-maven pipelinerun with registry credentials", func() {
		if len([]string{"amd64"}) > 0 {
			archMatch := false
			for _, arch := range []string{"amd64"} {
				if config.Flags.ClusterArch == arch {
					archMatch = true
					break
				}
			}
			if !archMatch {
				Skip(fmt.Sprintf("requires one of architectures: %v (cluster is %s)", []string{"amd64"}, config.Flags.ClusterArch))
			}
		}

		ns := createTestNamespace("eco-jib-maven")
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")

		oc.ValidateAndCreateJibMavenSecret(ns)

		oc.Create("testdata/ecosystem/pipelines/jib-maven.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/jib-maven.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "jib-maven-run", "successful", ns)
	})
})

// PIPELINES-32-TC02
var _ = Describe("jib-maven P&Z pipelinerun", Label("ecosystem", "e2e", "sanity", "jib-maven"), func() {
	It("should create jib-maven pipelinerun with registry credentials", func() {
		if len([]string{"ppc64le", "s390x", "arm64"}) > 0 {
			archMatch := false
			for _, arch := range []string{"ppc64le", "s390x", "arm64"} {
				if config.Flags.ClusterArch == arch {
					archMatch = true
					break
				}
			}
			if !archMatch {
				Skip(fmt.Sprintf("requires one of architectures: %v (cluster is %s)", []string{"ppc64le", "s390x", "arm64"}, config.Flags.ClusterArch))
			}
		}

		ns := createTestNamespace("eco-jib-maven-pz")
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")

		oc.Create("testdata/ecosystem/pipelines/jib-maven-pz.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/jib-maven-pz.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "jib-maven-pz-run", "successful", ns)
	})
})

// PIPELINES-32-TC03
var _ = Describe("kn-apply pipelinerun", Label("ecosystem", "e2e", "kn-apply"), func() {
	It("should create and verify kn-apply pipelinerun", func() {
		if len([]string{"amd64"}) > 0 {
			archMatch := false
			for _, arch := range []string{"amd64"} {
				if config.Flags.ClusterArch == arch {
					archMatch = true
					break
				}
			}
			if !archMatch {
				Skip(fmt.Sprintf("requires one of architectures: %v (cluster is %s)", []string{"amd64"}, config.Flags.ClusterArch))
			}
		}

		if !k8s.IsAPIGroupAvailable(sharedClients, "serving.knative.dev") {
			Skip("serving.knative.dev API not available on this cluster — Knative Serving is not installed")
		}

		ns := createTestNamespace("eco-kn-apply")
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")
		// kn tasks need permission to manage Knative Service resources in the namespace
		oc.AddRoleToServiceAccount("cluster-admin", "pipeline", ns)

		oc.Create("testdata/ecosystem/pipelineruns/kn-apply.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "kn-apply-run", "successful", ns)
	})
})

// PIPELINES-32-TC04
var _ = Describe("kn-apply p&z pipelinerun", Label("ecosystem", "e2e", "kn-apply"), func() {
	It("should create and verify kn-apply pipelinerun", func() {
		if len([]string{"ppc64le", "s390x"}) > 0 {
			archMatch := false
			for _, arch := range []string{"ppc64le", "s390x"} {
				if config.Flags.ClusterArch == arch {
					archMatch = true
					break
				}
			}
			if !archMatch {
				Skip(fmt.Sprintf("requires one of architectures: %v (cluster is %s)", []string{"ppc64le", "s390x"}, config.Flags.ClusterArch))
			}
		}

		if !k8s.IsAPIGroupAvailable(sharedClients, "serving.knative.dev") {
			Skip("serving.knative.dev API not available on this cluster — Knative Serving is not installed")
		}

		ns := createTestNamespace("eco-kn-apply-pz")
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")
		oc.AddRoleToServiceAccount("cluster-admin", "pipeline", ns)

		oc.Create("testdata/ecosystem/pipelineruns/kn-apply-multiarch.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "kn-apply-pz-run", "successful", ns)
	})
})

// PIPELINES-32-TC05
var _ = Describe("kn pipelinerun", Label("ecosystem", "e2e", "kn"), func() {
	It("should create and verify kn pipelinerun", func() {
		if len([]string{"amd64"}) > 0 {
			archMatch := false
			for _, arch := range []string{"amd64"} {
				if config.Flags.ClusterArch == arch {
					archMatch = true
					break
				}
			}
			if !archMatch {
				Skip(fmt.Sprintf("requires one of architectures: %v (cluster is %s)", []string{"amd64"}, config.Flags.ClusterArch))
			}
		}

		if !k8s.IsAPIGroupAvailable(sharedClients, "serving.knative.dev") {
			Skip("serving.knative.dev API not available on this cluster — Knative Serving is not installed")
		}

		ns := createTestNamespace("eco-kn")
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")
		oc.AddRoleToServiceAccount("cluster-admin", "pipeline", ns)

		oc.Create("testdata/ecosystem/pipelineruns/kn.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "kn-run", "successful", ns)
	})
})

// PIPELINES-32-TC06
var _ = Describe("kn p&z pipelinerun", Label("ecosystem", "e2e", "kn"), func() {
	It("should create and verify kn pipelinerun", func() {
		if len([]string{"ppc64le", "s390x"}) > 0 {
			archMatch := false
			for _, arch := range []string{"ppc64le", "s390x"} {
				if config.Flags.ClusterArch == arch {
					archMatch = true
					break
				}
			}
			if !archMatch {
				Skip(fmt.Sprintf("requires one of architectures: %v (cluster is %s)", []string{"ppc64le", "s390x"}, config.Flags.ClusterArch))
			}
		}

		if !k8s.IsAPIGroupAvailable(sharedClients, "serving.knative.dev") {
			Skip("serving.knative.dev API not available on this cluster — Knative Serving is not installed")
		}

		ns := createTestNamespace("eco-kn-pz")
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")
		oc.AddRoleToServiceAccount("cluster-admin", "pipeline", ns)

		oc.Create("testdata/ecosystem/pipelineruns/kn-pz.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "kn-pz-run", "successful", ns)
	})
})
