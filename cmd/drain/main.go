package main

import (
	"bitbucket.org/getsocial/ecs-drain-lambda"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(drain.HandleRequest)
}
