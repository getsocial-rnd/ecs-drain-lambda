package drain

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
)

var asgClient = autoscaling.New(session.New())

type ASGLifecycleEvent struct {
	LifecycleActionToken string
	AutoScalingGroupName string
	LifecycleHookName    string
	EC2InstanceID        string
	LifecycleTransition  string
}

func HandleRequest(ctx context.Context, event *events.CloudWatchEvent) error {
	str, _ := json.Marshal(event)
	fmt.Println("Got CloudWatch Event:")
	fmt.Println(string(str))

	var asgEvent = &ASGLifecycleEvent{}
	if err := json.Unmarshal(event.Detail, &asgEvent); err != nil {
		return err
	}

	// Getting instance UserData, because if ECS Instance was
	// created via CloudFormation, then ECS Cluster name
	// should be specified there in next format:
	// // #!/bin/bash -xe
	// // echo ECS_CLUSTER=${ECSCluster} >> /etc/ecs/ecs.config
	// // ...
	ecsCluster, err := GetClusterNameFromInstanceUserData(asgEvent.EC2InstanceID)
	switch err {
	case nil:
		// do nothing
	case ErrMissingUserData:
		fmt.Printf("No UserData not found, instance %q probably not part of an ECS Cluster\n", asgEvent.EC2InstanceID)
		return nil
	case ErrMissingECSClusterInUserData:
		fmt.Printf("No ECSCLuster definition found in instance %q UserData\n", asgEvent.EC2InstanceID)
		return nil
	default:
		return err
	}

	fmt.Println("ECS Cluster -", ecsCluster)
	fmt.Println("EC2 Instance - ", asgEvent.EC2InstanceID)

	// Doing actuall "Drain" operation, which will move all the running
	// task from current instance to other aviliable instances.
	// This operation could take some time, so if we have a lot of task
	// on the instance, probably it make sense to extend wait time
	// TODO: track lambda execution time and start new one if needed
	if err := Drain(ecsCluster, asgEvent.EC2InstanceID); err != nil {
		return err
	}

	// TODO?
	// if the Drain fails, after timeout hook will be completed with ABANDON result,
	// which in case of instance termination will still Terminate the instance
	// https://docs.aws.amazon.com/autoscaling/ec2/userguide/lifecycle-hooks.html#lifecycle-hook-considerations
	// may be it worth to set Scale In protection on the instance, to complete the Drain manually?

	// Once instance drained, we can proceed with the termination
	if _, err = asgClient.CompleteLifecycleAction(&autoscaling.CompleteLifecycleActionInput{
		InstanceId:            &asgEvent.EC2InstanceID,
		LifecycleHookName:     &asgEvent.LifecycleHookName,
		LifecycleActionToken:  &asgEvent.LifecycleActionToken,
		AutoScalingGroupName:  &asgEvent.AutoScalingGroupName,
		LifecycleActionResult: aws.String("CONTINUE"),
	}); err != nil {
		return err
	}

	fmt.Printf("Lifecycle hook action %q (%s) completed for ASG %q and InstanceID %q\n",
		asgEvent.LifecycleHookName,
		asgEvent.LifecycleActionToken,
		asgEvent.AutoScalingGroupName,
		asgEvent.EC2InstanceID)

	return nil
}
