# pod-identity-reloader

A [stakater/Reloader](https://github.com/stakater/Reloader)-style Kubernetes
operator, but for **AWS IAM roles instead of ConfigMaps/Secrets**.

When an [EKS Pod Identity Association](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html)
is repointed at a new IAM role, running pods keep the old credentials for
their entire lifetime. This operator detects the change and triggers a
rollout so pods pick up the new role automatically.

## Prerequisites

- An **EKS cluster** with the [Pod Identity Agent](https://docs.aws.amazon.com/eks/latest/userguide/pod-identity-agent-setup.html)
  add-on installed.
- Workloads must use **EKS Pod Identity Associations** (not IRSA) to bind an
  IAM role to a ServiceAccount.
- The controller needs its own IAM role (via a Pod Identity Association or
  IRSA) granting `eks:ListPodIdentityAssociations` and
  `eks:DescribePodIdentityAssociation`.
- `--cluster-name=<your-eks-cluster-name>` must be set on the manager so it
  knows which cluster to query.

## Install

### Helm (recommended)

```sh
helm install pod-identity-reloader \
  oci://ghcr.io/josephaw1022/charts/pod-identity-reloader \
  --version <version> \
  --set clusterName=<your-eks-cluster-name>
```

### kubectl apply

```sh
make build-installer IMG=ghcr.io/josephaw1022/pod-identity-reloader:sha-<short-sha>
kubectl apply -f dist/install.yaml
```

Set `--cluster-name=<your-eks-cluster-name>` in `config/manager/manager.yaml`
if not using the Helm chart.

## Usage

Add this annotation to a Deployment, StatefulSet, or DaemonSet to opt it in:

```yaml
metadata:
  annotations:
    pod-identity-reloader.dev/auto: "true"
```

The controller resolves the IAM role currently bound to the workload's
`spec.template.spec.serviceAccountName` via the EKS API, and triggers a
rollout (same mechanism as `kubectl rollout restart`) whenever that role
changes. Workloads are re-checked on a poll interval (default `30s`,
configurable via `--poll-interval`), since a role change on the AWS side
produces no Kubernetes watch event.

Workloads whose ServiceAccount has no Pod Identity Association are ignored.

See [`examples/sample-deployment.yaml`](examples/sample-deployment.yaml) for
a full example.

## Uninstall

```sh
helm uninstall pod-identity-reloader
# or, if installed via kubectl apply
kubectl delete -f dist/install.yaml
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local development, testing, and
release workflows.

## License

MIT — see [LICENSE](LICENSE).
