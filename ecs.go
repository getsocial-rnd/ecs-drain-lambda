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
	ecsRegExp                      = regexp.MustCompile(`ECS_CLUSTER=([0-9A-Za-z_\-]*)`)
	ErrMissingUserData             = errors.New("This instance seems not to have UserData")
	ErrMissingECSClusterInUserData = errors.New("This instance seems not to have EcsCluster definition in UserData")
	ErrInstanceTerminated          = errors.New("This instance is already terminated")
)

func Drain(ecsCluster, ec2Instance string) error {
	// Getting ECS Container instance representation by EC2 instance ID
	instance, err := getContainerInstance(ecsCluster, ec2Instance)
	if err != nil {
		return err
	}

	printJSON("Container instance", instance)

	var tasksToShutdownCount int64
	if instance != nil && instance.RunningTasksCount != nil {
		tasksToShutdownCount = *instance.RunningTasksCount
	}

	var runningTaskArns []*string
	// if we have some tasks running on the instance
	// we need to drain it and wait for all tasks to shutdown
	for tasksToShutdownCount > 0 {
		// if instance not being drained yet,
		// start the drain
		if *instance.Status != ecs.ContainerInstanceStatusDraining {
			fmt.Println("Starting draining and waiting for all tasks to shutdown")
			_, err := ecsClient.UpdateContainerInstancesState(&ecs.UpdateContainerInstancesStateInput{
				Cluster:            &ecsCluster,
				ContainerInstances: []*string{instance.ContainerInstanceArn},
				Status:             aws.String(ecs.ContainerInstanceStatusDraining),
			})
			if err != nil {
				return err
			}

			// fetch list of tasks running on that instance
			resp, err := ecsClient.ListTasks(&ecs.ListTasksInput{
				ContainerInstance: instance.ContainerInstanceArn,
				Cluster:           &ecsCluster,
			})
			if err != nil {
				return err
			}
			if resp != nil {
				runningTaskArns = resp.TaskArns
			}

			// update instance information, to be sure that it started draining
			instance, err = getContainerInstance(ecsCluster, ec2Instance)
			if err != nil {
				return err
			}
		}

		if len(runningTaskArns) == 0 {
			fmt.Println("no running tasks found")
			break
		}

		// monitor status of the tasks running on the current instance
		tasks, err := ecsClient.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: &ecsCluster,
			Tasks:   runningTaskArns,
		})
		if err != nil {
			return err
		}

		if tasks == nil || len(tasks.Tasks) == 0 {
			fmt.Println("no tasks found")
		}

		taskStates := map[string]int{}
		tasksToShutdownCount = 0

		for _, task := range tasks.Tasks {
			// wait explicitly for tasks to become "STOPPED"
			// other way we may stop the instance with the tasks that
			// are still being in the "DEACTIVATING" state
			// see https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-lifecycle.html
			if task.LastStatus == nil {
				continue
			}

			taskStates[*task.LastStatus]++
			if *task.LastStatus != ecs.DesiredStatusStopped {
				tasksToShutdownCount++
			}
		}

		printJSON("Instance task states", taskStates)
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
	// check instance state, error if already terminated
	resp, err := ec2client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{&ec2Instance},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Reservations) > 0 && len(resp.Reservations[0].Instances) > 0 {
		switch *resp.Reservations[0].Instances[0].State.Name {
		case ec2.InstanceStateNameTerminated, ec2.InstanceStateNameShuttingDown:
			return "", ErrInstanceTerminated
		}
	}

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

	// Using RegExp to get actual ECS Cluster name from UserData string
	m := ecsRegExp.FindAllStringSubmatch(string(decodedUserData), -1)
	if len(m) == 0 || len(m[0]) < 2 {
		fmt.Printf("UserData:\n%s", string(decodedUserData))
		return "", ErrMissingECSClusterInUserData
	}

	// getting ECS Cluster name which we got from UserData
	return m[0][1], nil
}

// AWS CloudWatch Logs prints only JSONs nicely
func printJSON(text string, data interface{}) {
	if b, err := json.Marshal(data); err == nil {
		fmt.Println(text, string(b))
	}
}
