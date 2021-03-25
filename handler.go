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

var asgClient *autoscaling.AutoScaling

const (
	EventASGTerminateDetailType       = "EC2 Instance-terminate Lifecycle Action"
	EventEC2SpotInteruptionDetailType = "EC2 Spot Instance Interruption Warning"
)

type (
	ASGLifecycleEventDetail struct {
		LifecycleActionToken string
		AutoScalingGroupName string
		LifecycleHookName    string
		EC2InstanceID        string
		LifecycleTransition  string
	}
	EC2SpotInterruptionEventDetail struct {
		InstanceID     string `json:"instance-id"`
		InstanceAction string `json:"instance-action"`
	}
)

func HandleRequest(ctx context.Context, event *events.CloudWatchEvent) error {
	printJSON("CloudWatch Event", event)

	var instanceToDrain string
	var finalAction = func() error { return nil }

	switch event.DetailType {
	case EventASGTerminateDetailType:
		asgEvent := &ASGLifecycleEventDetail{}
		if err := json.Unmarshal(event.Detail, &asgEvent); err != nil {
			return err
		}

		instanceToDrain = asgEvent.EC2InstanceID
		finalAction = asgEvent.CompleteLifecycle
	case EventEC2SpotInteruptionDetailType:
		spotEvent := &EC2SpotInterruptionEventDetail{}
		if err := json.Unmarshal(event.Detail, &spotEvent); err != nil {
			return err
		}

		instanceToDrain = spotEvent.InstanceID
	}

	// Getting instance UserData, because if ECS Instance was
	// created via CloudFormation, then ECS Cluster name
	// should be specified there in next format:
	// // #!/bin/bash -xe
	// // echo ECS_CLUSTER=${ECSCluster} >> /etc/ecs/ecs.config
	// // ...
	ecsCluster, err := GetClusterNameFromInstanceUserData(instanceToDrain)
	switch err {
	case nil:
		// do nothing
	case ErrMissingUserData:
		fmt.Printf("No UserData not found, instance %q probably not part of an ECS Cluster\n", instanceToDrain)
	case ErrInstanceTerminated:
		fmt.Printf("Instance %q already terminated", instanceToDrain)
		// if instance is already terminated, try compliting final action anyway
		// this is needed for the Spot instances particulary, see here:
		// https://docs.aws.amazon.com/en_us/autoscaling/ec2/userguide/lifecycle-hooks.html#lifecycle-hook-spot
		return finalAction()
	}

	fmt.Println("ECS Cluster -", ecsCluster)
	fmt.Println("EC2 Instance - ", instanceToDrain)

	// Doing actual "Drain" operation, which will move all the running
	// task from current instance to other aviliable instances.
	// This operation could take some time, so if we have a lot of task
	// on the instance, but 15 minutes lambda limit, should be enough
	if err := Drain(ecsCluster, instanceToDrain); err != nil {
		return err
	}

	// Do the final actions after the drain which is:
	// - Complete lifecycle action for ASG Hook
	// - Nothing for the Spot Interruption
	return finalAction()
}

func (asgEvent *ASGLifecycleEventDetail) CompleteLifecycle() error {
	// lazy init asgClient
	if asgClient == nil {
		asgClient = autoscaling.New(session.New())
	}

	// TODO?
	// if the Drain fails, after timeout hook will be completed with ABANDON result,
	// which in case of instance termination will still Terminate the instance
	// https://docs.aws.amazon.com/autoscaling/ec2/userguide/lifecycle-hooks.html#lifecycle-hook-considerations
	// may be it worth to set Scale In protection on the instance, to complete the Drain manually?

	// Once instance drained, we can proceed with the termination
	if _, err := asgClient.CompleteLifecycleAction(&autoscaling.CompleteLifecycleActionInput{
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
