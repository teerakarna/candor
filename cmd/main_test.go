/*
Copyright 2026 Albert Asawaroengchai.

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

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// This is the regression test for issue #58, found via slice 10's real-cluster verification: a
// fake client never simulates caching or RBAC at all, and every other envtest suite in this repo
// uses the suite's own admin config, which bypasses RBAC entirely - neither could ever have caught
// the real bug (a cached Secret Get blocking forever against a ServiceAccount with only `get` on
// one named Secret, no cluster-wide list/watch). This builds a client scoped to exactly that
// restricted identity, against a real control plane (envtest, not mocked), and proves
// managerClientOptions - the literal value cmd/main.go's ctrl.NewManager call uses - is what makes
// the difference between hanging forever and succeeding.
//
// No candor CRDs are needed here - this is entirely about core v1 Secret/RBAC behavior under the
// manager's client configuration, independent of anything candor-specific.

const (
	testNamespace  = "default"
	testSecretName = "webhook-secret"
	testSAName     = "restricted-secret-reader"
	testRoleName   = "read-one-secret"
)

func TestManagerClientOptions_SecretGetSucceedsUnderRealRBAC(t *testing.T) {
	cfg, cleanup := startEnvTestWithRestrictedIdentity(t)
	defer cleanup()

	restricted := restrictedConfig(cfg)
	c := newCachedClient(t, restricted, managerClientOptions)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: testSecretName}, got)
	if err != nil {
		t.Fatalf("Get() = %v, want success - managerClientOptions must stop a cached informer sync from blocking this", err)
	}
}

// TestManagerClientOptions_WithoutDisableFor_SecretGetHangs proves the regression test above is
// actually meaningful, not trivially passing for an unrelated reason: the identical restricted
// identity, with Secrets caching left enabled (the pre-fix configuration), must fail to complete
// within a short bounded window - the original bug was a permanent hang, not a slow success, so
// even a short timeout reliably reproduces it without slowing this test suite down.
func TestManagerClientOptions_WithoutDisableFor_SecretGetHangs(t *testing.T) {
	cfg, cleanup := startEnvTestWithRestrictedIdentity(t)
	defer cleanup()

	restricted := restrictedConfig(cfg)
	c := newCachedClient(t, restricted, client.Options{}) // no DisableFor - the pre-fix shape

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: testSecretName}, got)
	if err == nil {
		t.Fatal("Get() succeeded, want a context-deadline error - without DisableFor, " +
			"this must reproduce the original hang against a real RBAC-restricted identity")
	}
}

// startEnvTestWithRestrictedIdentity starts a real (not mocked) control plane, creates a Secret,
// a ServiceAccount, and a Role/RoleBinding granting that ServiceAccount `get` on exactly that one
// Secret - matching README.md's documented WebhookReceiver/GitOpsRepo secret-access pattern
// exactly, not an invented shape.
func startEnvTestWithRestrictedIdentity(t *testing.T) (cfg *rest.Config, cleanup func()) {
	t.Helper()
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	env := &envtest.Environment{}
	if dir := firstEnvTestBinaryDir(); dir != "" {
		env.BinaryAssetsDirectory = dir
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}

	admin, err := client.New(cfg, client.Options{})
	if err != nil {
		_ = env.Stop()
		t.Fatal(err)
	}

	ctx := context.Background()
	objs := []client.Object{
		&corev1.Secret{
			Name: testSecretName, Namespace: testNamespace,
			Data: map[string][]byte{"secret": []byte("s3cr3t")},
		},
		&corev1.ServiceAccount{
			Name: testSAName, Namespace: testNamespace,
		},
		&rbacv1.Role{
			Name: testRoleName, Namespace: testNamespace,
			Rules: []rbacv1.PolicyRule{{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{testSecretName},
				Verbs:         []string{"get"},
			}},
		},
		&rbacv1.RoleBinding{
			Name: testRoleName, Namespace: testNamespace,
			Subjects: []rbacv1.Subject{{
				Kind: "ServiceAccount", Name: testSAName, Namespace: testNamespace,
			}},
			RoleRef: rbacv1.RoleRef{Kind: "Role", Name: testRoleName, APIGroup: "rbac.authorization.k8s.io"},
		},
	}
	for _, obj := range objs {
		if err := admin.Create(ctx, obj); err != nil {
			_ = env.Stop()
			t.Fatal(err)
		}
	}

	return cfg, func() {
		if err := env.Stop(); err != nil {
			t.Error(err)
		}
	}
}

// restrictedConfig returns cfg impersonating the restricted ServiceAccount created above, rather
// than the envtest admin identity - impersonated requests go through the apiserver's full,
// real authorization chain for the impersonated user, not the impersonating one.
func restrictedConfig(cfg *rest.Config) *rest.Config {
	restricted := rest.CopyConfig(cfg)
	restricted.Impersonate = rest.ImpersonationConfig{
		UserName: "system:serviceaccount:" + testNamespace + ":" + testSAName,
	}
	return restricted
}

// newCachedClient builds a client the same way ctrl.NewManager does internally: a cache backing
// reads for every type except opts.Cache.DisableFor, wired through client.New. Controller-runtime
// caches are lazy - an informer for a given GVK is only created (and only then does its List+Watch
// sync, or fail to) the first time something actually reads that type through the client, which is
// exactly the Get call each test above makes.
func newCachedClient(t *testing.T, cfg *rest.Config, opts client.Options) client.Client {
	t.Helper()
	c, err := cache.New(cfg, cache.Options{Scheme: clientgoscheme.Scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = c.Start(ctx) }()

	var disableFor []client.Object
	if opts.Cache != nil {
		disableFor = opts.Cache.DisableFor
	}
	opts.Cache = &client.CacheOptions{Reader: c, DisableFor: disableFor}

	cl, err := client.New(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	return cl
}

// firstEnvTestBinaryDir mirrors internal/controller/suite_test.go's own helper - see its comment
// for why (BinaryAssetsDirectory must be explicit outside `make test`). Duplicated, not shared:
// each envtest suite in this repo already does this independently, and it's a few lines, not worth
// a shared package for.
func firstEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
