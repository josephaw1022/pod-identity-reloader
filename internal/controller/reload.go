/*
Copyright 2026 Joseph Whiteaker.

Licensed under the MIT License. See LICENSE for details.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/josephaw1022/pod-identity-reloader/pkg/podidentity"
)

const (
	// AutoReloadAnnotation opts a workload into pod-identity-reloader. Set it
	// to "true" on a Deployment/StatefulSet/DaemonSet to have its pods
	// restarted whenever the IAM role bound to its ServiceAccount changes.
	AutoReloadAnnotation = "pod-identity-reloader.dev/auto"

	// RoleARNHashAnnotation is written on the pod template and stores the
	// hash of the last observed IAM role ARN.
	RoleARNHashAnnotation = "pod-identity-reloader.dev/role-arn-hash"

	// RestartedAtAnnotation is written on the pod template to force a
	// rollout, mirroring `kubectl rollout restart`.
	RestartedAtAnnotation = "pod-identity-reloader.dev/restarted-at"

	// defaultPollInterval controls how often a tracked workload is
	// re-checked against the EKS Pod Identity Association API, since role
	// reassignment happens outside the Kubernetes API and produces no watch
	// event.
	defaultPollInterval = 30 * time.Second

	// annotationValueTrue is the opt-in value expected on
	// AutoReloadAnnotation.
	annotationValueTrue = "true"
)

// autoReloadEnabled matches objects opted into reloading via
// AutoReloadAnnotation, keeping the controllers from reconciling every
// workload in the cluster.
var autoReloadEnabled = predicate.NewPredicateFuncs(func(obj client.Object) bool {
	return obj.GetAnnotations()[AutoReloadAnnotation] == annotationValueTrue
})

// podTemplate is implemented by the workload kinds this controller supports,
// letting a single reload routine drive Deployments, StatefulSets, and
// DaemonSets.
type podTemplate interface {
	client.Object
	// template returns a pointer to the embedded pod template so its
	// annotations can be read and patched in place.
	template() *corev1.PodTemplateSpec
	// object returns the concrete workload object (e.g. *appsv1.Deployment)
	// so it can be passed to client.Update. Passing the wrapper itself would
	// break the client's scheme lookup, since its type is never registered.
	object() client.Object
}

func pollInterval(configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return defaultPollInterval
}

// serviceAccountName returns the pod template's service account, defaulting
// to "default" the same way Kubernetes does when it is unset.
func serviceAccountName(tpl *corev1.PodTemplateSpec) string {
	name := tpl.Spec.ServiceAccountName
	if name == "" {
		name = tpl.Spec.DeprecatedServiceAccount
	}
	if name == "" {
		name = "default"
	}
	return name
}

// reloadIfRoleChanged looks up the IAM role currently bound to wl's
// ServiceAccount and, if it differs from the last observed role, patches the
// pod template to force a rollout restart. It is shared by every workload
// controller so the reload behavior stays identical across kinds.
func reloadIfRoleChanged(ctx context.Context, c client.Client, lookup podidentity.Lookup, clusterName string, interval time.Duration, wl podTemplate) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if wl.GetAnnotations()[AutoReloadAnnotation] != annotationValueTrue {
		return ctrl.Result{}, nil
	}

	tpl := wl.template()
	sa := serviceAccountName(tpl)

	roleARN, err := lookup.RoleARN(ctx, clusterName, wl.GetNamespace(), sa)
	if err != nil {
		if errors.Is(err, podidentity.ErrNoAssociation) {
			log.V(1).Info("No pod identity association for service account, nothing to track",
				"serviceAccount", sa)
			return ctrl.Result{RequeueAfter: pollInterval(interval)}, nil
		}
		return ctrl.Result{}, err
	}

	hash := sha256.Sum256([]byte(roleARN))
	newHash := hex.EncodeToString(hash[:])

	if tpl.Annotations != nil && tpl.Annotations[RoleARNHashAnnotation] == newHash {
		return ctrl.Result{RequeueAfter: pollInterval(interval)}, nil
	}

	log.Info("IAM role changed for service account, restarting workload",
		"serviceAccount", sa, "name", wl.GetName(), "namespace", wl.GetNamespace())

	if tpl.Annotations == nil {
		tpl.Annotations = map[string]string{}
	}
	tpl.Annotations[RoleARNHashAnnotation] = newHash
	tpl.Annotations[RestartedAtAnnotation] = time.Now().UTC().Format(time.RFC3339)

	if err := c.Update(ctx, wl.object()); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: pollInterval(interval)}, nil
}
