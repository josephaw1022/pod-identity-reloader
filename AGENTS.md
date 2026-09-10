# pod-identity-reloader - AI Agent Guide

This is a Kubebuilder-scaffolded Kubernetes operator. It watches Deployments,
StatefulSets, and DaemonSets and restarts them when the IAM role bound to
their ServiceAccount via an EKS Pod Identity Association changes.

The project follows the standard Kubebuilder single-group layout: API types
under `api/<version>/`, reconciliation logic under `internal/controller/`,
generated CRDs and RBAC under `config/`, and the manager entrypoint at
`cmd/main.go`. Files marked as auto-generated (CRDs, RBAC, webhook manifests,
`zz_generated.*.go`, and `PROJECT`) must never be hand-edited — regenerate
them with `make manifests` and `make generate` instead.

Always scaffold new APIs, controllers, and webhooks using the `kubebuilder`
CLI rather than creating files by hand, and never remove the
`+kubebuilder:scaffold:*` markers the CLI relies on to inject code.

After changing `*_types.go` files or their markers, regenerate manifests and
deepcopy code. After changing any Go file, run the linter's auto-fix and the
unit test suite before considering the change complete. Unit tests run
against a real API server and etcd via envtest and are written in the
Ginkgo/Gomega BDD style. End-to-end tests must be run against an isolated
Kind cluster, never a real development or production cluster.

Status should be represented with `metav1.Condition` rather than custom
string fields, and reconciliation must be idempotent and safe to run
repeatedly. Re-fetch objects before updating them to avoid conflicts, use
owner references for garbage collection, and follow the Kubernetes logging
message style guidelines (capitalized, no trailing period, active voice,
past tense for completed actions, and the affected object type named
explicitly).

The project can be distributed either as a Kustomize-generated
`dist/install.yaml` bundle or as a Helm chart under `dist/chart/`
(or `charts/` if customized). If webhooks or manifests change after the Helm
chart was generated, back up any customizations before regenerating it and
restore them afterward.

Consult the Kubebuilder book and controller-runtime documentation for
background on these conventions rather than duplicating them here.
