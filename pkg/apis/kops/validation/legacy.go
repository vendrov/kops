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

package validation

import (
	"fmt"
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/apis/kops/util"
	"k8s.io/kops/upup/pkg/fi"

	"github.com/blang/semver"
)

// legacy contains validation functions that don't match the apimachinery style

// ValidateCluster is responsible for checking the validitity of the Cluster spec
func ValidateCluster(c *kops.Cluster, strict bool) *field.Error {
	fieldSpec := field.NewPath("Spec")
	var err error

	// kubernetesRelease is the version with only major & minor fields
	var kubernetesRelease semver.Version

	// KubernetesVersion
	if c.Spec.KubernetesVersion == "" {
		return field.Required(fieldSpec.Child("KubernetesVersion"), "")
	}

	sv, err := util.ParseKubernetesVersion(c.Spec.KubernetesVersion)
	if err != nil {
		return field.Invalid(fieldSpec.Child("KubernetesVersion"), c.Spec.KubernetesVersion, "unable to determine kubernetes version")
	}
	kubernetesRelease = semver.Version{Major: sv.Major, Minor: sv.Minor}

	if c.ObjectMeta.Name == "" {
		return field.Required(field.NewPath("Name"), "Cluster Name is required (e.g. --name=mycluster.myzone.com)")
	}

	{
		// Must be a dns name
		errs := validation.IsDNS1123Subdomain(c.ObjectMeta.Name)
		if len(errs) != 0 {
			return field.Invalid(field.NewPath("Name"), c.ObjectMeta.Name, fmt.Sprintf("Cluster Name must be a valid DNS name (e.g. --name=mycluster.myzone.com) errors: %s", strings.Join(errs, ", ")))
		}

		if !strings.Contains(c.ObjectMeta.Name, ".") {
			// Tolerate if this is a cluster we are importing for upgrade
			if c.ObjectMeta.Annotations[kops.AnnotationNameManagement] != kops.AnnotationValueManagementImported {
				return field.Invalid(field.NewPath("Name"), c.ObjectMeta.Name, "Cluster Name must be a fully-qualified DNS name (e.g. --name=mycluster.myzone.com)")
			}
		}
	}

	if c.Spec.CloudProvider == "" {
		return field.Required(fieldSpec.Child("CloudProvider"), "")
	}

	requiresSubnets := true
	requiresNetworkCIDR := true
	requiresSubnetCIDR := true
	switch kops.CloudProviderID(c.Spec.CloudProvider) {
	case kops.CloudProviderBareMetal:
		requiresSubnets = false
		requiresSubnetCIDR = false
		requiresNetworkCIDR = false
		if c.Spec.NetworkCIDR != "" {
			return field.Invalid(fieldSpec.Child("NetworkCIDR"), c.Spec.NetworkCIDR, "NetworkCIDR should not be set on bare metal")
		}

	case kops.CloudProviderGCE:
		requiresNetworkCIDR = false
		if c.Spec.NetworkCIDR != "" {
			return field.Invalid(fieldSpec.Child("NetworkCIDR"), c.Spec.NetworkCIDR, "NetworkCIDR should not be set on GCE")
		}
		requiresSubnetCIDR = false

	case kops.CloudProviderDO:
		requiresSubnets = false
		requiresSubnetCIDR = false
		requiresNetworkCIDR = false
		if c.Spec.NetworkCIDR != "" {
			return field.Invalid(fieldSpec.Child("NetworkCIDR"), c.Spec.NetworkCIDR, "NetworkCIDR should not be set on DigitalOcean")
		}
	case kops.CloudProviderAWS:
	case kops.CloudProviderVSphere:
	case kops.CloudProviderOpenstack:
		requiresNetworkCIDR = false
		requiresSubnetCIDR = false

	default:
		return field.Invalid(fieldSpec.Child("CloudProvider"), c.Spec.CloudProvider, "CloudProvider not recognized")
	}

	if requiresSubnets && len(c.Spec.Subnets) == 0 {
		// TODO: Auto choose zones from region?
		return field.Required(fieldSpec.Child("Subnets"), "must configure at least one Subnet (use --zones)")
	}

	if strict && c.Spec.Kubelet == nil {
		return field.Required(fieldSpec.Child("Kubelet"), "Kubelet not configured")
	}
	if strict && c.Spec.MasterKubelet == nil {
		return field.Required(fieldSpec.Child("MasterKubelet"), "MasterKubelet not configured")
	}
	if strict && c.Spec.KubeControllerManager == nil {
		return field.Required(fieldSpec.Child("KubeControllerManager"), "KubeControllerManager not configured")
	}
	if kubernetesRelease.LT(semver.MustParse("1.7.0")) && c.Spec.ExternalCloudControllerManager != nil {
		return field.Invalid(fieldSpec.Child("ExternalCloudControllerManager"), c.Spec.ExternalCloudControllerManager, "ExternalCloudControllerManager is not supported in version 1.6.0 or lower")
	}
	if strict && c.Spec.KubeDNS == nil {
		return field.Required(fieldSpec.Child("KubeDNS"), "KubeDNS not configured")
	}
	if strict && c.Spec.KubeScheduler == nil {
		return field.Required(fieldSpec.Child("KubeScheduler"), "KubeScheduler not configured")
	}
	if strict && c.Spec.KubeAPIServer == nil {
		return field.Required(fieldSpec.Child("KubeAPIServer"), "KubeAPIServer not configured")
	}
	if strict && c.Spec.KubeProxy == nil {
		return field.Required(fieldSpec.Child("KubeProxy"), "KubeProxy not configured")
	}
	if strict && c.Spec.Docker == nil {
		return field.Required(fieldSpec.Child("Docker"), "Docker not configured")
	}

	// Check NetworkCIDR
	var networkCIDR *net.IPNet
	{
		if c.Spec.NetworkCIDR == "" {
			if requiresNetworkCIDR {
				return field.Required(fieldSpec.Child("NetworkCIDR"), "Cluster did not have NetworkCIDR set")
			}
		} else {
			_, networkCIDR, err = net.ParseCIDR(c.Spec.NetworkCIDR)
			if err != nil {
				return field.Invalid(fieldSpec.Child("NetworkCIDR"), c.Spec.NetworkCIDR, fmt.Sprintf("Cluster had an invalid NetworkCIDR"))
			}
		}
	}

	// Check AdditionalNetworkCIDRs
	var additionalNetworkCIDRs []*net.IPNet
	{
		if len(c.Spec.AdditionalNetworkCIDRs) > 0 {
			for _, AdditionalNetworkCIDR := range c.Spec.AdditionalNetworkCIDRs {
				_, IPNetAdditionalNetworkCIDR, err := net.ParseCIDR(AdditionalNetworkCIDR)
				if err != nil {
					return field.Invalid(fieldSpec.Child("AdditionalNetworkCIDRs"), AdditionalNetworkCIDR, fmt.Sprintf("Cluster had an invalid AdditionalNetworkCIDRs"))
				}
				additionalNetworkCIDRs = append(additionalNetworkCIDRs, IPNetAdditionalNetworkCIDR)
			}
		}
	}

	// Check NonMasqueradeCIDR
	var nonMasqueradeCIDR *net.IPNet
	{
		nonMasqueradeCIDRString := c.Spec.NonMasqueradeCIDR
		if nonMasqueradeCIDRString == "" {
			return field.Required(fieldSpec.Child("NonMasqueradeCIDR"), "Cluster did not have NonMasqueradeCIDR set")
		}
		_, nonMasqueradeCIDR, err = net.ParseCIDR(nonMasqueradeCIDRString)
		if err != nil {
			return field.Invalid(fieldSpec.Child("NonMasqueradeCIDR"), nonMasqueradeCIDRString, "Cluster had an invalid NonMasqueradeCIDR")
		}

		if networkCIDR != nil && subnetsOverlap(nonMasqueradeCIDR, networkCIDR) && c.Spec.Networking != nil && c.Spec.Networking.AmazonVPC == nil {
			return field.Invalid(fieldSpec.Child("NonMasqueradeCIDR"), nonMasqueradeCIDRString, fmt.Sprintf("NonMasqueradeCIDR %q cannot overlap with NetworkCIDR %q", nonMasqueradeCIDRString, c.Spec.NetworkCIDR))
		}

		if c.Spec.Kubelet != nil && c.Spec.Kubelet.NonMasqueradeCIDR != nonMasqueradeCIDRString {
			if strict || c.Spec.Kubelet.NonMasqueradeCIDR != "" {
				return field.Invalid(fieldSpec.Child("NonMasqueradeCIDR"), nonMasqueradeCIDRString, "Kubelet NonMasqueradeCIDR did not match cluster NonMasqueradeCIDR")
			}
		}
		if c.Spec.MasterKubelet != nil && c.Spec.MasterKubelet.NonMasqueradeCIDR != nonMasqueradeCIDRString {
			if strict || c.Spec.MasterKubelet.NonMasqueradeCIDR != "" {
				return field.Invalid(fieldSpec.Child("NonMasqueradeCIDR"), nonMasqueradeCIDRString, "MasterKubelet NonMasqueradeCIDR did not match cluster NonMasqueradeCIDR")
			}
		}
	}

	// Check ServiceClusterIPRange
	var serviceClusterIPRange *net.IPNet
	{
		serviceClusterIPRangeString := c.Spec.ServiceClusterIPRange
		if serviceClusterIPRangeString == "" {
			if strict {
				return field.Required(fieldSpec.Child("ServiceClusterIPRange"), "Cluster did not have ServiceClusterIPRange set")
			}
		} else {
			_, serviceClusterIPRange, err = net.ParseCIDR(serviceClusterIPRangeString)
			if err != nil {
				return field.Invalid(fieldSpec.Child("ServiceClusterIPRange"), serviceClusterIPRangeString, "Cluster had an invalid ServiceClusterIPRange")
			}

			if !isSubnet(nonMasqueradeCIDR, serviceClusterIPRange) {
				return field.Invalid(fieldSpec.Child("ServiceClusterIPRange"), serviceClusterIPRangeString, fmt.Sprintf("ServiceClusterIPRange %q must be a subnet of NonMasqueradeCIDR %q", serviceClusterIPRangeString, c.Spec.NonMasqueradeCIDR))
			}

			if c.Spec.KubeAPIServer != nil && c.Spec.KubeAPIServer.ServiceClusterIPRange != serviceClusterIPRangeString {
				if strict || c.Spec.KubeAPIServer.ServiceClusterIPRange != "" {
					return field.Invalid(fieldSpec.Child("ServiceClusterIPRange"), serviceClusterIPRangeString, "KubeAPIServer ServiceClusterIPRange did not match cluster ServiceClusterIPRange")
				}
			}
		}
	}

	// Check Canal Networking Spec if used
	if c.Spec.Networking.Canal != nil {
		action := c.Spec.Networking.Canal.DefaultEndpointToHostAction
		switch action {
		case "", "ACCEPT", "DROP", "RETURN":
		default:
			return field.Invalid(fieldSpec.Child("Networking", "Canal", "DefaultEndpointToHostAction"), action, fmt.Sprintf("Unsupported value: %s, supports ACCEPT, DROP or RETURN", action))
		}

		chainInsertMode := c.Spec.Networking.Canal.ChainInsertMode
		switch chainInsertMode {
		case "", "insert", "append":
		default:
			return field.Invalid(fieldSpec.Child("Networking", "Canal", "ChainInsertMode"), action, fmt.Sprintf("Unsupported value: %s, supports 'insert' or 'append'", chainInsertMode))
		}
	}

	// Check ClusterCIDR
	if c.Spec.KubeControllerManager != nil {
		var clusterCIDR *net.IPNet
		clusterCIDRString := c.Spec.KubeControllerManager.ClusterCIDR
		if clusterCIDRString != "" {
			_, clusterCIDR, err = net.ParseCIDR(clusterCIDRString)
			if err != nil {
				return field.Invalid(fieldSpec.Child("KubeControllerManager", "ClusterCIDR"), clusterCIDRString, "Cluster had an invalid KubeControllerManager.ClusterCIDR")
			}

			if !isSubnet(nonMasqueradeCIDR, clusterCIDR) {
				return field.Invalid(fieldSpec.Child("KubeControllerManager", "ClusterCIDR"), clusterCIDRString, fmt.Sprintf("KubeControllerManager.ClusterCIDR %q must be a subnet of NonMasqueradeCIDR %q", clusterCIDRString, c.Spec.NonMasqueradeCIDR))
			}
		}
	}

	// Check KubeDNS.ServerIP
	if c.Spec.KubeDNS != nil {
		serverIPString := c.Spec.KubeDNS.ServerIP
		if serverIPString == "" {
			return field.Required(fieldSpec.Child("KubeDNS", "ServerIP"), "Cluster did not have KubeDNS.ServerIP set")
		}

		dnsServiceIP := net.ParseIP(serverIPString)
		if dnsServiceIP == nil {
			return field.Invalid(fieldSpec.Child("KubeDNS", "ServerIP"), serverIPString, "Cluster had an invalid KubeDNS.ServerIP")
		}

		if !serviceClusterIPRange.Contains(dnsServiceIP) {
			return field.Invalid(fieldSpec.Child("KubeDNS", "ServerIP"), serverIPString, fmt.Sprintf("ServiceClusterIPRange %q must contain the DNS Server IP %q", c.Spec.ServiceClusterIPRange, serverIPString))
		}

		if c.Spec.Kubelet != nil && c.Spec.Kubelet.ClusterDNS != c.Spec.KubeDNS.ServerIP {
			return field.Invalid(fieldSpec.Child("KubeDNS", "ServerIP"), serverIPString, "Kubelet ClusterDNS did not match cluster KubeDNS.ServerIP")
		}
		if c.Spec.MasterKubelet != nil && c.Spec.MasterKubelet.ClusterDNS != c.Spec.KubeDNS.ServerIP {
			return field.Invalid(fieldSpec.Child("KubeDNS", "ServerIP"), serverIPString, "MasterKubelet ClusterDNS did not match cluster KubeDNS.ServerIP")
		}
	}

	// Check CloudProvider
	{

		var k8sCloudProvider string
		switch kops.CloudProviderID(c.Spec.CloudProvider) {
		case kops.CloudProviderAWS:
			k8sCloudProvider = "aws"
		case kops.CloudProviderGCE:
			k8sCloudProvider = "gce"
		case kops.CloudProviderDO:
			k8sCloudProvider = "external"
		case kops.CloudProviderVSphere:
			k8sCloudProvider = "vsphere"
		case kops.CloudProviderBareMetal:
			k8sCloudProvider = ""
		case kops.CloudProviderOpenstack:
			k8sCloudProvider = "openstack"
		default:
			return field.Invalid(fieldSpec.Child("CloudProvider"), c.Spec.CloudProvider, "unknown cloudprovider")
		}

		if c.Spec.Kubelet != nil && (strict || c.Spec.Kubelet.CloudProvider != "") {
			if c.Spec.Kubelet.CloudProvider != "external" && k8sCloudProvider != c.Spec.Kubelet.CloudProvider {
				return field.Invalid(fieldSpec.Child("Kubelet", "CloudProvider"), c.Spec.Kubelet.CloudProvider, "Did not match cluster CloudProvider")
			}
		}
		if c.Spec.MasterKubelet != nil && (strict || c.Spec.MasterKubelet.CloudProvider != "") {
			if c.Spec.MasterKubelet.CloudProvider != "external" && k8sCloudProvider != c.Spec.MasterKubelet.CloudProvider {
				return field.Invalid(fieldSpec.Child("MasterKubelet", "CloudProvider"), c.Spec.MasterKubelet.CloudProvider, "Did not match cluster CloudProvider")

			}
		}
		if c.Spec.KubeAPIServer != nil && (strict || c.Spec.KubeAPIServer.CloudProvider != "") {
			if c.Spec.KubeAPIServer.CloudProvider != "external" && k8sCloudProvider != c.Spec.KubeAPIServer.CloudProvider {
				return field.Invalid(fieldSpec.Child("KubeAPIServer", "CloudProvider"), c.Spec.KubeAPIServer.CloudProvider, "Did not match cluster CloudProvider")
			}
		}
		if c.Spec.KubeControllerManager != nil && (strict || c.Spec.KubeControllerManager.CloudProvider != "") {
			if c.Spec.KubeControllerManager.CloudProvider != "external" && k8sCloudProvider != c.Spec.KubeControllerManager.CloudProvider {
				return field.Invalid(fieldSpec.Child("KubeControllerManager", "CloudProvider"), c.Spec.KubeControllerManager.CloudProvider, "Did not match cluster CloudProvider")
			}
		}
	}

	// Check that the subnet CIDRs are all consistent
	{
		for i, s := range c.Spec.Subnets {
			fieldSubnet := fieldSpec.Child("Subnets").Index(i)
			if s.CIDR == "" {
				if requiresSubnetCIDR && strict {
					return field.Required(fieldSubnet.Child("CIDR"), "Subnet did not have a CIDR set")
				}
			} else {
				_, subnetCIDR, err := net.ParseCIDR(s.CIDR)
				if err != nil {
					return field.Invalid(fieldSubnet.Child("CIDR"), s.CIDR, "Subnet had an invalid CIDR")
				}

				if networkCIDR != nil && !validateSubnetCIDR(networkCIDR, additionalNetworkCIDRs, subnetCIDR) {
					return field.Invalid(fieldSubnet.Child("CIDR"), s.CIDR, fmt.Sprintf("Subnet %q had a CIDR %q that was not a subnet of the NetworkCIDR %q", s.Name, s.CIDR, c.Spec.NetworkCIDR))
				}
			}
		}
	}

	// UpdatePolicy
	if c.Spec.UpdatePolicy != nil {
		switch *c.Spec.UpdatePolicy {
		case kops.UpdatePolicyExternal:
		// Valid
		default:
			return field.Invalid(fieldSpec.Child("UpdatePolicy"), *c.Spec.UpdatePolicy, "unrecognized value for UpdatePolicy")
		}
	}

	// KubeProxy
	if c.Spec.KubeProxy != nil {
		kubeProxyPath := fieldSpec.Child("KubeProxy")

		master := c.Spec.KubeProxy.Master
		// We no longer require the master to be set; nodeup can infer it automatically
		//if strict && master == "" {
		//      return field.Required(kubeProxyPath.Child("Master"), "")
		//}

		if master != "" && !isValidAPIServersURL(master) {
			return field.Invalid(kubeProxyPath.Child("Master"), master, "Not a valid APIServer URL")
		}
	}

	// Kubelet
	if c.Spec.Kubelet != nil {
		kubeletPath := fieldSpec.Child("Kubelet")

		if kubernetesRelease.GTE(semver.MustParse("1.6.0")) {
			// Flag removed in 1.6
			if c.Spec.Kubelet.APIServers != "" {
				return field.Invalid(
					kubeletPath.Child("APIServers"),
					c.Spec.Kubelet.APIServers,
					"api-servers flag was removed in 1.6")
			}
		} else {
			if strict && c.Spec.Kubelet.APIServers == "" {
				return field.Required(kubeletPath.Child("APIServers"), "")
			}
		}

		if c.Spec.Kubelet.APIServers != "" && !isValidAPIServersURL(c.Spec.Kubelet.APIServers) {
			return field.Invalid(kubeletPath.Child("APIServers"), c.Spec.Kubelet.APIServers, "Not a valid APIServer URL")
		}
	}

	// MasterKubelet
	if c.Spec.MasterKubelet != nil {
		masterKubeletPath := fieldSpec.Child("MasterKubelet")

		if kubernetesRelease.GTE(semver.MustParse("1.6.0")) {
			// Flag removed in 1.6
			if c.Spec.MasterKubelet.APIServers != "" {
				return field.Invalid(
					masterKubeletPath.Child("APIServers"),
					c.Spec.MasterKubelet.APIServers,
					"api-servers flag was removed in 1.6")
			}
		} else {
			if strict && c.Spec.MasterKubelet.APIServers == "" {
				return field.Required(masterKubeletPath.Child("APIServers"), "")
			}
		}

		if c.Spec.MasterKubelet.APIServers != "" && !isValidAPIServersURL(c.Spec.MasterKubelet.APIServers) {
			return field.Invalid(masterKubeletPath.Child("APIServers"), c.Spec.MasterKubelet.APIServers, "Not a valid APIServer URL")
		}
	}

	// Topology support
	if c.Spec.Topology != nil {
		if c.Spec.Topology.Masters != "" && c.Spec.Topology.Nodes != "" {
			if c.Spec.Topology.Masters != kops.TopologyPublic && c.Spec.Topology.Masters != kops.TopologyPrivate {
				return field.Invalid(fieldSpec.Child("Topology", "Masters"), c.Spec.Topology.Masters, "Invalid Masters value for Topology")
			} else if c.Spec.Topology.Nodes != kops.TopologyPublic && c.Spec.Topology.Nodes != kops.TopologyPrivate {
				return field.Invalid(fieldSpec.Child("Topology", "Nodes"), c.Spec.Topology.Nodes, "Invalid Nodes value for Topology")
			}

		} else {
			return field.Required(fieldSpec.Child("Masters"), "Topology requires non-nil values for Masters and Nodes")
		}
		if c.Spec.Topology.Bastion != nil {
			bastion := c.Spec.Topology.Bastion
			if c.Spec.Topology.Masters == kops.TopologyPublic || c.Spec.Topology.Nodes == kops.TopologyPublic {
				return field.Invalid(fieldSpec.Child("Topology", "Masters"), c.Spec.Topology.Masters, "Bastion supports only Private Masters and Nodes")
			}
			if bastion.IdleTimeoutSeconds != nil && *bastion.IdleTimeoutSeconds <= 0 {
				return field.Invalid(fieldSpec.Child("Topology", "Bastion", "IdleTimeoutSeconds"), *bastion.IdleTimeoutSeconds, "Bastion IdleTimeoutSeconds should be greater than zero")
			}
			if bastion.IdleTimeoutSeconds != nil && *bastion.IdleTimeoutSeconds > 3600 {
				return field.Invalid(fieldSpec.Child("Topology", "Bastion", "IdleTimeoutSeconds"), *bastion.IdleTimeoutSeconds, "Bastion IdleTimeoutSeconds cannot be greater than one hour")
			}

		}
	}
	// Egress specification support
	{
		for i, s := range c.Spec.Subnets {
			fieldSubnet := fieldSpec.Child("Subnets").Index(i)
			if s.Egress != "" && !strings.HasPrefix(s.Egress, "nat-") {
				return field.Invalid(fieldSubnet.Child("Egress"), s.Egress, "egress must be of type NAT Gateway")
			}
			if s.Egress != "" && !(s.Type == "Private") {
				return field.Invalid(fieldSubnet.Child("Egress"), s.Egress, "egress can only be specified for Private subnets")
			}
		}
	}

	// Etcd
	{
		fieldEtcdClusters := fieldSpec.Child("EtcdClusters")

		if len(c.Spec.EtcdClusters) == 0 {
			return field.Required(fieldEtcdClusters, "")
		}
		for i, x := range c.Spec.EtcdClusters {
			if err := validateEtcdClusterSpec(x, fieldEtcdClusters.Index(i)); err != nil {
				return err
			}
		}
		if err := validateEtcdTLS(c.Spec.EtcdClusters, fieldEtcdClusters); err != nil {
			return err
		}
		if err := validateEtcdStorage(c.Spec.EtcdClusters, fieldEtcdClusters); err != nil {
			return err
		}
	}

	if kubernetesRelease.GTE(semver.MustParse("1.4.0")) {
		if c.Spec.Networking != nil && c.Spec.Networking.Classic != nil {
			return field.Invalid(fieldSpec.Child("Networking"), "classic", "classic networking is not supported with kubernetes versions 1.4 and later")
		}
	}

	if c.Spec.Networking != nil && c.Spec.Networking.AmazonVPC != nil &&
		c.Spec.Kubelet != nil && (c.Spec.Kubelet.CloudProvider != "aws") {
		return field.Invalid(fieldSpec.Child("Networking"), "amazon-vpc-routed-eni", "amazon-vpc-routed-eni networking is supported only in AWS")
	}

	if kubernetesRelease.LT(semver.MustParse("1.7.0")) {
		if c.Spec.Networking != nil && c.Spec.Networking.Romana != nil {
			return field.Invalid(fieldSpec.Child("Networking"), "romana", "romana networking is not supported with kubernetes versions 1.6 or lower")
		}

		if c.Spec.Networking != nil && c.Spec.Networking.AmazonVPC != nil {
			return field.Invalid(fieldSpec.Child("Networking"), "amazon-vpc-routed-eni", "amazon-vpc-routed-eni networking is not supported with kubernetes versions 1.6 or lower")
		}
	}

	if errs := newValidateCluster(c); len(errs) != 0 {
		return errs[0]
	}

	return nil
}

// validateSubnetCIDR is responsible for validating subnets are part of the CIRDs assigned to the cluster.
func validateSubnetCIDR(networkCIDR *net.IPNet, additionalNetworkCIDRs []*net.IPNet, subnetCIDR *net.IPNet) bool {
	if isSubnet(networkCIDR, subnetCIDR) {
		return true
	}

	for _, additionalNetworkCIDR := range additionalNetworkCIDRs {
		if isSubnet(additionalNetworkCIDR, subnetCIDR) {
			return true
		}
	}

	return false
}

// validateEtcdClusterSpec is responsible for validating the etcd cluster spec
func validateEtcdClusterSpec(spec *kops.EtcdClusterSpec, fieldPath *field.Path) *field.Error {
	if spec.Name == "" {
		return field.Required(fieldPath.Child("Name"), "EtcdCluster did not have name")
	}
	if len(spec.Members) == 0 {
		return field.Required(fieldPath.Child("Members"), "No members defined in etcd cluster")
	}
	if (len(spec.Members) % 2) == 0 {
		// Not technically a requirement, but doesn't really make sense to allow
		return field.Invalid(fieldPath.Child("Members"), len(spec.Members), "Should be an odd number of master-zones for quorum. Use --zones and --master-zones to declare node zones and master zones separately")
	}
	if err := validateEtcdVersion(spec, fieldPath); err != nil {
		return err
	}
	for _, m := range spec.Members {
		if err := validateEtcdMemberSpec(m, fieldPath); err != nil {
			return err
		}
	}

	return nil
}

// validateEtcdTLS checks the TLS settings for etcd are valid
func validateEtcdTLS(specs []*kops.EtcdClusterSpec, fieldPath *field.Path) *field.Error {
	var usingTLS int
	for _, x := range specs {
		if x.EnableEtcdTLS {
			usingTLS++
		}
	}
	// check both clusters are using tls if one us enabled
	if usingTLS > 0 && usingTLS != len(specs) {
		return field.Invalid(fieldPath.Index(0).Child("EnableEtcdTLS"), false, "Both etcd clusters must have TLS enabled or none at all")
	}

	return nil
}

// validateEtcdStorage is responsible for checks version are identical
func validateEtcdStorage(specs []*kops.EtcdClusterSpec, fieldPath *field.Path) *field.Error {
	version := specs[0].Version
	for i, x := range specs {
		if x.Version != "" && x.Version != version {
			return field.Invalid(fieldPath.Index(i).Child("Version"), x.Version, fmt.Sprintf("cluster: %q, has a different storage versions: %q, both must be the same", x.Name, x.Version))
		}
	}

	return nil
}

// validateEtcdVersion is responsible for validating the storage version of etcd
// @TODO semvar package doesn't appear to ignore a 'v' in v1.1.1 should could be a problem later down the line
func validateEtcdVersion(spec *kops.EtcdClusterSpec, fieldPath *field.Path) *field.Error {
	// @check if the storage is specified, thats is valid
	if spec.Version == "" {
		return nil // as it will be filled in by default for us
	}

	sem, err := semver.Parse(strings.TrimPrefix(spec.Version, "v"))
	if err != nil {
		return field.Invalid(fieldPath.Child("Version"), spec.Version, "the storage version is invalid")
	}

	// we only support v3 and v2 for now
	if sem.Major == 3 || sem.Major == 2 {
		return nil
	}

	return field.Invalid(fieldPath.Child("Version"), spec.Version, "unsupported storage version, we only support major versions 2 and 3")
}

// validateEtcdMemberSpec is responsible for validate the cluster member
func validateEtcdMemberSpec(spec *kops.EtcdMemberSpec, fieldPath *field.Path) *field.Error {
	if spec.Name == "" {
		return field.Required(fieldPath.Child("Name"), "EtcdMember did not have Name")
	}

	if fi.StringValue(spec.InstanceGroup) == "" {
		return field.Required(fieldPath.Child("InstanceGroup"), "EtcdMember did not have InstanceGroup")
	}

	return nil
}

func DeepValidate(c *kops.Cluster, groups []*kops.InstanceGroup, strict bool) error {
	if err := ValidateCluster(c, strict); err != nil {
		return err
	}

	if len(groups) == 0 {
		return fmt.Errorf("must configure at least one InstanceGroup")
	}

	masterGroupCount := 0
	nodeGroupCount := 0
	for _, g := range groups {
		if g.IsMaster() {
			masterGroupCount++
		} else {
			nodeGroupCount++
		}
	}

	if masterGroupCount == 0 {
		return fmt.Errorf("must configure at least one Master InstanceGroup")
	}

	if nodeGroupCount == 0 {
		return fmt.Errorf("must configure at least one Node InstanceGroup")
	}

	for _, g := range groups {
		err := CrossValidateInstanceGroup(g, c, strict)
		if err != nil {
			return err
		}

		// Additional cloud-specific validation rules,
		// such as making sure that identifiers match the expected formats for the given cloud
		switch kops.CloudProviderID(c.Spec.CloudProvider) {
		case kops.CloudProviderAWS:
			errs := awsValidateInstanceGroup(g)
			if len(errs) != 0 {
				return errs[0]
			}
		}
	}

	return nil
}
