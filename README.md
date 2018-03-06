# ecs-drain-lambda

Based on the original idea from [AWS Blog post](https://aws.amazon.com/ru/blogs/compute/how-to-automate-container-instance-draining-in-amazon-ecs/) and [GitHub](https://github.com/aws-samples/ecs-cid-sample). With the following differences:

- Autoscaling Hooks events are received via CloudWatch rules, which makes possible having one function for draining many ECS Clusters

- Reduced number of needed permissions and AWS API calls

- [Serverless Framework](https://github.com/serverless/serverless) based

- Written in Golang

## Why?

When updating AMI for the ECS instances then ASG replaces them without ["Draining"](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/container-instance-draining.html) , which may cause a short downtime of deployed containers. This function automates the ECS Cluster Instances Drain process.

## How does it work?

*ecs-drain-lambda* function:

- Receives **ANY** AutoScaling Lifecycle Terminate event (***[EC2 Auto Scaling Lifecycle Hooks](https://docs.aws.amazon.com/autoscaling/ec2/userguide/lifecycle-hooks.html) for `autoscaling:EC2_INSTANCE_TERMINATING` event should be configured on your ASG***) from CloudWatch Events

- Gets the ID of the instance that has to be terminated

- Looks for the ECS Cluster name in the UserData in the following format: `ECS_CLUSTER=xxxxxxxxx`

- If some ECS Tasks are running on the instance, starts the `Drain` process

- Waits for all the ECS Tasks to shutdown

- [Completes Lifecycle Hook](https://docs.aws.amazon.com/autoscaling/ec2/userguide/lifecycle-hooks.html#completing-lifecycle-hooks), which lets ASG proceed with instance termination

## Requirements

- [Serverless Framework](https://github.com/serverless/serverless)

- [Golang](https://golang.org/doc/install)

- GNU Make

- Configured [EC2 Auto Scaling Lifecycle Hooks](https://docs.aws.amazon.com/autoscaling/ec2/userguide/lifecycle-hooks.html) for `autoscaling:EC2_INSTANCE_TERMINATING` event on your ASG

    Example CloudFormation resource:

        ASGTerminateHook:
           Type: "AWS::AutoScaling::LifecycleHook"
           Properties:
             AutoScalingGroupName: !Ref ECSAutoScalingGroup
             DefaultResult: "ABANDON"
             HeartbeatTimeout: "900"
             LifecycleTransition: "autoscaling:EC2_INSTANCE_TERMINATING"

## How to use

- Clone the repo with `git clone`

- Enter the project directory `cd ecs-drain-lambda`

- Run `make deploy`

## Limitations

- Function waits for 5 minutes for Drain to complete and fails with the timeout after

- If function fails, then the default lifecycle hook action will be triggered (`ABANDON` or `CONTINUE` depending on your Hook configuration), either result will end up with eventual instance termination.

    [Documentation](https://docs.aws.amazon.com/autoscaling/ec2/userguide/lifecycle-hooks.html#lifecycle-hook-considerations)
        
        If the instance is terminating, both ABANDON and CONTINUE allow the instance to terminate. However, ABANDON stops any remaining actions, such as other lifecycle hooks, while CONTINUE allows any other lifecycle hooks to complete.