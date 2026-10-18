# Contributing

This project is a [Kubebuilder](https://book.kubebuilder.io/)-scaffolded
operator built on [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).
It follows the standard single-group Kubebuilder layout:

- `api/<version>/` — API types
- `internal/controller/` — reconciliation logic
- `config/` — generated CRDs, RBAC, and manager manifests (Kustomize)
- `charts/pod-identity-reloader/` — Helm chart
- `cmd/main.go` — manager entrypoint

## Requirements

- go v1.24.6+
- docker v17.03+
- kubectl v1.11.3+
- [kind](https://kind.sigs.k8s.io/) (for e2e tests)
- Access to an EKS cluster (for manual/integration verification)

## Scaffolding new code

Use the `kubebuilder` CLI to scaffold new APIs, controllers, and webhooks —
do not hand-write these files. Never remove the `+kubebuilder:scaffold:*`
markers; the CLI relies on them to inject generated code.

After changing `*_types.go` files or their markers, regenerate manifests and
deepcopy code:

```sh
make manifests generate
```

The following files are auto-generated and must never be hand-edited:
CRDs, RBAC, webhook manifests, `zz_generated.*.go`, and `PROJECT`.

## Local development

```sh
make build      # Compile the manager binary
make run        # Run the manager locally against your current kubeconfig context
make fmt vet    # Format and vet
make lint       # Run golangci-lint
make lint-fix   # Run golangci-lint with auto-fix
```

Run the linter's auto-fix and the full unit test suite before considering
any Go change complete.

### Local development against MiniStack

This project has no dependency on a real AWS account for local development.
[MiniStack](https://github.com/ministackorg/ministack) is a free,
open-source AWS emulator used as a stand-in EKS/IAM API. See
`hack/local-dev/env.sh` for the shared configuration (cluster name,
MiniStack image/ports, seeded IAM role/namespace/service account).

Two flows are available:

**Fast loop — run the manager as a local Go process:**

```sh
make local-up    # Start MiniStack in Docker; seed an IAM role, EKS cluster, and Pod Identity Association
make local-run   # go run the manager against MiniStack (--cluster-name matches the seeded cluster)
make local-down  # Stop and remove the MiniStack container
```

**Full loop — Kind cluster with MiniStack and the controller deployed in-cluster:**

```sh
make local-kind-deploy  # Kind cluster + in-cluster MiniStack + locally built/loaded controller image, manifests applied
make local-kind-down    # Delete the Kind cluster
```

`local-kind-deploy` builds the manager image, loads it into Kind, deploys
MiniStack under `config/local-dev/ministack`, deploys the operator via the
`config/local-dev/controller` kustomize overlay (which points
`AWS_ENDPOINT_URL` at the in-cluster MiniStack service), and seeds MiniStack
with a matching `--cluster-name`. The seeded cluster name always matches the
`--cluster-name` the controller is started with — see
`hack/local-dev/seed-aws.sh` and `config/local-dev/controller/manager_local_dev_patch.yaml`.

## Testing

```sh
make test       # Unit tests (envtest: real API server + etcd), Ginkgo/Gomega BDD style
make test-e2e   # E2e tests against an isolated Kind cluster
```

Never run e2e tests against a real development or production cluster.

`make test-e2e` reuses the local development tooling under `hack/local-dev/`:
it creates an isolated Kind cluster (`hack/local-dev/kind-up.sh`), and the
Ginkgo suite deploys MiniStack into it, deploys the controller pointed at
MiniStack, and seeds a sample IAM role/EKS cluster/Pod Identity Association
(`hack/local-dev/seed-aws.sh`). One test rotates the sample role
(`hack/local-dev/rotate-sample-role.sh`) and asserts the controller detects
the change and triggers a rollout. See `test/e2e/e2e_test.go` and
`test/e2e/testdata/sample-workload.yaml`.

## Conventions

- Represent status with `metav1.Condition`, not custom string fields.
- Reconciliation must be idempotent and safe to run repeatedly.
- Re-fetch objects before updating them to avoid update conflicts.
- Use owner references for garbage collection.
- Follow Kubernetes log message style: capitalized, no trailing period,
  active voice, past tense for completed actions, and name the affected
  object type explicitly.

## Helm chart

If webhooks or manifests change, the Helm chart under `charts/pod-identity-reloader/`
may need regeneration. Back up any manual chart customizations first, then
restore them after regenerating.

## Releasing

- Pushing to `main` or a version tag triggers the `Publish image` workflow,
  which builds and pushes the manager image to GHCR.
- Pushing a version tag triggers the `Publish Helm chart` workflow, which
  packages and publishes the chart to GHCR as an OCI artifact.

## Submitting changes

1. Open a PR with a clear description of the change and rationale.
2. Ensure `make lint` and `make test` pass locally.
3. Keep unrelated refactors out of feature/fix PRs.
