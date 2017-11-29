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

package instancegroups

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	api "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/cloudinstances"
	"k8s.io/kops/upup/pkg/fi"
)

// RollingUpdateCluster is a struct containing cluster information for a rolling update.
type RollingUpdateCluster struct {
	Cloud fi.Cloud

	// MasterInterval is the amount of time to wait after stopping a master instance
	MasterInterval time.Duration
	// NodeInterval is the amount of time to wait after stopping a non-master instance
	NodeInterval time.Duration
	// BastionInterval is the amount of time to wait after stopping a bastion instance
	BastionInterval time.Duration

	Force bool

	K8sClient        kubernetes.Interface
	ClientConfig     clientcmd.ClientConfig
	FailOnDrainError bool
	FailOnValidate   bool
	CloudOnly        bool
	ClusterName      string

	// PostDrainDelay is the duration we wait after draining each node
	PostDrainDelay time.Duration

	// ValidationTimeout is the maximum time to wait for the cluster to validate, once we start validation
	ValidationTimeout time.Duration
}

// RollingUpdate performs a rolling update on a K8s Cluster.
func (c *RollingUpdateCluster) RollingUpdate(groups map[string]*cloudinstances.CloudInstanceGroup, instanceGroups *api.InstanceGroupList) error {
	if len(groups) == 0 {
		glog.Infof("Cloud Instance Group length is zero. Not doing a rolling-update.")
		return nil
	}

	var resultsMutex sync.Mutex
	results := make(map[string]error)

	masterGroups := make(map[string]*cloudinstances.CloudInstanceGroup)
	nodeGroups := make(map[string]*cloudinstances.CloudInstanceGroup)
	bastionGroups := make(map[string]*cloudinstances.CloudInstanceGroup)
	for k, group := range groups {
		switch group.InstanceGroup.Spec.Role {
		case api.InstanceGroupRoleNode:
			nodeGroups[k] = group
		case api.InstanceGroupRoleMaster:
			masterGroups[k] = group
		case api.InstanceGroupRoleBastion:
			bastionGroups[k] = group
		default:
			return fmt.Errorf("unknown group type for group %q", group.InstanceGroup.ObjectMeta.Name)
		}
	}

	// Upgrade bastions first; if these go down we can't see anything
	{
		var wg sync.WaitGroup

		for k, bastionGroup := range bastionGroups {
			wg.Add(1)
			go func(k string, group *cloudinstances.CloudInstanceGroup) {
				resultsMutex.Lock()
				results[k] = fmt.Errorf("function panic bastions")
				resultsMutex.Unlock()

				defer wg.Done()

				g, err := NewRollingUpdateInstanceGroup(c.Cloud, group)
				if err == nil {
					err = g.RollingUpdate(c, instanceGroups, true, c.BastionInterval, c.ValidationTimeout)
				}

				resultsMutex.Lock()
				results[k] = err
				resultsMutex.Unlock()
			}(k, bastionGroup)
		}

		wg.Wait()
	}

	// Upgrade master next
	{
		var wg sync.WaitGroup

		// We run master nodes in series, even if they are in separate instance groups
		// typically they will be in separate instance groups, so we can force the zones,
		// and we don't want to roll all the masters at the same time.  See issue #284
		wg.Add(1)

		go func() {
			for k := range masterGroups {
				resultsMutex.Lock()
				results[k] = fmt.Errorf("function panic masters")
				resultsMutex.Unlock()
			}

			defer wg.Done()

			for k, group := range masterGroups {
				g, err := NewRollingUpdateInstanceGroup(c.Cloud, group)
				if err == nil {
					err = g.RollingUpdate(c, instanceGroups, false, c.MasterInterval, c.ValidationTimeout)
				}

				resultsMutex.Lock()
				results[k] = err
				resultsMutex.Unlock()

				// TODO: Bail on error?
			}
		}()

		wg.Wait()
	}

	// Upgrade nodes, with greater parallelism
	{
		var wg sync.WaitGroup

		// We run nodes in series, even if they are in separate instance groups
		// typically they will not being separate instance groups. If you roll the nodes in parallel
		// you can get into a scenario where you can evict multiple statefulset pods from the same
		// statefulset at the same time. Further improvements needs to be made to protect from this as
		// well.

		wg.Add(1)

		go func() {
			for k := range nodeGroups {
				resultsMutex.Lock()
				results[k] = fmt.Errorf("function panic nodes")
				resultsMutex.Unlock()
			}

			defer wg.Done()

			for k, group := range nodeGroups {
				g, err := NewRollingUpdateInstanceGroup(c.Cloud, group)
				if err == nil {
					err = g.RollingUpdate(c, instanceGroups, false, c.NodeInterval, c.ValidationTimeout)
				}

				resultsMutex.Lock()
				results[k] = err
				resultsMutex.Unlock()

				// TODO: Bail on error?
			}
		}()

		wg.Wait()
	}

	for _, err := range results {
		if err != nil {
			return err
		}
	}

	glog.Infof("Rolling update completed for cluster %q!", c.ClusterName)
	return nil
}
