/*
Copyright 2018 The Kubernetes Authors.

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

package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"k8s.io/kops/cmd/kops/util"
	api "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/featureflag"
)

type SetClusterOptions struct {
	Fields      []string
	ClusterName string
}

// RunSetCluster implements the set cluster command logic
func RunSetCluster(f *util.Factory, cmd *cobra.Command, out io.Writer, options *SetClusterOptions) error {
	if !featureflag.SpecOverrideFlag.Enabled() {
		return fmt.Errorf("set cluster command is current feature gated; set `export KOPS_FEATURE_FLAGS=SpecOverrideFlag`")
	}

	if options.ClusterName == "" {
		return field.Required(field.NewPath("ClusterName"), "Cluster name is required")
	}

	clientset, err := f.Clientset()
	if err != nil {
		return err
	}

	cluster, err := clientset.GetCluster(options.ClusterName)
	if err != nil {
		return err
	}

	instanceGroups, err := ReadAllInstanceGroups(clientset, cluster)
	if err != nil {
		return err
	}

	if err := setClusterFields(options.Fields, cluster); err != nil {
		return err
	}

	if err := UpdateCluster(clientset, cluster, instanceGroups); err != nil {
		return err
	}

	return nil
}

// setClusterFields sets field values in the cluster
func setClusterFields(fields []string, cluster *api.Cluster) error {
	for _, field := range fields {
		kv := strings.SplitN(field, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("unhandled field: %q", field)
		}

		// For now we have hard-code the values we want to support; we'll get test coverage and then do this properly...
		switch kv[0] {
		case "spec.kubernetesVersion":
			cluster.Spec.KubernetesVersion = kv[1]
		default:
			return fmt.Errorf("unhandled field: %q", field)
		}
	}
	return nil
}
