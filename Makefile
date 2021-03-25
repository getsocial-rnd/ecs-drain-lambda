build: export GOOS=linux
build: 
	go build -o bin/drain cmd/drain/main.go
	@du -h bin/drain 

deploy: build
	sls deploy -v

outdated-deps: ## get list of outdated direct dependencies
	@go list -u -f '{{if and (.Update) (not .Indirect)}}{{.}}{{end}}' -m all