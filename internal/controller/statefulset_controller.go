/*
Copyright 2026 Joseph Whiteaker.

Licensed under the MIT License. See LICENSE for details.
*/

package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/josephaw1022/pod-identity-reloader/pkg/podidentity"
)

// statefulsetWorkload adapts a StatefulSet to the shared podTemplate interface used
// by reloadIfRoleChanged.
type statefulsetWorkload struct{ *appsv1.StatefulSet }

func (w *statefulsetWorkload) template() *corev1.PodTemplateSpec { return &w.Spec.Template }

// StatefulSetReconciler restarts StatefulSets whose ServiceAccount's EKS Pod Identity
// Association IAM role has changed since it was last observed.
type StatefulSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Lookup resolves the current IAM role ARN for a namespace/service
	// account pair.
	Lookup podidentity.Lookup
	// ClusterName is the EKS cluster name passed to the Pod Identity
	// Association API.
	ClusterName string
	// PollInterval controls how frequently tracked StatefulSets are
	// re-reconciled. Defaults to 30s when zero.
	PollInterval time.Duration
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=update

// Reconcile restarts the StatefulSet's pods when its ServiceAccount's IAM role,
// as reported by the EKS Pod Identity Association API, has changed.
func (r *StatefulSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj appsv1.StatefulSet
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	return reloadIfRoleChanged(ctx, r.Client, r.Lookup, r.ClusterName, r.PollInterval, &statefulsetWorkload{&obj})
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}, builder.WithPredicates(autoReloadEnabled)).
		Named("statefulset").
		Complete(r)
}
