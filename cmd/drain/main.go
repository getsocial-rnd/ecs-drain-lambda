package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	drain "github.com/getsocial-rnd/ecs-drain-lambda"
)

func main() {
	lambda.Start(drain.HandleRequest)
}
