/*
Copyright 2020 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package operator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tektoncd/operator/pkg/apis/operator/v1alpha1"
	chainv1alpha "github.com/tektoncd/operator/pkg/client/clientset/versioned/typed/operator/v1alpha1"
	"github.com/tektoncd/operator/test/utils"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

// "quay.io/openshift-pipeline/chainstest"
var repo = os.Getenv("CHAINS_REPOSITORY")
var publicKeyPath = config.Path("testdata/chains/key")

// EnsureTektonChainsExists waits until a TektonChain CR with the given name is ready.
func EnsureTektonChainsExists(clients chainv1alpha.TektonChainInterface, names utils.ResourceNames) (*v1alpha1.TektonChain, error) {
	ks, err := clients.Get(context.TODO(), names.TektonChain, metav1.GetOptions{})
	err = wait.PollUntilContextTimeout(context.TODO(), config.APIRetry, config.APITimeout, false, func(context.Context) (bool, error) {
		ks, err = clients.Get(context.TODO(), names.TektonChain, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				log.Printf("Waiting for availability of chains cr [%s]\n", names.TektonChain)
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	return ks, err
}

// UpdateTektonConfigForChains patches the TektonConfig CR to configure Tekton Chains
// with the given format, taskrun storage, OCI storage, and transparency settings.
func UpdateTektonConfigForChains(format, taskrunStorage, ociStorage, transparency string) {
	patchData := fmt.Sprintf(`{"spec":{"chain":{"options":{"disabled":false},"artifacts.taskrun.format":"%s","artifacts.taskrun.storage":"%s","artifacts.oci.storage":"%s","transparency.enabled":"%s"}}}`,
		format, taskrunStorage, ociStorage, transparency)
	cmd.MustSucceed("oc", "patch", "tektonconfig", "config", "-p", patchData, "--type=merge")
	log.Printf("Updated TektonConfig for chains: format=%s, taskrunStorage=%s, ociStorage=%s, transparency=%s", format, taskrunStorage, ociStorage, transparency)
}

// RestoreTektonConfigChains restores the TektonConfig chains settings to defaults.
func RestoreTektonConfigChains() {
	patchData := `{"spec":{"chain":{"options":{"disabled":false},"artifacts.taskrun.format":"in-toto","artifacts.taskrun.storage":"tekton","artifacts.oci.storage":"","transparency.enabled":"false"}}}`
	cmd.MustSucceed("oc", "patch", "tektonconfig", "config", "-p", patchData, "--type=merge")
	log.Println("Restored TektonConfig chains settings to defaults")
}

const (
	chainsSignatureInterval = 10 * time.Second
	chainsSignatureTimeout  = 3 * time.Minute
)

// VerifySignature verifies that a Tekton Chains signature exists for the given resource type.
// It polls every 10 seconds for up to 3 minutes waiting for both the
// chains.tekton.dev/signed and chains.tekton.dev/signature-<type>-<uid> annotations to be set.
func VerifySignature(resourceType string) error {
	ns := store.Namespace()
	if ns == "" {
		return fmt.Errorf("VerifySignature: store.Namespace() is empty - ensure hooks are configured")
	}

	// Get the UID of the last resource (created before polling starts).
	resourceUID := cmd.MustSucceed("opc", resourceType, "describe", "--last", "-o", "jsonpath='{.metadata.uid}'", "-n", ns).Stdout()
	resourceUID = strings.Trim(resourceUID, "'")

	sigJSONPath := fmt.Sprintf("jsonpath=\"{.metadata.annotations.chains\\.tekton\\.dev/signature-%s-%s}\"", resourceType, resourceUID)
	signedJSONPath := "jsonpath=\"{.metadata.annotations.chains\\.tekton\\.dev/signed}\""

	log.Printf("Polling for Chains signature on %s (uid=%s), timeout=%s", resourceType, resourceUID, chainsSignatureTimeout)

	var signature, isSigned string
	ctx, cancel := context.WithTimeout(context.Background(), chainsSignatureTimeout)
	defer cancel()

	pollErr := wait.PollUntilContextTimeout(ctx, chainsSignatureInterval, chainsSignatureTimeout, true, func(context.Context) (bool, error) {
		isSigned = strings.Trim(cmd.MustSucceed("opc", resourceType, "describe", "--last", "-o", signedJSONPath, "-n", ns).Stdout(), "\"")
		signature = strings.Trim(cmd.MustSucceed("opc", resourceType, "describe", "--last", "-o", sigJSONPath, "-n", ns).Stdout(), "\"")

		if isSigned == "true" && len(signature) > 0 {
			return true, nil
		}
		log.Printf("  chains.tekton.dev/signed=%q, signature present=%v — retrying in %s", isSigned, len(signature) > 0, chainsSignatureInterval)
		return false, nil
	})
	if pollErr != nil {
		return fmt.Errorf("timed out waiting for Chains to sign %s (uid=%s): signed=%q, signature present=%v",
			resourceType, resourceUID, isSigned, len(signature) > 0)
	}

	// Decode the signature and verify with cosign.
	decodedSignature, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("error decoding base64 signature: %w", err)
	}
	file, err := os.Create("sign")
	if err != nil {
		return fmt.Errorf("error creating signature file: %w", err)
	}
	//nolint:errcheck
	defer file.Close()
	if _, err = file.WriteString(string(decodedSignature)); err != nil {
		return fmt.Errorf("error writing signature file: %w", err)
	}
	cmd.MustSucceed("cosign", "verify-blob-attestation", "--insecure-ignore-tlog", "--key", publicKeyPath+"/cosign.pub", "--signature", "sign", "--type", "slsaprovenance", "--check-claims=false", "/dev/null")
	return nil
}

// StartKanikoTask starts a Kaniko TaskRun to build and push an image, used for chains signing tests.
func StartKanikoTask() {
	ns := store.Namespace()
	if ns == "" {
		panic("StartKanikoTask: store.Namespace() is empty - ensure hooks are configured")
	}
	var tag = time.Now().Format("060102150405")
	cmd.MustSucceed("oc", "secrets", "link", "pipeline", "chains-image-registry-credentials", "--for=pull,mount", "-n", ns)
	image := fmt.Sprintf("IMAGE=%s:%s", repo, tag)
	cmd.MustSucceed("opc", "task", "start", "--param", image, "--use-param-defaults", "--workspace", "name=source,claimName=chains-pvc", "--workspace", "name=dockerconfig,secret=chains-image-registry-credentials", "kaniko-chains", "-n", ns)
	log.Println("Waiting 2 minutes for images to appear in image registry")
	cmd.MustSucceedIncreasedTimeout(time.Second*130, "sleep", "120")
}

// GetImageURLAndDigest returns the image URL and digest from the completed Kaniko TaskRun.
func GetImageURLAndDigest() (string, string, error) {
	ns := store.Namespace()
	if ns == "" {
		return "", "", fmt.Errorf("GetImageURLAndDigest: store.Namespace() is empty - ensure hooks are configured")
	}
	// Get Image digest
	var imageDigest string
	jsonOutput := cmd.MustSucceed("opc", "tr", "describe", "--last", "-o", "jsonpath={.status.results}", "-n", ns).Stdout()
	// Parse Json Output
	type Result struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}

	var results []Result
	err := json.Unmarshal([]byte(jsonOutput), &results)
	if err != nil {
		return "", "", fmt.Errorf("error parsing JSON output: %w", err)
	}

	// Get IMAGE_DIGEST value
	for _, result := range results {
		if strings.Contains(result.Name, "IMAGE_DIGEST") {
			imageDigest = strings.Split(result.Value, ":")[1]
		}
	}

	// Return image url with digest
	url := fmt.Sprintf("%s@sha256:%s", repo, imageDigest)
	return url, imageDigest, nil
}

// VerifyImageSignature verifies that the Kaniko-built image has a valid cosign signature.
func VerifyImageSignature() error {
	url, _, err := GetImageURLAndDigest()
	if err != nil {
		return err
	}
	cmd.MustSucceed("cosign", "verify", "--key", publicKeyPath+"/cosign.pub", url)
	return nil
}

// VerifyAttestation verifies that an attestation exists for the Kaniko-built image.
func VerifyAttestation() error {
	url, _, err := GetImageURLAndDigest()
	if err != nil {
		return err
	}
	cmd.MustSucceed("cosign", "verify-attestation", "--key", publicKeyPath+"/cosign.pub", "--type", "slsaprovenance", url)
	return nil
}

// CheckAttestationExists checks whether a cosign attestation exists for the built image.
// It polls Rekor for up to 5 minutes because indexing can lag after Chains uploads the attestation.
func CheckAttestationExists() error {
	_, imageDigest, err := GetImageURLAndDigest()
	if err != nil {
		return err
	}

	type uuidList struct {
		UUIDs []string `json:"UUIDs"`
	}

	var rekorUUID string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := wait.PollUntilContextTimeout(ctx, 15*time.Second, 5*time.Minute, true, func(_ context.Context) (bool, error) {
		result := cmd.Run("rekor-cli", "search", "--format", "json", "--sha", imageDigest)
		if result.ExitCode != 0 {
			log.Printf("Rekor entry not ready yet (exit %d): %s", result.ExitCode, result.Stderr())
			return false, nil
		}
		var u uuidList
		if err := json.Unmarshal([]byte(result.Stdout()), &u); err != nil || len(u.UUIDs) == 0 {
			return false, nil
		}
		rekorUUID = u.UUIDs[0]
		return true, nil
	}); err != nil {
		return fmt.Errorf("attestation not found in transparency log within 5 minutes for digest %s", imageDigest)
	}

	if strings.Contains(cmd.Run("rekor-cli", "get", "--uuid", rekorUUID).Stdout(), "getLogEntryByUuidNotFound") {
		return fmt.Errorf("failed to find attestation for UUID %s", rekorUUID)
	}
	return nil
}

const (
	signingSecretInterval     = 10 * time.Second
	signingSecretTimeout      = 2 * time.Minute
	signingSecretRetryTrigger = 3 // after this many empty polls, trigger key generation
)

// CreateFileWithCosignPubKey checks whether the Chains signing keypair already
// exists in the "signing-secrets" secret (openshift-pipelines). If cosign.pub is
// populated it is written to disk immediately. If after 3 polls it is still empty,
// generateSigningSecret=true is patched into TektonConfig to trigger key generation,
// and polling continues until the key appears or the 2-minute timeout is reached.
//
// Background: Chains uses the x509 signer by default. The keypair lives in
// Secret "signing-secrets" (openshift-pipelines). It is normally created by the
// OLM install suite via EnableChainsSigningSecret, but may not be present when
// running the chains suite standalone against a pre-installed cluster.
func CreateFileWithCosignPubKey() error {
	log.Printf("Checking signing-secrets/cosign.pub (will trigger generation after %d empty polls)", signingSecretRetryTrigger)

	var chainsPublicKey string
	attempt := 0
	triggered := false

	ctx, cancel := context.WithTimeout(context.Background(), signingSecretTimeout)
	defer cancel()

	pollErr := wait.PollUntilContextTimeout(ctx, signingSecretInterval, signingSecretTimeout, true, func(context.Context) (bool, error) {
		attempt++
		raw := cmd.MustSucceed("oc", "get", "secrets", "signing-secrets", "-n", "openshift-pipelines",
			"-o", "jsonpath='{.data.cosign\\.pub}'").Stdout()
		raw = strings.Trim(raw, "'")
		if len(raw) > 0 {
			chainsPublicKey = raw
			return true, nil
		}
		if attempt >= signingSecretRetryTrigger && !triggered {
			log.Printf("  cosign.pub still empty after %d attempts — patching TektonConfig to trigger key generation", attempt)
			patchData := `{"spec":{"chain":{"generateSigningSecret":true}}}`
			cmd.MustSucceed("oc", "patch", "tektonconfig", "config", "-p", patchData, "--type=merge")
			triggered = true
		}
		log.Printf("  signing-secrets/cosign.pub not populated yet (attempt %d) — retrying in %s", attempt, signingSecretInterval)
		return false, nil
	})
	if pollErr != nil {
		return fmt.Errorf("timed out waiting for signing-secrets/cosign.pub after %d attempts (generateSigningSecret triggered=%v): %w",
			attempt, triggered, pollErr)
	}

	decodedPublicKey, err := base64.StdEncoding.DecodeString(chainsPublicKey)
	if err != nil {
		return fmt.Errorf("error decoding cosign.pub from base64: %w", err)
	}
	cmd.MustSucceed("mkdir", "-p", publicKeyPath)
	fullPath := filepath.Join(publicKeyPath, "cosign.pub")
	file, err := os.Create(filepath.Clean(fullPath))
	if err != nil {
		return fmt.Errorf("error creating cosign.pub file: %w", err)
	}
	//nolint:errcheck
	defer file.Close()
	if _, err = file.WriteString(string(decodedPublicKey)); err != nil {
		return fmt.Errorf("error writing cosign.pub file: %w", err)
	}
	log.Printf("cosign.pub written to %s", fullPath)
	return nil
}
