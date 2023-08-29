GOOS=linux
GOARCH=amd64
CGO_ENABLED=0
export

build:
	go build -tags lambda.norpc -o bootstrap cmd/drain/main.go
	@du -h bootstrap

deploy: build
	sls deploy -v

outdated-deps: ## get list of outdated direct dependencies
	@go list -u -f '{{if and (.Update) (not .Indirect)}}{{.}}{{end}}' -m all
