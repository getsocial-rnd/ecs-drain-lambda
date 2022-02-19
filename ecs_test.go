package drain

import (
	"fmt"
	"testing"
)

func TestECSVariable(t *testing.T) {
	var testcases = []struct {
		testString    string
		expectedValue string
		expectedError error
	}{
		{
			testString:    `echo "ECS_CLUSTER=test-ecs-cluster123" >> /etc/ecs/ecs.config`,
			expectedValue: "test-ecs-cluster123",
		},
		{
			testString:    `echo 'ECS_CLUSTER=test-ecs-cluster123' >> /etc/ecs/ecs.config`,
			expectedValue: "test-ecs-cluster123",
		},
		{
			testString:    `echo ECS_CLUSTER=test-ecs-cluster123 >> /etc/ecs/ecs.config`,
			expectedValue: "test-ecs-cluster123",
		},
		{
			testString: `#!/bin/bash -xe
			echo ECS_CLUSTER=test-ecs-cluster123 >> /etc/ecs/ecs.config`,
			expectedValue: "test-ecs-cluster123",
		},
		{
			testString: `
			write_files:
			- path: /etc/ecs/ecs.config
			  append: true
			  content: |
				ECS_CLUSTER=my-super-ECS-cluster-22abc
				ECS_CONTAINER_STOP_TIMEOUT=2m
`,
			expectedValue: "my-super-ECS-cluster-22abc",
		},
		{
			testString: `
			write_files:
			- path: /etc/ecs/ecs.config
			  append: true
			  content: |
				ECS_CONTAINER_STOP_TIMEOUT=2m
`,
			expectedError: ErrMissingECSClusterInUserData,
		},
		{
			testString:    "",
			expectedError: ErrMissingECSClusterInUserData,
		},
		{
			testString: `
pip install awscli
aws configure set default.region ${AWS::Region}
echo ECS_CLUSTER=my-super-ECS-cluster-22abc >> /etc/ecs/ecs.config
`,
			expectedValue: "my-super-ECS-cluster-22abc",
		},
	}

	for i, tst := range testcases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			val, err := parseECSClusterValue(tst.testString)
			if err != tst.expectedError {
				t.Errorf("Expected %v error, but got %v", tst.expectedError, err)
			}

			if val != tst.expectedValue {
				t.Errorf("Expected %q value, but got %q", tst.expectedValue, val)
			}
		})
	}
}
