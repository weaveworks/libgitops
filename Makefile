UID_GID?=$(shell id -u):$(shell id -g)
GO_VERSION=1.12.7
DOCKER_USER=weaveworks
GIT_VERSION:=$(shell hack/ldflags.sh --version-only)
PROJECT = github.com/weaveworks/libgitops
BOUNDING_API_DIRS = ${PROJECT}/pkg,${PROJECT}/cmd/apis
API_DIRS = ${PROJECT}/cmd/sample-app/apis/sample,${PROJECT}/cmd/sample-app/apis/sample/v1alpha1,${PROJECT}/pkg/runtime
CACHE_DIR = $(shell pwd)/bin/cache
API_DOCS = api/sample-app.md api/runtime.md

all: sample-app

BINARIES=sample-app
$(BINARIES):
	$(MAKE) shell COMMAND="make bin/$@"

# Make make execute this target although the file already exists.
.PHONY: bin/sample-app
bin/sample-app: bin/%:
	CGO_ENABLED=0 go build -mod=vendor -ldflags "$(shell ./hack/ldflags.sh)" -o bin/$* ./cmd/$*

shell:
	mkdir -p $(CACHE_DIR)/go $(CACHE_DIR)/cache
	docker run -it --rm \
		-v $(CACHE_DIR)/go:/go \
		-v $(CACHE_DIR)/cache:/.cache/go-build \
		-v $(shell pwd):/go/src/${PROJECT} \
		-w /go/src/${PROJECT} \
		-u $(shell id -u):$(shell id -g) \
		-e GO111MODULE=on \
		golang:$(GO_VERSION) \
		$(COMMAND)

tidy:
	$(MAKE) shell COMMAND="make dockerized-tidy"

dockerized-tidy: /go/bin/goimports
	go mod tidy
	go mod vendor
	hack/generate-client.sh
	gofmt -s -w pkg cmd
	goimports -w pkg cmd

autogen:
	$(MAKE) shell COMMAND="make dockerized-autogen"

dockerized-autogen: /go/bin/deepcopy-gen /go/bin/defaulter-gen /go/bin/conversion-gen /go/bin/openapi-gen
	# Let the boilerplate be empty
	touch /tmp/boilerplate
	/go/bin/deepcopy-gen \
		--input-dirs ${API_DIRS} \
		--bounding-dirs ${BOUNDING_API_DIRS} \
		-O zz_generated.deepcopy \
		-h /tmp/boilerplate 

	/go/bin/defaulter-gen \
		--input-dirs ${API_DIRS} \
		-O zz_generated.defaults \
		-h /tmp/boilerplate

	/go/bin/conversion-gen \
		--input-dirs ${API_DIRS} \
		-O zz_generated.conversion \
		-h /tmp/boilerplate
	
	/go/bin/openapi-gen \
		--input-dirs ${API_DIRS} \
		--output-package ${PROJECT}/api/openapi \
		--report-filename api/openapi/violations.txt \
		-h /tmp/boilerplate

/go/bin/deepcopy-gen /go/bin/defaulter-gen /go/bin/conversion-gen: /go/bin/%:
	go get k8s.io/code-generator/cmd/$*

/go/bin/openapi-gen:
	go get k8s.io/kube-openapi/cmd/openapi-gen

/go/bin/goimports:
	go get golang.org/x/tools/cmd/goimports