package signal

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveSecretKey fetches the named Secret and returns the value at key, or an error if the
// Secret can't be read or the key is missing/empty. Shared by every provider that authenticates
// or authorizes against one Secret data key by convention (GitOpsRepo.SecretRef's "token",
// WebhookReceiver.SecretRef's "secret") - both used to duplicate this same resolve-and-validate
// logic with the identical error string, which could drift silently between the two.
func ResolveSecretKey(ctx context.Context, c client.Client, ref client.ObjectKey, key string) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, ref, secret); err != nil {
		return nil, fmt.Errorf("getting secret %s: %w", ref, err)
	}
	value := secret.Data[key]
	if len(value) == 0 {
		return nil, fmt.Errorf("secret %s has no data key %q", ref, key)
	}
	return value, nil
}
