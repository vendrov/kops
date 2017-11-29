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

package model

import (
	"github.com/golang/glog"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awstasks"
)

// ExternalAccessModelBuilder configures security group rules for external access
// (SSHAccess, KubernetesAPIAccess)
type ExternalAccessModelBuilder struct {
	*KopsModelContext
	Lifecycle *fi.Lifecycle
}

var _ fi.ModelBuilder = &ExternalAccessModelBuilder{}

func (b *ExternalAccessModelBuilder) Build(c *fi.ModelBuilderContext) error {
	if len(b.Cluster.Spec.KubernetesAPIAccess) == 0 {
		glog.Warningf("KubernetesAPIAccess is empty")
	}

	if len(b.Cluster.Spec.SSHAccess) == 0 {
		glog.Warningf("SSHAccess is empty")
	}

	// SSH is open to AdminCIDR set
	if b.UsesSSHBastion() {
		// If we are using a bastion, we only access through the bastion
		// This is admittedly a little odd... adding a bastion shuts down direct access to the masters/nodes
		// But I think we can always add more permissions in this case later, but we can't easily take them away
		glog.V(2).Infof("bastion is in use; won't configure SSH access to master / node instances")
	} else {
		for _, sshAccess := range b.Cluster.Spec.SSHAccess {
			c.AddTask(&awstasks.SecurityGroupRule{
				Name:          s("ssh-external-to-master-" + sshAccess),
				Lifecycle:     b.Lifecycle,
				SecurityGroup: b.LinkToSecurityGroup(kops.InstanceGroupRoleMaster),
				Protocol:      s("tcp"),
				FromPort:      i64(22),
				ToPort:        i64(22),
				CIDR:          s(sshAccess),
			})

			c.AddTask(&awstasks.SecurityGroupRule{
				Name:          s("ssh-external-to-node-" + sshAccess),
				Lifecycle:     b.Lifecycle,
				SecurityGroup: b.LinkToSecurityGroup(kops.InstanceGroupRoleNode),
				Protocol:      s("tcp"),
				FromPort:      i64(22),
				ToPort:        i64(22),
				CIDR:          s(sshAccess),
			})
		}
	}

	for _, nodePortAccess := range b.Cluster.Spec.NodePortAccess {
		nodePortRange, err := b.NodePortRange()
		if err != nil {
			return err
		}

		c.AddTask(&awstasks.SecurityGroupRule{
			Name:          s("nodeport-tcp-external-to-node-" + nodePortAccess),
			Lifecycle:     b.Lifecycle,
			SecurityGroup: b.LinkToSecurityGroup(kops.InstanceGroupRoleNode),
			Protocol:      s("tcp"),
			FromPort:      i64(int64(nodePortRange.Base)),
			ToPort:        i64(int64(nodePortRange.Base + nodePortRange.Size - 1)),
			CIDR:          s(nodePortAccess),
		})
		c.AddTask(&awstasks.SecurityGroupRule{
			Name:          s("nodeport-udp-external-to-node-" + nodePortAccess),
			Lifecycle:     b.Lifecycle,
			SecurityGroup: b.LinkToSecurityGroup(kops.InstanceGroupRoleNode),
			Protocol:      s("udp"),
			FromPort:      i64(int64(nodePortRange.Base)),
			ToPort:        i64(int64(nodePortRange.Base + nodePortRange.Size - 1)),
			CIDR:          s(nodePortAccess),
		})
	}

	if !b.UseLoadBalancerForAPI() {
		// Configuration for the master, when not using a Loadbalancer (ELB)
		// We expect that either the IP address is published, or DNS is set up to point to the IPs
		// We need to open security groups directly to the master nodes (instead of via the ELB)

		// HTTPS to the master is allowed (for API access)
		for _, apiAccess := range b.Cluster.Spec.KubernetesAPIAccess {
			t := &awstasks.SecurityGroupRule{
				Name:          s("https-external-to-master-" + apiAccess),
				Lifecycle:     b.Lifecycle,
				SecurityGroup: b.LinkToSecurityGroup(kops.InstanceGroupRoleMaster),
				Protocol:      s("tcp"),
				FromPort:      i64(443),
				ToPort:        i64(443),
				CIDR:          s(apiAccess),
			}
			c.AddTask(t)
		}
	}

	return nil
}
