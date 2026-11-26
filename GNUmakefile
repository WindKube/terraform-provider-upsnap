UPSNAP_PORT ?= 8090
UPSNAP_USERNAME ?= admin@example.com
UPSNAP_PASSWORD ?= tfacc-password-1234
export UPSNAP_PORT

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
	TF_ACC=1 UPSNAP_ENDPOINT=$${UPSNAP_ENDPOINT:-http://127.0.0.1:$(UPSNAP_PORT)} UPSNAP_USERNAME=$(UPSNAP_USERNAME) UPSNAP_PASSWORD=$(UPSNAP_PASSWORD) \
		go test -v -cover -timeout 120m ./...

# UpSnap ignores CLI subcommands while UPSNAP_HTTP_LISTEN is set, so it is cleared for the upsert.
testacc-up:
	docker compose up -d --wait
	docker compose exec -T -e UPSNAP_HTTP_LISTEN= upsnap /app/upsnap superuser upsert $(UPSNAP_USERNAME) $(UPSNAP_PASSWORD)

testacc-down:
	docker compose down -v

.PHONY: fmt lint test testacc testacc-up testacc-down build install generate docs-validate
