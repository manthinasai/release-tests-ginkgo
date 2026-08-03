package chains_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
)

var oc = occmd.OC{}

var _ = Describe("Tekton Chains: PIPELINES-27", Label("chains", "e2e"), func() {

	Describe("Using Tekton Chains to create and verify task run signatures: PIPELINES-27-TC01", Label("sanity", "taskrun"), Ordered, ContinueOnFailure, func() {
		BeforeAll(func() {
			// Update TektonConfig for taskrun signing
			operator.UpdateTektonConfigForChains("in-toto", "tekton", "", "false")
			DeferCleanup(operator.RestoreTektonConfigChains)

			// Store cosign public key
			err := operator.CreateFileWithCosignPubKey()
			Expect(err).NotTo(HaveOccurred(), "Failed to store cosign public key")
		})

		It("applies the task-output-image task and verifies taskrun signature", func() {
			oc.Apply("testdata/chains/task-output-image.yaml")

			err := operator.VerifySignature("taskrun")
			Expect(err).NotTo(HaveOccurred(), "Failed to verify taskrun signature")
		})
	})

	Describe("Using Tekton Chains to sign and verify image and provenance: PIPELINES-27-TC02", Label("image"), Ordered, ContinueOnFailure, func() {
		BeforeAll(func() {
			if os.Getenv("CHAINS_REPOSITORY") == "" {
				Skip("CHAINS_REPOSITORY not set -- skipping image signature test")
			}

			if os.Getenv("CHAINS_DOCKER_CONFIG_JSON") == "" {
				Skip("CHAINS_DOCKER_CONFIG_JSON not set -- skipping image signature test")
			}

			// Update TektonConfig for image signing with OCI storage
			operator.UpdateTektonConfigForChains("in-toto", "oci", "oci", "true")
			DeferCleanup(operator.RestoreTektonConfigChains)

			// Store cosign public key
			err := operator.CreateFileWithCosignPubKey()
			Expect(err).NotTo(HaveOccurred(), "Failed to store cosign public key")

			// Create image registry credentials secret
			oc.CreateChainsImageRegistrySecret(os.Getenv("CHAINS_DOCKER_CONFIG_JSON"))
		})

		It("starts kaniko task and applies chain resources", func() {
			oc.Apply("testdata/pvc/chains-pvc.yaml")
			oc.Apply("testdata/chains/kaniko.yaml")
			operator.StartKanikoTask()
		})

		It("verifies image signature", func() {
			err := operator.VerifyImageSignature()
			Expect(err).NotTo(HaveOccurred(), "Failed to verify image signature")
		})

		It("checks attestation exists in transparency log", func() {
			err := operator.CheckAttestationExists()
			Expect(err).NotTo(HaveOccurred(), "Failed to find attestation")
		})

		It("verifies attestation", func() {
			err := operator.VerifyAttestation()
			Expect(err).NotTo(HaveOccurred(), "Failed to verify attestation")
		})
	})
})
