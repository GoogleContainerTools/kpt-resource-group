# Resource Group

ResourceGroup provide a custom resource type and an accompanying controller
for grouping Kubernetes resources and computing the aggregate resource
status.

## Contributing

If you are interested in contributing please start with
[contribution guidelines](docs/CONTRIBUTING.md).

## Local Development

This section contains instructions to properly set up an environment for
building/developing locally.

### Install kubebuilder

[kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) is a dependency
for building the project from source. The following steps detail how this
dependency may be installed:

```shell
export KUBEBUILDER_VERSION=2.3.1
export GOOS=$(go env GOOS)
export GOARCH=$(go env GOARCH)
wget "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${GOARCH}.tar.gz"
tar zxvf "kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${GOARCH}.tar.gz"
mkdir -p /usr/local/kubebuilder/bin
mv "kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${GOARCH}"/bin/* /usr/local/kubebuilder/bin
```
