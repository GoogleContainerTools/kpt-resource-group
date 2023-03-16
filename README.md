# Resource Group

ResourceGroup provide a custom resource type and an accompanying controller
for grouping Kubernetes resources and computing the aggregate resource
status.

## Contributing

If you are interested in contributing please start with
[contribution guidelines](docs/contributing.md).

## Local Development

This section contains instructions to properly set up an environment for
building/developing locally.

### kubebuilder

[kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) is a dependency
for building the project from source. The following steps detail how this
dependency may be installed:

```shell
export KUBEBUILDER_VERSION=2.3.2
export GOOS=$(go env GOOS)
export GOARCH=$(go env GOARCH)
wget "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${GOARCH}.tar.gz"
tar zxvf "kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${GOARCH}.tar.gz"
mkdir -p /usr/local/kubebuilder/bin
mv "kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${GOARCH}"/bin/* /usr/local/kubebuilder/bin
```

### Tests

Build, test, and lint:
```
make
```

Run unit tests:
```
make test
```

Run end-to-end tests locally in kind:
```
# Requirements:
# - Docker

make e2e-test-kind
```

Run end-to-end tests remotely in GKE with Config Connector:
```
# Requirements:
# - Docker
# - gcloud configured with a GCP project set as default.
# - GKE cluster deployed in the GCP project
# - Sufficient GKE node cpu & memory to deploy the controller
# - gcloud authenticated with a user or service account
# - IAM policy that gives the current gcloud user cluster admin on the GKE cluster
# - Config Connector deployed on the GKE cluster (addon or manual)
# - Config Connector configured with a GCP service account
# - IAM policy that gives the GCP service account edit on the GCP project
# Note: This is designed for running from Prow.

./hack/e2e.sh
```