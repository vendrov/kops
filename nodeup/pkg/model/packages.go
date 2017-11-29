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
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/nodeup/nodetasks"
)

// PackagesBuilder adds miscellaneous OS packages that we need
type PackagesBuilder struct {
	*NodeupModelContext
}

var _ fi.ModelBuilder = &DockerBuilder{}

func (b *PackagesBuilder) Build(c *fi.ModelBuilderContext) error {
	// kubelet needs:
	//   ebtables - kops #1711
	//   ethtool - kops #1830
	if b.Distribution.IsDebianFamily() {
		c.AddTask(&nodetasks.Package{Name: "ebtables"})
		c.AddTask(&nodetasks.Package{Name: "ethtool"})
	} else if b.Distribution.IsRHELFamily() {
		c.AddTask(&nodetasks.Package{Name: "ebtables"})
		c.AddTask(&nodetasks.Package{Name: "ethtool"})
		c.AddTask(&nodetasks.Package{Name: "socat"})
	} else {
		// Hopefully it's already installed
		glog.Infof("ebtables package not known for distro %q", b.Distribution)
	}

	return nil
}
