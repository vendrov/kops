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

package components

import (
	"fmt"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi/loader"
)

const DefaultBackupImage = "kopeio/etcd-backup:1.0.20180220"

// EtcdOptionsBuilder adds options for etcd to the model
type EtcdOptionsBuilder struct {
	Context *OptionsContext
}

var _ loader.OptionsBuilder = &EtcdOptionsBuilder{}

// BuildOptions is responsible for filling in the defaults for the etcd cluster model
func (b *EtcdOptionsBuilder) BuildOptions(o interface{}) error {
	spec := o.(*kops.ClusterSpec)

	// @check the version are set and if not preset the defaults
	for _, x := range spec.EtcdClusters {
		// @TODO if nothing is set, set the defaults. At a late date once we have a way of detecting a 'new' cluster
		// we can default all clusters to v3
		if x.Version == "" {
			x.Version = "2.2.1"
		}
	}

	// remap image
	for _, c := range spec.EtcdClusters {
		image := c.Image
		if image == "" {
			image = fmt.Sprintf("k8s.gcr.io/etcd:%s", c.Version)
		}

		image, err := b.Context.AssetBuilder.RemapImage(image)
		if err != nil {
			return fmt.Errorf("unable to remap container %q: %v", image, err)
		}
		c.Image = image
	}

	// remap backup manager images
	for _, c := range spec.EtcdClusters {
		if c.Backups == nil {
			continue
		}
		image := c.Backups.Image
		if image == "" {
			image = fmt.Sprintf(DefaultBackupImage)
		}

		image, err := b.Context.AssetBuilder.RemapImage(image)
		if err != nil {
			return fmt.Errorf("unable to remap container %q: %v", image, err)
		}
		c.Backups.Image = image
	}

	return nil
}
