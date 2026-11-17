default: fmt lint build generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

# Formats the examples and generates docs/ from them, the templates and the
# provider's schema, which OpenTofu reports.
generate:
	cd tools; go generate ./...

docs-validate:
	sh tools/docs.sh validate

fmt:
	gofmt -s -w -e .

# The client's unit tests; the resource tests skip without TF_ACC.
test:
	go test -v -cover -timeout=120s ./...

# Every test, the resource tests driving OpenTofu against an in-memory fake of
# OneSignal's API. Nothing reaches OneSignal.
testacc:
	TF_ACC=1 go test -v -cover -timeout 20m ./...

# The live tests, against OneSignal itself: see internal/provider/live_test.go.
testlive:
	ONESIGNAL_LIVE=1 TF_ACC=1 go test -v -timeout 20m -run '^TestLive' ./internal/provider/

.PHONY: default build install lint generate docs-validate fmt test testacc testlive
