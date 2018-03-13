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

// Code generated by ""fitask" -type=ManagedFile"; DO NOT EDIT

package fitasks

import (
	"encoding/json"

	"k8s.io/kops/upup/pkg/fi"
)

// ManagedFile

// JSON marshalling boilerplate
type realManagedFile ManagedFile

// UnmarshalJSON implements conversion to JSON, supporitng an alternate specification of the object as a string
func (o *ManagedFile) UnmarshalJSON(data []byte) error {
	var jsonName string
	if err := json.Unmarshal(data, &jsonName); err == nil {
		o.Name = &jsonName
		return nil
	}

	var r realManagedFile
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*o = ManagedFile(r)
	return nil
}

var _ fi.HasLifecycle = &ManagedFile{}

// GetLifecycle returns the Lifecycle of the object, implementing fi.HasLifecycle
func (o *ManagedFile) GetLifecycle() *fi.Lifecycle {
	return o.Lifecycle
}

// SetLifecycle sets the Lifecycle of the object, implementing fi.SetLifecycle
func (o *ManagedFile) SetLifecycle(lifecycle fi.Lifecycle) {
	o.Lifecycle = &lifecycle
}

var _ fi.HasName = &ManagedFile{}

// GetName returns the Name of the object, implementing fi.HasName
func (o *ManagedFile) GetName() *string {
	return o.Name
}

// SetName sets the Name of the object, implementing fi.SetName
func (o *ManagedFile) SetName(name string) {
	o.Name = &name
}

// String is the stringer function for the task, producing readable output using fi.TaskAsString
func (o *ManagedFile) String() string {
	return fi.TaskAsString(o)
}
