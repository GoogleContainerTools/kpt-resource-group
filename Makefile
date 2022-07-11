
IMAGE_NAME=resource-group
IMAGE_TAG=$(shell git log --pretty=format:'%h' -n 1)
REGISTRY=gcr.io/$(shell gcloud config get-value project)

# Image URL to use all building/pushing image targets
IMG ?= $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,crdVersions=v1"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: test manager

-include Makefile.manifest

# Run tests
test: generate fmt vet manifests
	go mod tidy
	GO111MODULE=on go test ./controllers/... ./apis/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	GO111MODULE=on go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	GO111MODULE=on go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete --ignore-not-found -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Deploy otel-collector in the configured Kubernetes cluster in ~/.kube/config
deploy-otel-collector:
	kubectl create namespace config-management-monitoring || true
	kustomize build config/otel-collector | kubectl apply -f -

# UnDeploy controller in the configured Kubernetes cluster in ~/.kube/config
undeploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl delete --ignore-not-found -f -

# UnDeploy otel-collector in the configured Kubernetes cluster in ~/.kube/config
undeploy-otel-collector:
	kustomize build config/otel-collector | kubectl delete --ignore-not-found -f -
	kubectl delete namespace config-management-monitoring --ignore-not-found

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	GO111MODULE=on $(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	(which addlicense || go install github.com/google/addlicense@latest)
	$(GOBIN)/addlicense ./config

# Run go fmt against code
fmt:
	GO111MODULE=on go fmt ./...

# Run go vet against code
vet:
	GO111MODULE=on go vet ./...

# Generate code
generate: controller-gen
	GO111MODULE=on $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: gen-licenses
	docker build . -t ${IMG};\
	rm -r golang-lru

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# Run e2e test
e2e-test: generate manifests docker-build docker-push
	GO111MODULE=on go test -v ./e2e/... --timeout 20m

# Generate LICENSES.txt file.
# We copy the golang-lru source into the root to make sure the source is included
# in the image. It is required to satisfy the licensing terms.
.PHONY: gen-licenses
gen-licenses:
	rm LICENSES.txt; go mod vendor;\
	find vendor -name LICENSE* | xargs cat >> LICENSES.txt;\
	find vendor -name COPYING | xargs cat >> LICENSES.txt;\
	cp -r vendor/github.com/hashicorp/golang-lru/ golang-lru/;\
	rm -r vendor

.PHONY: license
license:
	(which addlicense || go install github.com/google/addlicense@latest)
	$(GOBIN)/addlicense -ignore vendor/** -ignore .idea/** .

.PHONY: check-license
check-license:
	(which addlicense || go install github.com/google/addlicense@latest)
	$(GOBIN)/addlicense -check -ignore vendor/** -ignore .idea/** .

apply-v1-crd:
	kubectl apply -f config/crd/bases/kpt.dev_resourcegroups.yaml

apply-v1beta1-crd:
	kubectl apply -f config/crd/v1beta1/kpt.dev_resourcegroups_v1beta1.yaml

delete-crd:
	kubectl delete --ignore-not-found -f config/crd/bases/kpt.dev_resourcegroups.yaml
