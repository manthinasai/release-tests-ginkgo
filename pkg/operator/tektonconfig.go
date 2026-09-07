package operator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"

	"github.com/tektoncd/operator/pkg/apis/operator/v1alpha1"
	configv1alpha1 "github.com/tektoncd/operator/pkg/client/clientset/versioned/typed/operator/v1alpha1"
	"github.com/tektoncd/operator/test/utils"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureTektonConfigExists creates a TektonConfig with the name names.TektonConfig, if it does not exist.
func EnsureTektonConfigExists(clients configv1alpha1.TektonConfigInterface, names utils.ResourceNames) (*v1alpha1.TektonConfig, error) {
	tcCR, err := clients.Get(context.TODO(), names.TektonConfig, metav1.GetOptions{})
	if err == nil {
		return tcCR, err
	}

	err = wait.PollUntilContextTimeout(context.TODO(), config.APIRetry, config.APITimeout, false, func(context.Context) (bool, error) {
		tcCR, err = clients.Get(context.TODO(), names.TektonConfig, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				log.Printf("Waiting for availability of %s cr\n", names.TektonConfig)
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	return tcCR, err
}

// EnsureTektonConfigStatusInstalled waits until TektonConfig CR reports InstallSucceeded.
func EnsureTektonConfigStatusInstalled(clients configv1alpha1.TektonConfigInterface, names utils.ResourceNames) {
	err := wait.PollUntilContextTimeout(context.TODO(), config.APIRetry, config.APITimeout, true, func(context.Context) (bool, error) {
		cr, err := EnsureTektonConfigExists(clients, names)
		if err != nil {
			Fail(fmt.Sprintf("TektonConfig CR error: %v", err))
		}
		for _, cc := range cr.Status.Conditions {
			if cc.Type != "InstallSucceeded" && cc.Status != "True" {
				log.Printf("Waiting for %s cr InstalledStatus Actual: [%s] Expected: [True]\n", names.TektonConfig, cc.Status)
				return false, nil
			}
		}
		return true, nil
	})
	Expect(err).NotTo(HaveOccurred(), "TektonConfig failed to reach installed status")
}

// IsTektonConfigReady will check the status conditions of the TektonConfig and return true if the TektonConfig is ready.
func IsTektonConfigReady(s *v1alpha1.TektonConfig, err error) (bool, error) {
	return s.Status.IsReady(), err
}

// AssertTektonConfigCRReadyStatus verifies if the TektonConfig reaches the READY status.
func AssertTektonConfigCRReadyStatus(cs *clients.Clients, names utils.ResourceNames) {
	var lastState *v1alpha1.TektonConfig
	waitErr := wait.PollUntilContextTimeout(context.TODO(), config.APIRetry, config.APITimeout, true, func(context.Context) (bool, error) {
		lastState, err := cs.TektonConfig().Get(context.TODO(), names.TektonConfig, metav1.GetOptions{})
		return IsTektonConfigReady(lastState, err)
	})
	Expect(waitErr).NotTo(HaveOccurred(),
		"TektonConfigCR %q failed to get to the READY status: got %+v", names.TektonConfig, lastState)
}

// TektonConfigCRDelete deletes the TektonConfig to verify all resources will be deleted.
func TektonConfigCRDelete(cs *clients.Clients, crNames utils.ResourceNames) {
	err := cs.TektonConfig().Delete(context.TODO(), crNames.TektonConfig, metav1.DeleteOptions{})
	Expect(err).NotTo(HaveOccurred(), "TektonConfigCR %q failed to delete", crNames.TektonConfig)

	err = wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, true, func(context.Context) (bool, error) {
		_, err := cs.TektonConfig().Get(context.TODO(), crNames.TektonConfig, metav1.GetOptions{})
		if apierrs.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
	Expect(err).NotTo(HaveOccurred(), "timed out waiting on TektonConfigCR to delete")

	verifyNoTektonConfigCR(cs)
}

func verifyNoTektonConfigCR(cs *clients.Clients) {
	configs, err := cs.TektonConfig().List(context.TODO(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred(), "failed to list TektonConfig CRs")
	Expect(configs.Items).To(BeEmpty(), "expected no TektonConfig CRs to exist")
}

// GetTektonConfigField returns a jsonpath field value from the TektonConfig CR.
func GetTektonConfigField(jsonpath string) string {
	result := cmd.Run("oc", "get", "TektonConfig", "config", "-o", "jsonpath="+jsonpath)
	return strings.TrimSpace(result.Stdout())
}

// Ensure the unused import warning doesn't fire.
var (
	_ = GinkgoWriter
	_ = errors.New
)
