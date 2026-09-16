package tektonkueue

import (
	"context"
	"errors"
	"reflect"
	"testing"

	operatorv1alpha1 "github.com/tektoncd/operator/pkg/apis/operator/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
)

func TestCleanupStopsAtFailedBarrierAndRetriesRemainder(t *testing.T) {
	barrierErr := errors.New("cleanup barrier failed")
	environment := &Environment{}
	var order []int
	barrierAttempts := 0
	for i := 1; i <= 3; i++ {
		i := i
		environment.addCleanup(func(context.Context) error {
			order = append(order, i)
			if i == 2 {
				barrierAttempts++
				if barrierAttempts == 1 {
					return barrierErr
				}
			}
			return nil
		})
	}

	err := environment.Cleanup(context.Background())
	if !errors.Is(err, barrierErr) {
		t.Fatalf("Cleanup() error = %v, want %v", err, barrierErr)
	}
	if want := []int{3, 2}; !reflect.DeepEqual(order, want) {
		t.Fatalf("cleanup crossed failed barrier: order=%v, want %v", order, want)
	}
	if err := environment.Cleanup(context.Background()); err != nil {
		t.Fatalf("second Cleanup() = %v, want nil", err)
	}
	if want := []int{3, 2, 2, 1}; !reflect.DeepEqual(order, want) {
		t.Fatalf("cleanup retry order=%v, want %v", order, want)
	}
	if err := environment.Cleanup(context.Background()); err != nil {
		t.Fatalf("third Cleanup() = %v, want nil", err)
	}
	if !reflect.DeepEqual(order, []int{3, 2, 2, 1}) {
		t.Fatalf("third Cleanup() reran completed operations: %v", order)
	}
}

func TestEnsurePipelineRunIntegration(t *testing.T) {
	kueue := (&Environment{Prefix: "rtg-mk-test"}).requiredKueue()
	changed, err := ensurePipelineRunIntegration(kueue)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatal("complete Kueue configuration unexpectedly changed")
	}

	delete(kueue.Object["spec"].(map[string]any)["config"].(map[string]any), "multiKueue")
	changed, err = ensurePipelineRunIntegration(kueue)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 {
		t.Fatalf("changed paths = %v, want one missing integration", changed)
	}
	changed, err = ensurePipelineRunIntegration(kueue)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatal("integration update was not idempotent")
	}
}

func TestScopedKubeconfig(t *testing.T) {
	data, err := scopedKubeconfig(&rest.Config{Host: "https://worker.example.test", BearerToken: "must-not-copy"}, "scoped-token", []byte("test-ca"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := clientcmd.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if config.Clusters["worker"].Server != "https://worker.example.test" {
		t.Fatalf("server = %q", config.Clusters["worker"].Server)
	}
	if config.AuthInfos["worker"].Token != "scoped-token" {
		t.Fatalf("token = %q, want scoped token", config.AuthInfos["worker"].Token)
	}
	if string(config.Clusters["worker"].CertificateAuthorityData) != "test-ca" {
		t.Fatalf("CA = %q", config.Clusters["worker"].CertificateAuthorityData)
	}
}

func TestRestoreSchedulerFieldsPreservesConcurrentOptions(t *testing.T) {
	disabled := true
	original := &operatorv1alpha1.Scheduler{Disabled: &disabled, MultiClusterConfig: operatorv1alpha1.MultiClusterConfig{MultiClusterDisabled: true}}
	original.QueueName = "original"
	original.Options.ConfigMaps = map[string]corev1.ConfigMap{"existing": {}}

	installed := original.DeepCopy()
	enabled := false
	installed.Disabled = &enabled
	installed.MultiClusterDisabled = false
	installed.MultiClusterRole = operatorv1alpha1.MultiClusterRoleHub
	installed.QueueName = "rtg-mk-test"
	installed.MultiKueueOverride = true
	installed.Options.ConfigMaps["concurrent"] = corev1.ConfigMap{}

	if conflicts := restoreSchedulerFields(installed, original, operatorv1alpha1.MultiClusterRoleHub, "rtg-mk-test"); len(conflicts) != 0 {
		t.Fatalf("restoreSchedulerFields() conflicts = %v", conflicts)
	}
	if installed.Disabled == nil || !*installed.Disabled || !installed.MultiClusterDisabled || installed.MultiClusterRole != "" || installed.QueueName != "original" || installed.MultiKueueOverride {
		t.Fatalf("scheduler fields were not restored: %+v", installed)
	}
	if _, found := installed.Options.ConfigMaps["concurrent"]; !found {
		t.Fatal("concurrent scheduler options were overwritten")
	}

	installed.QueueName = "administrator-change"
	if conflicts := restoreSchedulerFields(installed, original, operatorv1alpha1.MultiClusterRoleHub, "rtg-mk-test"); len(conflicts) == 0 {
		t.Fatal("concurrent queueName change was overwritten")
	}
	if installed.QueueName != "administrator-change" {
		t.Fatalf("queueName = %q, want concurrent value", installed.QueueName)
	}
}

func TestWorkerRBACLimitsClusterWideAccessToReadOnly(t *testing.T) {
	readRole := workerReadClusterRole("test")
	for _, rule := range readRole.Rules {
		for _, verb := range rule.Verbs {
			if verb != "get" && verb != "list" && verb != "watch" {
				t.Fatalf("cluster-wide worker role grants destructive verb %q", verb)
			}
		}
	}
	for _, rule := range append(readRole.Rules, workerWriteRole("test").Rules...) {
		for _, resource := range rule.Resources {
			if rule.APIGroups[0] == "" || resource == "secrets" || resource == "pods" || resource == "taskruns" || resource == "taskruns/status" {
				t.Fatalf("worker RBAC grants unrelated resource %q in API group %q", resource, rule.APIGroups[0])
			}
		}
	}
}

func TestSpokeAPIPeersRestrictDestinations(t *testing.T) {
	environment := &Environment{Spokes: []Cluster{
		{Name: "spoke-1", Clients: &clients.Clients{KubeConfig: &rest.Config{Host: "https://192.0.2.10:443"}}},
		{Name: "spoke-2", Clients: &clients.Clients{KubeConfig: &rest.Config{Host: "https://[2001:db8::10]:6443"}}},
	}}
	peers, ports, err := environment.spokeAPIPeers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 || len(ports) != 2 {
		t.Fatalf("peers=%v ports=%v, want two restricted peers and ports", peers, ports)
	}
	for _, peer := range peers {
		if peer.IPBlock == nil || (peer.IPBlock.CIDR != "192.0.2.10/32" && peer.IPBlock.CIDR != "2001:db8::10/128") {
			t.Fatalf("unexpected egress peer: %+v", peer)
		}
	}
}

func TestEnvironmentRejectsDuplicateClusters(t *testing.T) {
	cluster := func(name string) Cluster {
		return Cluster{Name: name, Clients: &clients.Clients{KubeConfig: &rest.Config{Host: "https://same.example.test"}}}
	}
	environment := NewEnvironment(cluster("hub"), []Cluster{cluster("spoke-1")}, "rtg-mk-test")
	if err := environment.validate(); err == nil {
		t.Fatal("validate() accepted duplicate API servers")
	}
}
