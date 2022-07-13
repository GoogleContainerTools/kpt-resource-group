
# Name for labelling container images
IMAGE_NAME ?= resource-group

# Version for labelling container images
IMAGE_TAG ?= $(shell git log --pretty=format:'%h' -n 1)

# Registry for labelling container images
REGISTRY ?= local

# Full container image label
IMG ?= $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

# Test filter (regex)
FOCUS ?= ""
ifeq (,$(FOCUS))
GINKGO_FOCUS=
else
GINKGO_FOCUS=--ginkgo.focus "$(FOCUS)"
endif

# List of dirs containing go code owned by Nomos
CODE_DIRS := apis config e2e
GO_PKGS := $(foreach dir,$(CODE_DIRS),./$(dir)/...)

all: test manager

-include Makefile.deps
-include Makefile.manifest

.PHONY: test
# Run tests
test: generate lint manifests
	go mod tidy
	GO111MODULE=on go test ./controllers/... ./apis/... -coverprofile cover.out

.PHONY: manager
# Build manager binary
manager: generate lint
	GO111MODULE=on go build -o bin/manager main.go

.PHONY: run
# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate lint manifests
	GO111MODULE=on go run ./main.go

.PHONY: install
# Install CRDs into a cluster
install: manifests "$(KUSTOMIZE)"
	"$(KUSTOMIZE)" build config/crd | kubectl apply -f -

.PHONY: uninstall
# Uninstall CRDs from a cluster
uninstall: manifests "$(KUSTOMIZE)"
	"$(KUSTOMIZE)" build config/crd | kubectl delete --ignore-not-found -f -

.PHONY: deploy
# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests "$(KUSTOMIZE)"
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | kubectl apply -f -

.PHONY: deploy-otel-collector
# Deploy otel-collector in the configured Kubernetes cluster in ~/.kube/config
deploy-otel-collector: "$(KUSTOMIZE)"
	kubectl create namespace config-management-monitoring || true
	"$(KUSTOMIZE)" build config/otel-collector | kubectl apply -f -

.PHONY: undeploy
# UnDeploy controller in the configured Kubernetes cluster in ~/.kube/config
undeploy: manifests "$(KUSTOMIZE)"
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | kubectl delete --ignore-not-found -f -

.PHONY: undeploy-otel-collector
# UnDeploy otel-collector in the configured Kubernetes cluster in ~/.kube/config
undeploy-otel-collector:
	"$(KUSTOMIZE)" build config/otel-collector | kubectl delete --ignore-not-found -f -
	kubectl delete namespace config-management-monitoring --ignore-not-found

.PHONY: manifests
# Generate manifests e.g. CRD, RBAC etc.
manifests: "$(CONTROLLER_GEN)" "$(ADDLICENSE)"
	GO111MODULE=on "$(CONTROLLER_GEN)" $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	"$(ADDLICENSE)" ./config

.PHONY: fmt
# Run go fmt against code
fmt:
	GO111MODULE=on go fmt ./...

.PHONY: lint
# Run all linters
lint: lint-go lint-license-headers

.PHONY: lint-go
# Lint the Go code
lint-go: "$(GOLINT)"
	"$(GOLINT)" run ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: generate
# Generate code
generate: "$(CONTROLLER_GEN)"
	GO111MODULE=on "$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: docker-build
# Build the docker image
docker-build: gen-licenses
	docker build . -t ${IMG}

.PHONY: docker-push
# Push the docker image
docker-push:
	docker push ${IMG}

.PHONY: kind-push
# Push the docker image
kind-push: "$(KIND)"
	"$(KIND)" load docker-image --name $(KIND_CLUSTER_NAME) ${IMG}

.PHONY: e2e-test
# Run e2e test using ~/.kube/config
e2e-test:
	GO111MODULE=on go test -v ./e2e/... --timeout 20m $(GINKGO_FOCUS)

.PHONY: e2e-test-gke
# Run e2e tests in a persistent remote GKE cluster
e2e-test-gke: \
	generate manifests \
	docker-build docker-push \
	undeploy undeploy-otel-collector uninstall \
	install deploy-otel-collector deploy \
	e2e-test \
	undeploy undeploy-otel-collector uninstall

.PHONY: e2e-test-kind
# Run e2e tests in an ephemeral local kind cluster
e2e-test-kind: clean-kind-cluster kind-cluster \
	generate manifests \
	docker-build kind-push \
	undeploy undeploy-otel-collector uninstall \
	install deploy-otel-collector deploy \
	e2e-test \
	undeploy undeploy-otel-collector uninstall \
	clean-kind-cluster

.PHONY: gen-licenses
# Generate LICENSES.txt file.
gen-licenses:
	rm -f LICENSES.txt && \
	go mod vendor && \
	find vendor -name LICENSE* | xargs cat >> LICENSES.txt && \
	find vendor -name COPYING | xargs cat >> LICENSES.txt && \
	rm -r vendor

.PHONY: license-headers
license-headers: "$(ADDLICENSE)"
	"$(ADDLICENSE)" -v -c "Google LLC" -l apache -ignore=vendor/** -ignore=third_party/** -ignore=.idea/** . 2>&1 | sed '/ skipping: / d'

.PHONY: lint-license-headers
lint-license-headers: "$(ADDLICENSE)"
	"$(ADDLICENSE)" -check -ignore=vendor/** -ignore=third_party/** -ignore=.idea/** . 2>&1 | sed '/ skipping: / d'

.PHONY: apply-v1-crd
apply-v1-crd:
	kubectl apply -f config/crd/bases/kpt.dev_resourcegroups.yaml

.PHONY: apply-v1beta1-crd
apply-v1beta1-crd:
	kubectl apply -f config/crd/v1beta1/kpt.dev_resourcegroups_v1beta1.yaml

.PHONY: delete-crd
delete-crd:
	kubectl delete --ignore-not-found -f config/crd/bases/kpt.dev_resourcegroups.yaml

.PHONY: deploy-kcc
deploy-kcc: "$(OUTPUT_DIR)"
	gsutil cp gs://configconnector-operator/latest/release-bundle.tar.gz "$(OUTPUT_DIR)"/release-bundle.tar.gz
	cd "$(OUTPUT_DIR)" && tar zxvf release-bundle.tar.gz
	kubectl apply -f "$(OUTPUT_DIR)"/operator-system/configconnector-operator.yaml
	rm "$(OUTPUT_DIR)"/release-bundle.tar.gz
	# TODO: configure KCC service account with with workload identity
	# TODO: configure IAM to give the identity editor on the project
