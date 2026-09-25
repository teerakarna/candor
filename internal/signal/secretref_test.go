package signal

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testSecretName    = "creds"
	testSecretNS      = "default"
	testSecretDataKey = "token"
)

func newSecretClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestResolveSecretKey_Success(t *testing.T) {
	c := newSecretClient(t, &corev1.Secret{
		Name: testSecretName, Namespace: testSecretNS,
		Data: map[string][]byte{testSecretDataKey: []byte("s3cr3t")},
	})

	got, err := ResolveSecretKey(context.Background(), c, client.ObjectKey{Namespace: testSecretNS, Name: testSecretName}, testSecretDataKey)
	if err != nil {
		t.Fatalf("ResolveSecretKey() error = %v, want nil", err)
	}
	if string(got) != "s3cr3t" {
		t.Fatalf("ResolveSecretKey() = %q, want %q", got, "s3cr3t")
	}
}

func TestResolveSecretKey_SecretNotFound(t *testing.T) {
	c := newSecretClient(t)

	_, err := ResolveSecretKey(context.Background(), c, client.ObjectKey{Namespace: testSecretNS, Name: "missing"}, testSecretDataKey)
	if err == nil {
		t.Fatal("ResolveSecretKey() error = nil, want an error for a missing Secret")
	}
}

func TestResolveSecretKey_KeyMissing(t *testing.T) {
	c := newSecretClient(t, &corev1.Secret{
		Name: testSecretName, Namespace: testSecretNS,
		Data: map[string][]byte{"other": []byte("value")},
	})

	_, err := ResolveSecretKey(context.Background(), c, client.ObjectKey{Namespace: testSecretNS, Name: testSecretName}, testSecretDataKey)
	if err == nil {
		t.Fatal("ResolveSecretKey() error = nil, want an error for a missing data key")
	}
}

func TestResolveSecretKey_KeyEmpty(t *testing.T) {
	c := newSecretClient(t, &corev1.Secret{
		Name: testSecretName, Namespace: testSecretNS,
		Data: map[string][]byte{testSecretDataKey: {}},
	})

	_, err := ResolveSecretKey(context.Background(), c, client.ObjectKey{Namespace: testSecretNS, Name: testSecretName}, testSecretDataKey)
	if err == nil {
		t.Fatal("ResolveSecretKey() error = nil, want an error for an empty data key")
	}
}
