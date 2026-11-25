UPSNAP_VERSION ?= 5.7.3

default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

docs-validate:
	cd tools; go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs validate --provider-dir .. --provider-name upsnap

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate docs-validate
