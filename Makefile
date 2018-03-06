build: export GOOS=linux
build: 
	go build -o bin/drain cmd/drain/main.go
	@du -h bin/drain 

deploy: build
	sls deploy -v