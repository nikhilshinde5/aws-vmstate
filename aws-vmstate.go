package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"time"

	//"flag"

	"fmt"

	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

var client *ec2.Client

// EC2StartInstancesAPI defines the interface for the StartInstances function.
// We use this interface to test the function using a mocked service.
type EC2StartInstancesAPI interface {
	StartInstances(ctx context.Context,
		params *ec2.StartInstancesInput,
		optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	ModifyInstanceAttribute(ctx context.Context,
		params *ec2.ModifyInstanceAttributeInput,
		optFns ...func(*ec2.Options)) (*ec2.ModifyInstanceAttributeOutput, error)
	StopInstances(ctx context.Context,
		params *ec2.StopInstancesInput,
		optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
}

type ConfigMap struct {
	InstanceType string `json:"instance_type"`
	ImageId      string `json:"image_id"`
}

// StartInstance starts an Amazon Elastic Compute Cloud (Amazon EC2) instance.
// Inputs:
//
//	c is the context of the method call, which includes the AWS Region.
//	api is the interface that defines the method call.
//	input defines the input arguments to the service call.
//
// Output:
//
//	If success, a StartInstancesOutput object containing the result of the service call and nil.
//	Otherwise, nil and an error from the call to StartInstances.

func UpdateInstanceAttribute(c context.Context, api EC2StartInstancesAPI, input *ec2.ModifyInstanceAttributeInput) (*ec2.ModifyInstanceAttributeOutput, error) {
	return api.ModifyInstanceAttribute(c, input)
}

func PauseInstances(c context.Context, api EC2StartInstancesAPI, input *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error) {
	return api.StopInstances(c, input)
}

func StartInstances(c context.Context, api EC2StartInstancesAPI, input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	resp, err := api.StartInstances(c, input)

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "DryRunOperation" {
		fmt.Println("User has permission to start an instance.")
		input.DryRun = aws.Bool(false)
		return api.StartInstances(c, input)
	}
	return resp, err
}

func StartInstancesCmd(client EC2StartInstancesAPI, instanceIds []string) {

	fmt.Println(instanceIds)
	input := &ec2.StartInstancesInput{
		InstanceIds: instanceIds,
		DryRun:      aws.Bool(true),
	}
	_, err := StartInstances(context.TODO(), client, input)
	if err != nil {
		fmt.Println("Got an error starting the instance")
		fmt.Println(err)
		//return
	}
	fmt.Println("Started instances with IDs " + strings.Join(instanceIds, ","))
}

func init() {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic("configuration error, " + err.Error())
	}
	client = ec2.NewFromConfig(cfg)

}
func main() {
	name := flag.String("n", "", "The name of the tag to attach to the instance")
	value := flag.String("v", "", "The value of the tag to attach to the instance")

	flag.Parse()

	if *name == "" || *value == "" {
		fmt.Println("You must supply a name and value for the tag (-n NAME -v VALUE)")
		return
	}

	file, err := os.Open("data/config.json")
	if err != nil {
		fmt.Println("Error opening config file:", err)
		os.Exit(1)
	}
	defer file.Close()

	var config ConfigMap
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		fmt.Println("Error decoding config:", err)
		os.Exit(1)
	}

	var instanceType = config.InstanceType
	for {

		var instanceIds = make([]string, 0)
		input1 := &ec2.DescribeInstancesInput{
			Filters: []types.Filter{
				{
					Name: aws.String("tag:" + *name),
					Values: []string{
						*value,
					},
				},
			},
		}

		result, err := client.DescribeInstances(context.TODO(), input1)
		if err != nil {
			fmt.Println("Got an error fetching the status of the instance")
			fmt.Println(err)
		} else {

			fmt.Println(result)
			for _, r := range result.Reservations {
				fmt.Println("Reservation ID: " + *r.ReservationId)
				fmt.Println("Instance IDs:")
				for _, i := range r.Instances {
					fmt.Println("   " + *i.InstanceId)
					//value := *i.InstanceId
					instanceIds = append(instanceIds, *i.InstanceId)
				}

				fmt.Println(instanceIds)
			}

			file, err := os.Open("data/config.json")
			if err != nil {
				fmt.Println("Error opening config file:", err)
				os.Exit(1)
			}
			defer file.Close()

			var config ConfigMap
			if err := json.NewDecoder(file).Decode(&config); err != nil {
				fmt.Println("Error decoding config:", err)
				os.Exit(1)
			}

			var newInstanceType = config.InstanceType
			if newInstanceType != instanceType {

				fmt.Printf("Found change in instance type: %v --> %v \n", instanceType, newInstanceType)

				instanceID := instanceIds[0]

				//time.Sleep(30 * time.Second)

				//Stopping instances before changing instance type

				fmt.Println("Stopping instances before changing instance type")
				stopInstancesInput := &ec2.StopInstancesInput{
					InstanceIds: []string{instanceID},
					Force:       aws.Bool(false),
				}
				_, err = PauseInstances(context.TODO(), client, stopInstancesInput)
				if err != nil {
					fmt.Println("Got an error stoping the instance:")
					fmt.Println(err)
					return
				}

				//this sleep for letting ec2 stop
				time.Sleep(60 * time.Second)

				fmt.Println("Updating instance type of instance with ID: ", instanceID)
				attributeInput := &ec2.ModifyInstanceAttributeInput{
					InstanceId: &instanceID,
					InstanceType: &types.AttributeValue{
						Value: &newInstanceType,
					},
				}

				_, err = UpdateInstanceAttribute(context.TODO(), client, attributeInput)
				if err != nil {
					fmt.Println("Got an error updating the instance:")
					fmt.Println(err)
					return
				}

				instanceType = newInstanceType
				fmt.Println("Modified instance type of ec2 instance with Instance ID: ", instanceID)
			}

			input := &ec2.DescribeInstanceStatusInput{
				InstanceIds:         instanceIds,
				IncludeAllInstances: aws.Bool(true),
			}
			output, err := client.DescribeInstanceStatus(context.TODO(), input)
			if err != nil {
				fmt.Println("Got an error fetching the status of the instance")
				fmt.Println(err)
			} else {
				fmt.Println(output)
				if len(output.InstanceStatuses) != 1 {
					fmt.Println("The total number of instances did not match the request")
				}
				//////////////////////////////////////////////////////////////////////////////

				for _, instanceStatus := range output.InstanceStatuses {
					fmt.Println("+++++++++++++++++++++++++++++++++++++++++")
					fmt.Println("status check loop\n")
					fmt.Println(*instanceStatus.InstanceId, instanceStatus.InstanceState.Name)
					for key, value := range instanceIds {
						if *instanceStatus.InstanceId == value {
							fmt.Println("instance is found in config file")
							fmt.Printf(" %v : %v \n", key, value)
							if instanceStatus.InstanceState.Name == "running" {
								fmt.Println("instance is running\n")
							} else {
								fmt.Println("instance is not running\n")
								StartInstancesCmd(client, []string{*instanceStatus.InstanceId})
							}
						}
					} //key search ends
				} //instance id check ends
			} //aws-sdk call ends
			fmt.Println("+++++++++++++++++++++++++++++++++++++++++")
		} //instance exists
		time.Sleep(60 * time.Second)

	} //for loop ends
}
