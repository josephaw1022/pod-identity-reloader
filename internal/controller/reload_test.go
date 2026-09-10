/*
Copyright 2026 Joseph Whiteaker.

Licensed under the MIT License. See LICENSE for details.
*/

package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/josephaw1022/pod-identity-reloader/pkg/podidentity"
)

type fakeLookup struct {
	roleARN string
	err     error
}

func (f *fakeLookup) RoleARN(_ context.Context, _, _, _ string) (string, error) {
	return f.roleARN, f.err
}

func newTestDeployment(annotations map[string]string, saName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "app",
			Namespace:   "default",
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{ServiceAccountName: saName},
			},
		},
	}
}

func newFakeClient(objs ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		builder = builder.WithRuntimeObjects(o)
	}
	return builder.Build()
}

func TestReloadIfRoleChangedSkipsWithoutOptIn(t *testing.T) {
	dep := newTestDeployment(nil, "my-sa")
	c := newFakeClient(dep)

	res, err := reloadIfRoleChanged(context.Background(), c, &fakeLookup{roleARN: "arn:aws:iam::111122223333:role/r"}, "cluster", 0, &deploymentWorkload{dep})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no requeue when opted out, got %v", res)
	}
	if dep.Spec.Template.Annotations[RoleARNHashAnnotation] != "" {
		t.Fatal("expected no hash annotation to be written when opted out")
	}
}

func TestReloadIfRoleChangedSkipsWithoutAssociation(t *testing.T) {
	dep := newTestDeployment(map[string]string{AutoReloadAnnotation: "true"}, "my-sa")
	c := newFakeClient(dep)

	res, err := reloadIfRoleChanged(context.Background(), c, &fakeLookup{err: podidentity.ErrNoAssociation}, "cluster", 0, &deploymentWorkload{dep})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatal("expected a requeue to keep polling")
	}
}

func TestReloadIfRoleChangedPatchesOnFirstObservation(t *testing.T) {
	dep := newTestDeployment(map[string]string{AutoReloadAnnotation: "true"}, "my-sa")
	c := newFakeClient(dep)

	_, err := reloadIfRoleChanged(context.Background(), c, &fakeLookup{roleARN: "arn:aws:iam::111122223333:role/r"}, "cluster", 0, &deploymentWorkload{dep})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.Spec.Template.Annotations[RoleARNHashAnnotation] == "" {
		t.Fatal("expected role arn hash annotation to be set")
	}
	if dep.Spec.Template.Annotations[RestartedAtAnnotation] == "" {
		t.Fatal("expected restarted-at annotation to be set")
	}
}

func TestReloadIfRoleChangedNoopWhenRoleUnchanged(t *testing.T) {
	dep := newTestDeployment(map[string]string{AutoReloadAnnotation: "true"}, "my-sa")
	c := newFakeClient(dep)
	lookup := &fakeLookup{roleARN: "arn:aws:iam::111122223333:role/r"}

	if _, err := reloadIfRoleChanged(context.Background(), c, lookup, "cluster", 0, &deploymentWorkload{dep}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	restartedAt := dep.Spec.Template.Annotations[RestartedAtAnnotation]

	if _, err := reloadIfRoleChanged(context.Background(), c, lookup, "cluster", 0, &deploymentWorkload{dep}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.Spec.Template.Annotations[RestartedAtAnnotation] != restartedAt {
		t.Fatal("expected restarted-at annotation to stay unchanged when role did not change")
	}
}

func TestReloadIfRoleChangedPatchesWhenRoleChanges(t *testing.T) {
	dep := newTestDeployment(map[string]string{AutoReloadAnnotation: "true"}, "my-sa")
	c := newFakeClient(dep)

	if _, err := reloadIfRoleChanged(context.Background(), c, &fakeLookup{roleARN: "arn:aws:iam::111122223333:role/r1"}, "cluster", 0, &deploymentWorkload{dep}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	firstHash := dep.Spec.Template.Annotations[RoleARNHashAnnotation]

	if _, err := reloadIfRoleChanged(context.Background(), c, &fakeLookup{roleARN: "arn:aws:iam::111122223333:role/r2"}, "cluster", 0, &deploymentWorkload{dep}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.Spec.Template.Annotations[RoleARNHashAnnotation] == firstHash {
		t.Fatal("expected hash annotation to change when role changes")
	}
}
