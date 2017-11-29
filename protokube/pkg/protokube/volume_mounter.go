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

package protokube

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/util/mount"
)

type VolumeMountController struct {
	mounted map[string]*Volume

	provider Volumes
}

func newVolumeMountController(provider Volumes) *VolumeMountController {
	c := &VolumeMountController{}
	c.mounted = make(map[string]*Volume)
	c.provider = provider
	return c
}

func (k *VolumeMountController) mountMasterVolumes() ([]*Volume, error) {
	// TODO: mount ephemeral volumes (particular on AWS)?

	// Mount master volumes
	attached, err := k.attachMasterVolumes()
	if err != nil {
		return nil, fmt.Errorf("unable to attach master volumes: %v", err)
	}

	for _, v := range attached {
		existing := k.mounted[v.ID]
		if existing != nil {
			continue
		}

		glog.V(2).Infof("Master volume %q is attached at %q", v.ID, v.LocalDevice)

		mountpoint := "/mnt/master-" + v.ID

		// On ContainerOS, we mount to /mnt/disks instead (/mnt is readonly)
		_, err := os.Stat(pathFor("/mnt/disks"))
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error checking for /mnt/disks: %v", err)
			}
		} else {
			mountpoint = "/mnt/disks/master-" + v.ID
		}

		glog.Infof("Doing safe-format-and-mount of %s to %s", v.LocalDevice, mountpoint)
		fstype := ""
		err = k.safeFormatAndMount(v.LocalDevice, mountpoint, fstype)
		if err != nil {
			glog.Warningf("unable to mount master volume: %q", err)
			continue
		}

		glog.Infof("mounted master volume %q on %s", v.ID, mountpoint)

		v.Mountpoint = mountpoint
		k.mounted[v.ID] = v
	}

	var volumes []*Volume
	for _, v := range k.mounted {
		volumes = append(volumes, v)
	}
	return volumes, nil
}

func (k *VolumeMountController) safeFormatAndMount(device string, mountpoint string, fstype string) error {
	// Wait for the device to show up
	for {
		_, err := os.Stat(pathFor(device))
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("error checking for device %q: %v", device, err)
		}
		glog.Infof("Waiting for device %q to be attached", device)
		time.Sleep(1 * time.Second)
	}
	glog.Infof("Found device %q", device)

	safeFormatAndMount := &mount.SafeFormatAndMount{}

	if Containerized {
		// Build mount & exec implementations that execute in the host namespaces
		safeFormatAndMount.Interface = mount.NewNsenterMounter()
		safeFormatAndMount.Exec = NewNsEnterExec()

		// Note that we don't use pathFor for operations going through safeFormatAndMount,
		// because NewNsenterMounter and NewNsEnterExec will operate in the host
	} else {
		safeFormatAndMount.Interface = mount.New("")
		safeFormatAndMount.Exec = mount.NewOsExec()
	}

	// Check if it is already mounted
	// TODO: can we now use IsLikelyNotMountPoint or IsMountPointMatch instead here
	mounts, err := safeFormatAndMount.List()
	if err != nil {
		return fmt.Errorf("error listing existing mounts: %v", err)
	}

	var existing []*mount.MountPoint
	for i := range mounts {
		m := &mounts[i]
		glog.V(8).Infof("found existing mount: %v", m)
		// Note: when containerized, we still list mounts in the host, so we don't need to call pathFor(mountpoint)
		if m.Path == mountpoint {
			existing = append(existing, m)
		}
	}

	// Mount only if isn't mounted already
	if len(existing) == 0 {
		options := []string{}

		glog.Infof("Creating mount directory %q", pathFor(mountpoint))
		if err := os.MkdirAll(pathFor(mountpoint), 0750); err != nil {
			return err
		}

		glog.Infof("Mounting device %q on %q", device, mountpoint)

		err = safeFormatAndMount.FormatAndMount(device, mountpoint, fstype, options)
		if err != nil {
			return fmt.Errorf("error formatting and mounting disk %q on %q: %v", device, mountpoint, err)
		}
	} else {
		glog.Infof("Device already mounted on %q, verifying it is our device", mountpoint)

		if len(existing) != 1 {
			glog.Infof("Existing mounts unexpected")

			for i := range mounts {
				m := &mounts[i]
				glog.Infof("%s\t%s", m.Device, m.Path)
			}

			return fmt.Errorf("found multiple existing mounts of %q at %q", device, mountpoint)
		} else {
			glog.Infof("Found existing mount of %q at %q", device, mountpoint)
		}
	}

	// If we're containerized we also want to mount the device (again) into our container
	// We could also do this with mount propagation, but this is simple
	if Containerized {
		source := pathFor(device)
		target := pathFor(mountpoint)
		options := []string{}

		mounter := mount.New("")

		mountedDevice, _, err := mount.GetDeviceNameFromMount(mounter, target)
		if err != nil {
			return fmt.Errorf("error checking for mounts of %s inside container: %v", target, err)
		}

		if mountedDevice != "" {
			if mountedDevice != source {
				return fmt.Errorf("device already mounted at %s, but is %s and we want %s", target, mountedDevice, source)
			}
		} else {
			glog.Infof("mounting inside container: %s -> %s", source, target)
			if err := mounter.Mount(source, target, fstype, options); err != nil {
				return fmt.Errorf("error mounting %s inside container at %s: %v", source, target, err)
			}
		}
	}

	return nil
}

func (k *VolumeMountController) attachMasterVolumes() ([]*Volume, error) {
	volumes, err := k.provider.FindVolumes()
	if err != nil {
		return nil, err
	}

	var tryAttach []*Volume
	var attached []*Volume
	for _, v := range volumes {
		if doNotMountVolume(v) {
			continue
		}

		if v.AttachedTo == "" {
			tryAttach = append(tryAttach, v)
		}
		if v.LocalDevice != "" {
			attached = append(attached, v)
		}
	}

	if len(tryAttach) == 0 {
		return attached, nil
	}

	// Make sure we don't try to mount multiple volumes from the same cluster
	attachedClusters := sets.NewString()
	for _, attached := range attached {
		for _, etcdCluster := range attached.Info.EtcdClusters {
			attachedClusters.Insert(etcdCluster.ClusterKey)
		}
	}

	// Mount in a consistent order
	sort.Stable(ByEtcdClusterName(tryAttach))

	// Actually attempt the mounting
	for _, v := range tryAttach {
		alreadyMounted := ""
		for _, etcdCluster := range v.Info.EtcdClusters {
			if attachedClusters.Has(etcdCluster.ClusterKey) {
				alreadyMounted = etcdCluster.ClusterKey
			}
		}

		if alreadyMounted != "" {
			glog.V(2).Infof("Skipping mount of master volume %q, because etcd cluster %q is already mounted", v.ID, alreadyMounted)
			continue
		}

		glog.V(2).Infof("Trying to mount master volume: %q", v.ID)

		err := k.provider.AttachVolume(v)
		if err != nil {
			// We are racing with other instances here; this can happen
			glog.Warningf("Error attaching volume %q: %v", v.ID, err)
		} else {
			if v.LocalDevice == "" {
				glog.Fatalf("AttachVolume did not set LocalDevice")
			}
			attached = append(attached, v)

			// Mark this cluster as attached now
			for _, etcdCluster := range v.Info.EtcdClusters {
				attachedClusters.Insert(etcdCluster.ClusterKey)
			}
		}
	}

	glog.V(2).Infof("Currently attached volumes: %v", attached)
	return attached, nil
}

// doNotMountVolume tests that the volume has an Etcd Cluster associated
func doNotMountVolume(v *Volume) bool {
	if len(v.Info.EtcdClusters) == 0 {
		glog.Warningf("Local device: %q, volume id: %q is being skipped and will not mounted, since it does not have a etcd cluster", v.LocalDevice, v.ID)
		return true
	}
	return false
}

// ByEtcdClusterName sorts volumes so that we mount in a consistent order,
// and in addition we try to mount the main etcd volume before the events etcd volume
type ByEtcdClusterName []*Volume

func (a ByEtcdClusterName) Len() int {
	return len(a)
}
func (a ByEtcdClusterName) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a ByEtcdClusterName) Less(i, j int) bool {
	nameI := ""
	if len(a[i].Info.EtcdClusters) > 0 {
		nameI = a[i].Info.EtcdClusters[0].ClusterKey
	}
	nameJ := ""
	if len(a[j].Info.EtcdClusters) > 0 {
		nameJ = a[j].Info.EtcdClusters[0].ClusterKey
	}
	// reverse so "main" comes before "events"
	return nameI > nameJ
}
