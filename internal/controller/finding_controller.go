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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
)

// FindingReconciler reconciles a Finding object
type FindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=candor.dev,resources=findings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=candor.dev,resources=findings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=candor.dev,resources=findings/finalizers,verbs=update

// Reconcile is intentionally a no-op for now. A Finding is created fully-populated by
// internal/signal.Ingest, which is the only writer; there's nothing further to derive from a
// Finding's own state yet. That changes in later slices - fingerprint verification (re-checking
// whether a Finding's underlying condition cleared) and LLM enrichment both belong here once they
// exist (see docs/design.md's delivery slices). The reconciler is still registered and watching
// now so that transition doesn't require wiring anything new.
func (r *FindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&candorv1alpha1.Finding{}).
		Named("finding").
		Complete(r)
}
