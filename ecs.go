package drain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

var (
	ecsClient                      = ecs.New(session.New())
	ec2client                      = ec2.New(session.New())
	ecsRegExp                      = regexp.MustCompile(`ECS_CLUSTER=(\S*)`)
	ErrMissingUserData             = errors.New("This instance seems not to have UserData")
	ErrMissingECSClusterInUserData = errors.New("This instance seems not to have EcsCluster definition in UserData")
)

func Drain(ecsCluster, ec2Instance string) error {
	// Getting ECS Container instance representation by EC2 instance ID
	instance, err := getContainerInstance(ecsCluster, ec2Instance)
	if err != nil {
		return err
	}

	// logging as JSON to look better in CloudWatch logs
	str, _ := json.Marshal(instance)
	fmt.Println("Container instance")
	fmt.Println(string(str))

	// if we have some tasks running on the instance
	// we need to drain it and wait for all tasks to shutdown
	for *instance.RunningTasksCount > 0 {
		// if instance not being drained yet,
		// start the drain
		if *instance.Status != "DRAINING" {
			fmt.Println("Starting draining")
			_, err := ecsClient.UpdateContainerInstancesState(&ecs.UpdateContainerInstancesStateInput{
				Cluster:            &ecsCluster,
				ContainerInstances: []*string{instance.ContainerInstanceArn},
				Status:             aws.String("DRAINING"),
			})
			if err != nil {
				return err
			}
		}

		// Get the instance info, to find out how many tasks still running
		respInstances, err := ecsClient.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
			Cluster:            &ecsCluster,
			ContainerInstances: []*string{instance.ContainerInstanceArn},
		})
		if err != nil {
			return err
		}

		if len(respInstances.ContainerInstances) > 0 {
			instance = respInstances.ContainerInstances[0]
		} else {
			return fmt.Errorf("Something went wrong: Instance not part of the ECS Cluster anymore!")
		}

		fmt.Printf("Waiting for tasks to shutdown... still running #%d\n", *instance.RunningTasksCount)
		time.Sleep(10 * time.Second)
	}

	fmt.Println("Drain finished")
	return nil
}

func getContainerInstance(ecsCluster, ec2Instance string) (*ecs.ContainerInstance, error) {
	respList, err := ecsClient.ListContainerInstances(&ecs.ListContainerInstancesInput{Cluster: &ecsCluster})
	if err != nil {
		return nil, err
	}

	respInstances, err := ecsClient.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
		Cluster:            &ecsCluster,
		ContainerInstances: respList.ContainerInstanceArns,
	})
	if err != nil {
		return nil, err
	}

	for _, i := range respInstances.ContainerInstances {
		if *i.Ec2InstanceId == ec2Instance {
			return i, nil
		}
	}
	return nil, fmt.Errorf("%q not found in the cluster %q", ec2Instance, ecsCluster)
}

func GetClusterNameFromInstanceUserData(ec2Instance string) (string, error) {
	att, err := ec2client.DescribeInstanceAttribute(&ec2.DescribeInstanceAttributeInput{
		InstanceId: &ec2Instance,
		Attribute:  aws.String("userData"),
	})
	if err != nil {
		return "", err
	}

	// checking if we got some user data,
	// if we found none, then instance probably not a part of ECS Cluster
	if att == nil || att.UserData == nil || att.UserData.Value == nil {
		return "", ErrMissingUserData
	}

	decodedUserData, err := base64.StdEncoding.DecodeString(*att.UserData.Value)
	if err != nil {
		return "", err
	}

	// Using RegExp to get actuall ECS Cluster name from UserData string
	m := ecsRegExp.FindAllStringSubmatch(string(decodedUserData), -1)
	if len(m) == 0 || len(m[0]) < 2 {
		fmt.Printf("UserData:\n%s", string(decodedUserData))
		return "", ErrMissingECSClusterInUserData
	}

	// getting ECS Cluster name which we got from UserData
	return m[0][1], nil
}
