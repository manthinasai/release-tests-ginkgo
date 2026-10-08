package approvalgate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

const testAdminKubeconfig = `apiVersion: v1
kind: Config
current-context: primary
clusters:
- name: primary
  cluster:
    server: https://primary.example.com:6443
    certificate-authority-data: Y2EtcHJpbWFyeQ==
- name: secondary
  cluster:
    server: https://secondary.example.com:6443
    insecure-skip-tls-verify: true
users:
- name: admin-primary
  user:
    token: primary-token
- name: admin-secondary
  user:
    token: secondary-token
contexts:
- name: primary
  context:
    cluster: primary
    user: admin-primary
- name: secondary
  context:
    cluster: secondary
    user: admin-secondary
`

const testGroupsJSON = `{"apiVersion":"user.openshift.io/v1","kind":"GroupList","items":[
 {"metadata":{"name":"mag-ns1-reviewers"},"users":["user2","user1"]},
 {"metadata":{"name":"mag-ns1-approvers"},"users":["user1"]},
 {"metadata":{"name":"mag-ns2-approvers"},"users":["user3"]}
]}`

// addFakeOC puts an oc stand-in on PATH that answers the two queries the
// impersonation helpers make: the user groups and the control plane topology.
func addFakeOC(t *testing.T, groupsJSON, topology string) {
	t.Helper()
	binDir := t.TempDir()
	groupsFile := filepath.Join(binDir, "groups.json")
	if err := os.WriteFile(groupsFile, []byte(groupsJSON), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"  *'get groups'*) cat '" + groupsFile + "' ;;\n" +
		"  *'get infrastructure cluster'*) printf '%s' '" + topology + "' ;;\n" +
		"  *) echo \"unexpected oc invocation: $*\" >&2; exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(binDir, "oc"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeAdminKubeconfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "admin.kubeconfig")
	if err := os.WriteFile(path, []byte(testAdminKubeconfig), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func resetImpersonationChoice() {
	impersonationOnce = sync.Once{}
	impersonation = false
	impersonationErr = nil
}

func TestImpersonationKubeconfig(t *testing.T) {
	original := *config.Flags
	defer func() { *config.Flags = original }()
	addFakeOC(t, testGroupsJSON, "External")
	config.Flags.Kubeconfig = writeAdminKubeconfig(t)

	tests := []struct {
		name        string
		context     string
		cluster     string
		user        string
		wantServer  string
		wantToken   string
		wantGroups  []string
		wantContext string
	}{
		{
			name:        "current context with groups",
			user:        "user1",
			wantServer:  "https://primary.example.com:6443",
			wantToken:   "primary-token",
			wantGroups:  []string{"mag-ns1-approvers", "mag-ns1-reviewers"},
			wantContext: "primary",
		},
		{
			name:        "context override and user without groups",
			context:     "secondary",
			user:        "user4",
			wantServer:  "https://secondary.example.com:6443",
			wantToken:   "secondary-token",
			wantGroups:  nil,
			wantContext: "secondary",
		},
		{
			name:        "cluster override keeps the context credentials",
			cluster:     "secondary",
			user:        "user3",
			wantServer:  "https://secondary.example.com:6443",
			wantToken:   "primary-token",
			wantGroups:  []string{"mag-ns2-approvers"},
			wantContext: "primary",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			gomega.RegisterTestingT(t)
			config.Flags.Context = tc.context
			config.Flags.Cluster = tc.cluster

			kcPath := impersonationKubeconfig(tc.user)
			defer func() { _ = os.Remove(kcPath) }()

			cfg, err := clientcmd.LoadFromFile(kcPath)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(cfg.CurrentContext).To(gomega.Equal(tc.wantContext))
			g.Expect(cfg.Contexts).To(gomega.HaveLen(1))
			g.Expect(cfg.Clusters).To(gomega.HaveLen(1))
			g.Expect(cfg.AuthInfos).To(gomega.HaveLen(1))

			ctx := cfg.Contexts[cfg.CurrentContext]
			g.Expect(cfg.Clusters[ctx.Cluster].Server).To(gomega.Equal(tc.wantServer))
			auth := cfg.AuthInfos[ctx.AuthInfo]
			g.Expect(auth.Token).To(gomega.Equal(tc.wantToken))
			g.Expect(auth.Impersonate).To(gomega.Equal(tc.user))
			if len(tc.wantGroups) == 0 {
				g.Expect(auth.ImpersonateGroups).To(gomega.BeEmpty())
			} else {
				g.Expect(auth.ImpersonateGroups).To(gomega.Equal(tc.wantGroups))
			}
		})
	}
}

func TestUserEnvImpersonationRemovesKubeconfig(t *testing.T) {
	g := gomega.NewWithT(t)
	gomega.RegisterTestingT(t)
	original := *config.Flags
	defer func() { *config.Flags = original }()
	addFakeOC(t, testGroupsJSON, "External")
	config.Flags.Kubeconfig = writeAdminKubeconfig(t)
	config.Flags.Context = ""
	config.Flags.Cluster = ""
	t.Setenv(UseImpersonationEnv, "true")
	resetImpersonationChoice()
	defer resetImpersonationChoice()

	env, cleanup := userEnv("user1")
	g.Expect(env).To(gomega.HaveLen(1))
	g.Expect(env[0]).To(gomega.HavePrefix("KUBECONFIG="))
	kcPath := env[0][len("KUBECONFIG="):]
	g.Expect(kcPath).To(gomega.BeAnExistingFile())

	cleanup()
	g.Expect(kcPath).NotTo(gomega.BeAnExistingFile())
}

func TestUseImpersonation(t *testing.T) {
	defer resetImpersonationChoice()

	tests := []struct {
		name     string
		envValue string
		topology string
		want     bool
	}{
		{name: "hosted control plane detected", topology: "External", want: true},
		{name: "standalone control plane", topology: "HighlyAvailable", want: false},
		{name: "env forces impersonation", envValue: "true", topology: "HighlyAvailable", want: true},
		{name: "env disables impersonation", envValue: "0", topology: "External", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gomega.RegisterTestingT(t)
			addFakeOC(t, testGroupsJSON, tc.topology)
			t.Setenv(UseImpersonationEnv, tc.envValue)
			resetImpersonationChoice()

			if got := useImpersonation(); got != tc.want {
				t.Fatalf("useImpersonation() = %t, want %t", got, tc.want)
			}
		})
	}
}
