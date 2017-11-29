/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mockroute53

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
)

func (m *MockRoute53) ListResourceRecordSetsRequest(*route53.ListResourceRecordSetsInput) (*request.Request, *route53.ListResourceRecordSetsOutput) {
	panic("MockRoute53 ListResourceRecordSetsRequest not implemented")
	return nil, nil
}

func (m *MockRoute53) ListResourceRecordSets(*route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	panic("MockRoute53 ListResourceRecordSets not implemented")
	return nil, nil
}

func (m *MockRoute53) ListResourceRecordSetsPages(request *route53.ListResourceRecordSetsInput, callback func(*route53.ListResourceRecordSetsOutput, bool) bool) error {
	glog.Infof("ListResourceRecordSetsPages %v", request)

	if request.HostedZoneId == nil {
		// TODO: Use correct error
		return fmt.Errorf("HostedZoneId required")
	}

	if request.StartRecordIdentifier != nil || request.StartRecordName != nil || request.StartRecordType != nil || request.MaxItems != nil {
		glog.Fatalf("Unsupported options: %v", request)
	}

	zone := m.findZone(*request.HostedZoneId)

	if zone == nil {
		// TODO: Use correct error
		return fmt.Errorf("NOT FOUND")
	}

	page := &route53.ListResourceRecordSetsOutput{}
	for _, r := range zone.records {
		copy := *r
		page.ResourceRecordSets = append(page.ResourceRecordSets, &copy)
	}
	lastPage := true
	callback(page, lastPage)

	return nil
}

func (m *MockRoute53) ChangeResourceRecordSetsRequest(*route53.ChangeResourceRecordSetsInput) (*request.Request, *route53.ChangeResourceRecordSetsOutput) {
	panic("MockRoute53 ChangeResourceRecordSetsRequest not implemented")
	return nil, nil
}

func (m *MockRoute53) ChangeResourceRecordSets(request *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	glog.Infof("ChangeResourceRecordSets %v", request)

	if request.HostedZoneId == nil {
		// TODO: Use correct error
		return nil, fmt.Errorf("HostedZoneId required")
	}
	zone := m.findZone(*request.HostedZoneId)
	if zone == nil {
		// TODO: Use correct error
		return nil, fmt.Errorf("NOT FOUND")
	}

	response := &route53.ChangeResourceRecordSetsOutput{
		ChangeInfo: &route53.ChangeInfo{},
	}
	for _, change := range request.ChangeBatch.Changes {
		changeType := aws.StringValue(change.ResourceRecordSet.Type)
		changeName := aws.StringValue(change.ResourceRecordSet.Name)

		foundIndex := -1
		for i, rr := range zone.records {
			if aws.StringValue(rr.Type) != changeType {
				continue
			}
			if aws.StringValue(rr.Name) != changeName {
				continue
			}
			foundIndex = i
			break
		}

		switch aws.StringValue(change.Action) {
		case "UPSERT":
			if foundIndex == -1 {
				zone.records = append(zone.records, change.ResourceRecordSet)
			} else {
				zone.records[foundIndex] = change.ResourceRecordSet
			}

		case "CREATE":
			if foundIndex == -1 {
				zone.records = append(zone.records, change.ResourceRecordSet)
			} else {
				// TODO: Use correct error
				return nil, fmt.Errorf("duplicate record %s %q", changeType, changeName)
			}

		default:
			// TODO: Use correct error
			return nil, fmt.Errorf("Unsupported action: %q", aws.StringValue(change.Action))
		}
	}

	return response, nil
}
