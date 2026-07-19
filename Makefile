# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/k8s-operators-devops/k8s-maintenance-operator:latest
# YEAR defines the year value used for substituting the YEAR placeholder in the boilerplate header.
ifeq ($(OS),Windows_NT)
YEAR ?= $(shell powershell -NoProfile -Command "(Get-Date).Year")
else
YEAR ?= $(shell date +%Y)
endif

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Use a shell that is available on the current platform so the recipes run without extra setup.
ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
.SHELLFLAGS := /c
else
SHELL := /usr/bin/env bash -o pipefail
.SHELLFLAGS := -ec
endif

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./api/v1alpha1;./internal/controller" output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=$(YEAR) paths="./api/v1alpha1"

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
ifeq ($(OS),Windows_NT)
	for /f "delims=" %%i in ('$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path') do set "KUBEBUILDER_ASSETS=%%i" && go test ./... -coverprofile cover.out
else
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out
endif

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# kubectl kuberc is disabled by default for test isolation; enable with:
# - KUBECTL_KUBERC=true
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= k8s-maintenance-operator-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: verify
verify: fmt vet test manifests build ## Run local validation and whitespace checks.
	git diff --check

.PHONY: bundle
bundle: manifests kustomize ## Generate the end-user installation manifest.
	$(call mkdir-if-needed,deploy)
	$(KUSTOMIZE) build config/default > deploy/install.yaml

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name k8s-maintenance-operator-builder
	$(CONTAINER_TOOL) buildx use k8s-maintenance-operator-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm k8s-maintenance-operator-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(CURDIR)/bin
ifeq ($(OS),Windows_NT)
define mkdir-if-needed
@if not exist "$(1)" mkdir "$(1)"
endef
else
define mkdir-if-needed
@mkdir -p "$(1)"
endef
endif
$(LOCALBIN):
	$(call mkdir-if-needed,$(LOCALBIN))

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
ifeq ($(OS),Windows_NT)
KUSTOMIZE ?= $(if $(wildcard $(LOCALBIN)/kustomize.exe),$(LOCALBIN)/kustomize.exe,go run sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION))
CONTROLLER_GEN ?= $(if $(wildcard $(LOCALBIN)/controller-gen.exe),$(LOCALBIN)/controller-gen.exe,go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION))
ENVTEST ?= $(if $(wildcard $(LOCALBIN)/setup-envtest.exe),$(LOCALBIN)/setup-envtest.exe,go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION))
GOLANGCI_LINT ?= $(if $(wildcard $(LOCALBIN)/golangci-lint.exe),$(LOCALBIN)/golangci-lint.exe,go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION))
else
KUSTOMIZE ?= $(if $(wildcard $(LOCALBIN)/kustomize),$(LOCALBIN)/kustomize,go run sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION))
CONTROLLER_GEN ?= $(if $(wildcard $(LOCALBIN)/controller-gen),$(LOCALBIN)/controller-gen,go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION))
ENVTEST ?= $(if $(wildcard $(LOCALBIN)/setup-envtest),$(LOCALBIN)/setup-envtest,go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION))
GOLANGCI_LINT ?= $(if $(wildcard $(LOCALBIN)/golangci-lint),$(LOCALBIN)/golangci-lint,go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION))
endif

## Tool Versions
KUSTOMIZE_VERSION ?= v5.8.1
CONTROLLER_TOOLS_VERSION ?= v0.21.0

#ENVTEST_VERSION is the controller-runtime version to use for setup-envtest, derived from go.mod
ifeq ($(OS),Windows_NT)
ENVTEST_VERSION ?= $(shell go list -m -f "{{.Version}}" sigs.k8s.io/controller-runtime 2>NUL)
ENVTEST_K8S_VERSION ?= $(shell powershell -NoProfile -Command "$$v = go list -m -f '{{.Version}}' k8s.io/api; if ($$v -match '^v?0\.(\d+)') { '1.' + $$Matches[1] } else { $$v }")
else
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v")

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')
endif

GOLANGCI_LINT_VERSION ?= v2.12.2
.PHONY: kustomize
kustomize: ## Use kustomize through go run when no local binary is available.
	@echo "Using kustomize via $(KUSTOMIZE)"

.PHONY: controller-gen
controller-gen: ## Use controller-gen through go run when no local binary is available.
	@echo "Using controller-gen via $(CONTROLLER_GEN)"

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: ## Use setup-envtest through go run when no local binary is available.
	@echo "Using setup-envtest via $(ENVTEST)"

.PHONY: golangci-lint
golangci-lint: ## Use golangci-lint through go run when no local binary is available.
	@echo "Using golangci-lint via $(GOLANGCI_LINT)"

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist.
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
ifeq ($(OS),Windows_NT)
define go-install-tool
@setlocal EnableExtensions & set "pkg=$(2)@$(3)" & set "target=$(1)" & if exist "%target%" ( echo "Using %target%" ) else ( echo "Downloading %pkg%" & set "GOBIN=$(LOCALBIN)" & go install %pkg% & if exist "%target%" ( echo "Installed %target%" ) else ( echo "go install did not produce the expected binary" & exit /b 1 ) )
endef
else
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef
endif

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
