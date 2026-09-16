package tektonkueue

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"
)

var (
	resourceFlavorGVR    = kueueGVR("resourceflavors")
	clusterQueueGVR      = kueueGVR("clusterqueues")
	localQueueGVR        = kueueGVR("localqueues")
	admissionCheckGVR    = kueueGVR("admissionchecks")
	multiKueueConfigGVR  = kueueGVR("multikueueconfigs")
	multiKueueClusterGVR = kueueGVR("multikueueclusters")
	workloadGVR          = kueueGVR("workloads")
)

func kueueGVR(resource string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "kueue.x-k8s.io", Version: "v1beta2", Resource: resource}
}

func (e *Environment) createNamespace(ctx context.Context, cluster Cluster) error {
	namespaces := cluster.Clients.KubeClient.Kube.CoreV1().Namespaces()
	if _, err := namespaces.Get(ctx, e.Namespace, metav1.GetOptions{}); err == nil {
		return fmt.Errorf("namespace %s already exists", e.Namespace)
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	labels := e.ownedLabels()
	var uid types.UID
	e.addCleanup(func(cleanupCtx context.Context) error {
		current, getErr := namespaces.Get(cleanupCtx, e.Namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		if !e.owns(current.Labels) || (uid != "" && current.UID != uid) {
			return fmt.Errorf("refusing to delete replacement namespace %s on %s", e.Namespace, cluster.Name)
		}
		if err := ignoreNotFound(namespaces.Delete(cleanupCtx, e.Namespace, metav1.DeleteOptions{})); err != nil {
			return err
		}
		return waitForNotFound(cleanupCtx, 10*time.Minute, func(ctx context.Context) error {
			_, err := namespaces.Get(ctx, e.Namespace, metav1.GetOptions{})
			return err
		})
	})

	created, err := namespaces.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: e.Namespace, Labels: labels}}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	uid = created.UID
	return nil
}

func (e *Environment) createKueueControllerRBAC(ctx context.Context, cluster Cluster) error {
	name := e.Prefix + "-kueue-controller"
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"tekton.dev"},
			Resources: []string{"pipelineruns", "pipelineruns/status"},
			Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		}},
	}
	if err := e.createClusterRole(ctx, cluster, role); err != nil {
		return err
	}
	return e.createClusterRoleBinding(ctx, cluster, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "kueue-controller-manager", Namespace: kueueNamespace}},
	})
}

func (e *Environment) createQueues(ctx context.Context, cluster Cluster, hub bool) error {
	flavor := object("ResourceFlavor", e.Prefix, "", nil)
	if err := e.createDynamic(ctx, cluster, resourceFlavorGVR, flavor); err != nil {
		return err
	}

	quota := int64(1)
	if !hub {
		// The worker temporarily holds the copied MultiKueue Workload and the
		// PipelineRun-owned Workload, so one execution needs two quota units.
		quota = 2
	}
	clusterQueueSpec := map[string]any{
		"namespaceSelector": map[string]any{},
		"resourceGroups": []any{map[string]any{
			"coveredResources": []any{"tekton.dev/pipelineruns"},
			"flavors": []any{map[string]any{
				"name": e.Prefix,
				"resources": []any{map[string]any{
					"name":         "tekton.dev/pipelineruns",
					"nominalQuota": quota,
				}},
			}},
		}},
	}
	if hub {
		clusterQueueSpec["admissionChecksStrategy"] = map[string]any{
			"admissionChecks": []any{map[string]any{"name": e.Prefix}},
		}
	}
	clusterQueue := object("ClusterQueue", e.Prefix, "", clusterQueueSpec)
	if err := e.createDynamic(ctx, cluster, clusterQueueGVR, clusterQueue); err != nil {
		return err
	}

	localQueue := object("LocalQueue", e.Prefix, e.Namespace, map[string]any{"clusterQueue": e.Prefix})
	return e.createDynamic(ctx, cluster, localQueueGVR, localQueue)
}

func (e *Environment) createWorkerCredential(ctx context.Context, spoke Cluster) error {
	name := e.workerName(spoke)
	serviceAccounts := spoke.Clients.KubeClient.Kube.CoreV1().ServiceAccounts(e.Namespace)
	if _, err := serviceAccounts.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: e.Namespace, Labels: e.ownedLabels(),
	}}, metav1.CreateOptions{}); err != nil {
		return err
	}

	if err := e.createClusterRole(ctx, spoke, workerReadClusterRole(name)); err != nil {
		return err
	}
	if err := e.createClusterRoleBinding(ctx, spoke, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: e.Namespace}},
	}); err != nil {
		return err
	}
	if err := e.createRole(ctx, spoke, workerWriteRole(name)); err != nil {
		return err
	}
	if err := e.createRoleBinding(ctx, spoke, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: e.Namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: e.Namespace}},
	}); err != nil {
		return err
	}

	expirationSeconds := int64((3 * time.Hour).Seconds())
	token, err := serviceAccounts.CreateToken(ctx, name, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &expirationSeconds},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create bounded service-account token: %w", err)
	}
	var ca []byte
	configMaps := spoke.Clients.KubeClient.Kube.CoreV1().ConfigMaps(e.Namespace)
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		configMap, getErr := configMaps.Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
		if getErr != nil {
			return false, nil
		}
		ca = []byte(configMap.Data["ca.crt"])
		return len(ca) != 0, nil
	}); err != nil {
		return fmt.Errorf("worker root CA was not populated: %w", err)
	}
	kubeconfig, err := scopedKubeconfig(spoke.Clients.KubeConfig, token.Status.Token, ca)
	if err != nil {
		return err
	}
	if err := verifyWorkerCredentialScope(ctx, kubeconfig, e.Namespace); err != nil {
		return err
	}
	hubSecrets := e.Hub.Clients.KubeClient.Kube.CoreV1().Secrets(kueueNamespace)
	var hubSecretUID types.UID
	e.addCleanup(func(cleanupCtx context.Context) error {
		current, getErr := hubSecrets.Get(cleanupCtx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		if !e.owns(current.Labels) || (hubSecretUID != "" && current.UID != hubSecretUID) {
			return fmt.Errorf("refusing to delete replacement worker credential for %s", spoke.Name)
		}
		return ignoreNotFound(hubSecrets.Delete(cleanupCtx, name, metav1.DeleteOptions{}))
	})
	hubSecret, err := hubSecrets.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: kueueNamespace, Labels: e.ownedLabels()},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"kubeconfig": kubeconfig},
	}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	hubSecretUID = hubSecret.UID
	return nil
}

func workerReadClusterRole(name string) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"kueue.x-k8s.io"}, Resources: []string{"workloads"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"tekton.dev"}, Resources: []string{"pipelineruns"}, Verbs: []string{"get", "list", "watch"}},
		},
	}
}

func workerWriteRole(name string) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"kueue.x-k8s.io"}, Resources: []string{"workloads"}, Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{APIGroups: []string{"kueue.x-k8s.io"}, Resources: []string{"workloads/status"}, Verbs: []string{"get", "patch", "update"}},
			{APIGroups: []string{"tekton.dev"}, Resources: []string{"pipelineruns"}, Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{APIGroups: []string{"tekton.dev"}, Resources: []string{"pipelineruns/status"}, Verbs: []string{"get", "patch", "update"}},
		},
	}
}

func verifyWorkerCredentialScope(ctx context.Context, kubeconfig []byte, namespace string) error {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("parse scoped worker credential: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create scoped worker client: %w", err)
	}
	allowed := func(attributes authorizationv1.ResourceAttributes) (bool, error) {
		review, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &attributes},
		}, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		return review.Status.Allowed, nil
	}
	err = wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(context.Context) (bool, error) {
		return allowed(authorizationv1.ResourceAttributes{Verb: "create", Group: "tekton.dev", Resource: "pipelineruns", Namespace: namespace})
	})
	if err != nil {
		return fmt.Errorf("worker credential cannot create PipelineRuns in the test namespace: %w", err)
	}
	canCreateElsewhere, err := allowed(authorizationv1.ResourceAttributes{Verb: "create", Group: "tekton.dev", Resource: "pipelineruns", Namespace: "default"})
	if err != nil {
		return err
	}
	if canCreateElsewhere {
		return fmt.Errorf("worker credential can mutate PipelineRuns outside the test namespace")
	}
	canReadSecrets, err := allowed(authorizationv1.ResourceAttributes{Verb: "get", Resource: "secrets", Namespace: namespace})
	if err != nil {
		return err
	}
	if canReadSecrets {
		return fmt.Errorf("worker credential can read Secrets")
	}
	return nil
}

func scopedKubeconfig(config *rest.Config, token string, ca []byte) ([]byte, error) {
	if config == nil || config.Host == "" || token == "" || len(ca) == 0 {
		return nil, fmt.Errorf("REST config, token, and CA are required")
	}
	const name = "worker"
	return clientcmd.Write(clientcmdapi.Config{
		Clusters:       map[string]*clientcmdapi.Cluster{name: {Server: config.Host, CertificateAuthorityData: ca}},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{name: {Token: token}},
		Contexts:       map[string]*clientcmdapi.Context{name: {Cluster: name, AuthInfo: name}},
		CurrentContext: name,
	})
}

func (e *Environment) createHubMultiKueueResources(ctx context.Context) error {
	clusters := make([]any, 0, len(e.Spokes))
	for _, spoke := range e.Spokes {
		name := e.workerName(spoke)
		clusters = append(clusters, name)
		cluster := object("MultiKueueCluster", name, "", map[string]any{
			"clusterSource": map[string]any{
				"kubeConfig": map[string]any{"locationType": "Secret", "location": name},
			},
		})
		if err := e.createDynamic(ctx, e.Hub, multiKueueClusterGVR, cluster); err != nil {
			return fmt.Errorf("create MultiKueueCluster for %s: %w", spoke.Name, err)
		}
	}

	config := object("MultiKueueConfig", e.Prefix, "", map[string]any{"clusters": clusters})
	if err := e.createDynamic(ctx, e.Hub, multiKueueConfigGVR, config); err != nil {
		return err
	}
	admissionCheck := object("AdmissionCheck", e.Prefix, "", map[string]any{
		"controllerName": "kueue.x-k8s.io/multikueue",
		"parameters": map[string]any{
			"apiGroup": "kueue.x-k8s.io",
			"kind":     "MultiKueueConfig",
			"name":     e.Prefix,
		},
	})
	return e.createDynamic(ctx, e.Hub, admissionCheckGVR, admissionCheck)
}

func (e *Environment) createKueueEgressPolicy(ctx context.Context) error {
	peers, ports, err := e.spokeAPIPeers(ctx)
	if err != nil {
		return err
	}
	policies := e.Hub.Clients.KubeClient.Kube.NetworkingV1().NetworkPolicies(kueueNamespace)
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: e.Prefix, Namespace: kueueNamespace, Labels: e.ownedLabels()},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app.openshift.io/name": "kueue"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      []networkingv1.NetworkPolicyEgressRule{{To: peers, Ports: ports}},
		},
	}
	var uid types.UID
	e.addCleanup(func(cleanupCtx context.Context) error {
		current, getErr := policies.Get(cleanupCtx, policy.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		if !e.owns(current.Labels) || (uid != "" && current.UID != uid) {
			return fmt.Errorf("refusing to delete replacement Kueue egress policy")
		}
		return ignoreNotFound(policies.Delete(cleanupCtx, policy.Name, metav1.DeleteOptions{}))
	})
	created, err := policies.Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create temporary Kueue egress policy: %w", err)
	}
	uid = created.UID
	return nil
}

func (e *Environment) spokeAPIPeers(ctx context.Context) ([]networkingv1.NetworkPolicyPeer, []networkingv1.NetworkPolicyPort, error) {
	cidrs := map[string]bool{}
	ports := map[int]bool{}
	for _, spoke := range e.Spokes {
		endpoint, err := url.Parse(spoke.Clients.KubeConfig.Host)
		if err != nil || endpoint.Hostname() == "" {
			return nil, nil, fmt.Errorf("invalid API endpoint for %s", spoke.Name)
		}
		port := 443
		if endpoint.Port() != "" {
			port, err = strconv.Atoi(endpoint.Port())
			if err != nil {
				return nil, nil, fmt.Errorf("invalid API port for %s", spoke.Name)
			}
		}
		ports[port] = true

		ips := []net.IP{net.ParseIP(endpoint.Hostname())}
		if ips[0] == nil {
			ips, err = net.DefaultResolver.LookupIP(ctx, "ip", endpoint.Hostname())
			if err != nil {
				return nil, nil, fmt.Errorf("resolve API endpoint for %s: %w", spoke.Name, err)
			}
		}
		for _, ip := range ips {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			cidrs[ip.String()+"/"+strconv.Itoa(bits)] = true
		}
	}

	peers := make([]networkingv1.NetworkPolicyPeer, 0, len(cidrs))
	for cidr := range cidrs {
		peers = append(peers, networkingv1.NetworkPolicyPeer{IPBlock: &networkingv1.IPBlock{CIDR: cidr}})
	}
	networkPorts := make([]networkingv1.NetworkPolicyPort, 0, len(ports))
	for port := range ports {
		networkPorts = append(networkPorts, networkingv1.NetworkPolicyPort{Protocol: ptr(corev1.ProtocolTCP), Port: intPort(port)})
	}
	return peers, networkPorts, nil
}

func ptr[T any](value T) *T { return &value }

func intPort(port int) *intstr.IntOrString {
	value := intstr.FromInt(port)
	return &value
}

func (e *Environment) waitForKueueResources(ctx context.Context) error {
	for _, cluster := range e.clusters() {
		if err := waitForActive(ctx, cluster, clusterQueueGVR, "", e.Prefix); err != nil {
			return fmt.Errorf("ClusterQueue on %s: %w", cluster.Name, err)
		}
	}
	if err := waitForActive(ctx, e.Hub, admissionCheckGVR, "", e.Prefix); err != nil {
		return fmt.Errorf("AdmissionCheck: %w", err)
	}
	for _, spoke := range e.Spokes {
		if err := waitForActive(ctx, e.Hub, multiKueueClusterGVR, "", e.workerName(spoke)); err != nil {
			return fmt.Errorf("MultiKueueCluster for %s: %w", spoke.Name, err)
		}
	}
	return nil
}

func waitForActive(ctx context.Context, cluster Cluster, gvr schema.GroupVersionResource, namespace, name string) error {
	resource := cluster.Clients.Dynamic.Resource(gvr).Namespace(namespace)
	var last string
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		object, getErr := resource.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return false, nil
		}
		active, detail := conditionTrue(object, "Active")
		last = detail
		return active, nil
	})
	if err != nil {
		return fmt.Errorf("%s did not become active (%s): %w", name, last, err)
	}
	return nil
}

func (e *Environment) createDynamic(ctx context.Context, cluster Cluster, gvr schema.GroupVersionResource, object *unstructured.Unstructured) error {
	resource := cluster.Clients.Dynamic.Resource(gvr).Namespace(object.GetNamespace())
	if _, err := resource.Get(ctx, object.GetName(), metav1.GetOptions{}); err == nil {
		return fmt.Errorf("%s %s already exists on %s", object.GetKind(), object.GetName(), cluster.Name)
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	object.SetLabels(e.ownedLabels())

	var uid types.UID
	e.addCleanup(func(cleanupCtx context.Context) error {
		current, getErr := resource.Get(cleanupCtx, object.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		if !e.owns(current.GetLabels()) || (uid != "" && current.GetUID() != uid) {
			return fmt.Errorf("refusing to delete replacement %s %s on %s", object.GetKind(), object.GetName(), cluster.Name)
		}
		if err := ignoreNotFound(resource.Delete(cleanupCtx, object.GetName(), metav1.DeleteOptions{})); err != nil {
			return err
		}
		return waitForNotFound(cleanupCtx, 5*time.Minute, func(ctx context.Context) error {
			_, err := resource.Get(ctx, object.GetName(), metav1.GetOptions{})
			return err
		})
	})
	created, err := resource.Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	uid = created.GetUID()
	return nil
}

func object(kind, name, namespace string, spec map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{"name": name}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	value := map[string]any{"apiVersion": "kueue.x-k8s.io/v1beta2", "kind": kind, "metadata": metadata}
	if spec != nil {
		value["spec"] = spec
	}
	return &unstructured.Unstructured{Object: value}
}

func (e *Environment) createClusterRole(ctx context.Context, cluster Cluster, role *rbacv1.ClusterRole) error {
	roles := cluster.Clients.KubeClient.Kube.RbacV1().ClusterRoles()
	if _, err := roles.Get(ctx, role.Name, metav1.GetOptions{}); err == nil {
		return fmt.Errorf("ClusterRole %s already exists on %s", role.Name, cluster.Name)
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	role.Labels = e.ownedLabels()
	var uid types.UID
	e.addCleanup(func(cleanupCtx context.Context) error {
		current, getErr := roles.Get(cleanupCtx, role.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		if !e.owns(current.Labels) || (uid != "" && current.UID != uid) {
			return fmt.Errorf("refusing to delete replacement ClusterRole %s on %s", role.Name, cluster.Name)
		}
		return ignoreNotFound(roles.Delete(cleanupCtx, role.Name, metav1.DeleteOptions{}))
	})
	created, err := roles.Create(ctx, role, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	uid = created.UID
	return nil
}

func (e *Environment) createClusterRoleBinding(ctx context.Context, cluster Cluster, binding *rbacv1.ClusterRoleBinding) error {
	bindings := cluster.Clients.KubeClient.Kube.RbacV1().ClusterRoleBindings()
	if _, err := bindings.Get(ctx, binding.Name, metav1.GetOptions{}); err == nil {
		return fmt.Errorf("ClusterRoleBinding %s already exists on %s", binding.Name, cluster.Name)
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	binding.Labels = e.ownedLabels()
	var uid types.UID
	e.addCleanup(func(cleanupCtx context.Context) error {
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current, getErr := bindings.Get(cleanupCtx, binding.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(getErr) {
				return nil
			}
			if getErr != nil {
				return getErr
			}
			if !e.owns(current.Labels) || (uid != "" && current.UID != uid) {
				return fmt.Errorf("refusing to delete replacement ClusterRoleBinding %s on %s", binding.Name, cluster.Name)
			}
			current.Subjects = nil
			if _, updateErr := bindings.Update(cleanupCtx, current, metav1.UpdateOptions{}); updateErr != nil {
				return updateErr
			}
			return ignoreNotFound(bindings.Delete(cleanupCtx, binding.Name, metav1.DeleteOptions{}))
		})
	})
	created, err := bindings.Create(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	uid = created.UID
	return nil
}

func (e *Environment) createRole(ctx context.Context, cluster Cluster, role *rbacv1.Role) error {
	role.Namespace = e.Namespace
	role.Labels = e.ownedLabels()
	_, err := cluster.Clients.KubeClient.Kube.RbacV1().Roles(e.Namespace).Create(ctx, role, metav1.CreateOptions{})
	return err
}

func (e *Environment) createRoleBinding(ctx context.Context, cluster Cluster, binding *rbacv1.RoleBinding) error {
	binding.Namespace = e.Namespace
	binding.Labels = e.ownedLabels()
	_, err := cluster.Clients.KubeClient.Kube.RbacV1().RoleBindings(e.Namespace).Create(ctx, binding, metav1.CreateOptions{})
	return err
}

func (e *Environment) ownedLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "release-tests-ginkgo",
		"app.kubernetes.io/instance":   e.Prefix,
	}
}

func (e *Environment) owns(labels map[string]string) bool {
	return labels["app.kubernetes.io/managed-by"] == "release-tests-ginkgo" && labels["app.kubernetes.io/instance"] == e.Prefix
}

func waitForNotFound(ctx context.Context, timeout time.Duration, get func(context.Context) error) error {
	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		err := get(ctx)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (e *Environment) workerName(spoke Cluster) string {
	return e.Prefix + "-" + spoke.Name
}
