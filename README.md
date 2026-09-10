# pod-identity-reloader

A [stakater/Reloader](https://github.com/stakater/Reloader)-style Kubernetes
operator, but for **AWS IAM roles instead of ConfigMaps/Secrets**.

It watches Deployments, StatefulSets, and DaemonSets, and restarts them
whenever the IAM role bound to their ServiceAccount via an
[EKS Pod Identity Association](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html)
changes — for example after `aws eks update-pod-identity-association` swaps
the role, or after a Pod Identity Association is deleted and recreated
pointing at a new role. Pods otherwise keep the stale IAM credentials for
their entire lifetime, so a rollout is the only way to pick up the change.

## How it works

1. Opt a workload in by adding the annotation
   `pod-identity-reloader.josephaw1022.dev/auto: "true"`.
2. The controller reads the workload's `spec.template.spec.serviceAccountName`
   and calls the EKS API (`ListPodIdentityAssociations` /
   `DescribePodIdentityAssociation`) to resolve the IAM role currently bound
   to that namespace/ServiceAccount pair.
3. The role ARN is hashed and stored as a pod template annotation
   (`pod-identity-reloader.josephaw1022.dev/role-arn-hash`). If the hash
   changes, the controller patches
   `pod-identity-reloader.josephaw1022.dev/restarted-at` on the pod template,
   which triggers a rollout — the same mechanism `kubectl rollout restart`
   uses.
4. The workload is re-checked on a poll interval (default `30s`, configurable
   via `--poll-interval`) since role reassignment happens entirely on the AWS
   side and produces no Kubernetes watch event.

Workloads with no Pod Identity Association for their ServiceAccount are left
alone — there's nothing to track.

See [`examples/sample-deployment.yaml`](examples/sample-deployment.yaml) for
a minimal example.

## Prerequisites

- An EKS cluster with the [Pod Identity Agent](https://docs.aws.amazon.com/eks/latest/userguide/pod-identity-agent-setup.html)
  add-on installed.
- The controller itself needs an IAM role (granted via its own Pod Identity
  Association, or IRSA) with `eks:ListPodIdentityAssociations` and
  `eks:DescribePodIdentityAssociation` permissions.

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to an EKS cluster

### Build and deploy

```sh
make docker-build docker-push IMG=<some-registry>/pod-identity-reloader:tag
make deploy IMG=<some-registry>/pod-identity-reloader:tag
```

The manager requires `--cluster-name=<your-eks-cluster-name>`; set it via
`config/manager/manager.yaml` or an equivalent Helm/Kustomize override.

### Published image

The `Publish image` workflow builds the manager image and pushes it to GHCR
when `main` or a version tag is pushed:

```sh
docker pull ghcr.io/josephaw1022/pod-identity-reloader:main
```

### Helm chart

The `Publish Helm chart` workflow packages the chart and publishes it to GHCR
as an OCI artifact for version tags. Install it with:

```sh
helm install pod-identity-reloader \
  oci://ghcr.io/josephaw1022/pod-identity-reloader \
  --version <version> \
  --set clusterName=<your-eks-cluster-name>
```

### Try it out

```sh
kubectl apply -f examples/sample-deployment.yaml
```

### Uninstall

```sh
make undeploy
```

## Development

```sh
make build      # Compile the manager binary
make test       # Run unit tests (envtest)
make test-e2e   # Run e2e tests against a Kind cluster
make run        # Run the manager locally against your current kubeconfig context
```

Built with [Kubebuilder](https://book.kubebuilder.io/) and
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).

## License

MIT — see [LICENSE](LICENSE).
