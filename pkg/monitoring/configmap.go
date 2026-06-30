package monitoring

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
)

// VerifyConfigMapKeyExists checks that a key exists in the active data of a ConfigMap.
func VerifyConfigMapKeyExists(cs *clients.Clients, namespace, cmName, key string) error {
	cm, err := cs.KubeClient.Kube.CoreV1().ConfigMaps(namespace).Get(context.Background(), cmName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, cmName, err)
	}
	if _, ok := cm.Data[key]; !ok {
		return fmt.Errorf("key %q not found in ConfigMap %s/%s", key, namespace, cmName)
	}
	return nil
}

// VerifyConfigMapKeyAbsent checks that a key does NOT exist in the active data of a ConfigMap.
func VerifyConfigMapKeyAbsent(cs *clients.Clients, namespace, cmName, key string) error {
	cm, err := cs.KubeClient.Kube.CoreV1().ConfigMaps(namespace).Get(context.Background(), cmName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, cmName, err)
	}
	if _, ok := cm.Data[key]; ok {
		return fmt.Errorf("key %q should not exist in ConfigMap %s/%s but was found", key, namespace, cmName)
	}
	return nil
}
