/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright The KubeVirt Authors.
 *
 */

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"kubevirt.io/kubevirt/pkg/util"

	v1 "kubevirt.io/api/core/v1"
)

// Type represents allowed config types like ConfigMap or Secret.
type Type string

const (
	// ConfigMap respresents a configmap type,
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/
	ConfigMap Type = "configmap"
	// Secret represents a secret type,
	// https://kubernetes.io/docs/concepts/configuration/secret/
	Secret Type = "secret"
	// DownwardAPI represents a DownwardAPI type,
	// https://kubernetes.io/docs/tasks/inject-data-application/downward-api-volume-expose-pod-information/
	DownwardAPI Type = "downwardapi"
	// ServiceAccount represents a secret type,
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	ServiceAccount Type = "serviceaccount"

	mountBaseDir = "/var/run/kubevirt-private"
)

var (
	// ConfigMapSourceDir represents a location where ConfigMap is attached to the pod
	ConfigMapSourceDir = filepath.Join(mountBaseDir, "config-map")
	// SysprepSourceDir represents a location where a Sysprep is attached to the pod
	SysprepSourceDir = filepath.Join(mountBaseDir, "sysprep")
	// SecretSourceDir represents a location where Secrets is attached to the pod
	SecretSourceDir = filepath.Join(mountBaseDir, "secret")
	// DownwardAPISourceDir represents a location where downwardapi is attached to the pod
	DownwardAPISourceDir = filepath.Join(mountBaseDir, "downwardapi")
	// ServiceAccountSourceDir represents the location where the ServiceAccount token is attached to the pod
	ServiceAccountSourceDir = "/var/run/secrets/kubernetes.io/serviceaccount/"

	// ConfigMapDisksDir represents a path to ConfigMap iso images
	ConfigMapDisksDir = filepath.Join(mountBaseDir, "config-map-disks")
	// SecretDisksDir represents a path to Secrets iso images
	SecretDisksDir = filepath.Join(mountBaseDir, "secret-disks")
	// SysprepDisksDir represents a path to Syspreps iso images
	SysprepDisksDir = filepath.Join(mountBaseDir, "sysprep-disks")
	// DownwardAPIDisksDir represents a path to DownwardAPI iso images
	DownwardAPIDisksDir = filepath.Join(mountBaseDir, "downwardapi-disks")
	// DownwardMetricDisksDir represents a path to DownwardMetric block disk
	DownwardMetricDisksDir = filepath.Join(mountBaseDir, "downwardmetric-disk")
	// DownwardMetricDisks represents the disk location for the DownwardMetric disk
	DownwardMetricDisk = filepath.Join(DownwardAPIDisksDir, "vhostmd0")
	// ServiceAccountDiskDir represents a path to the ServiceAccount iso image
	ServiceAccountDiskDir = filepath.Join(mountBaseDir, "service-account-disk")
	// ServiceAccountDiskName represents the name of the ServiceAccount iso image
	ServiceAccountDiskName = "service-account.iso"
)

type isoPlan struct {
	output      string
	layout      ISOLayout
	placeholder bool
	size        int64
}

// ISOLayout describes the volume label and graft points of a config ISO.
type ISOLayout struct {
	Label       string
	GraftPoints []string
}

// BuildISOLayout maps mounted directory entries to ISO graft points without
// reading or writing any files.
func BuildISOLayout(label, source string, entries []os.DirEntry) ISOLayout {
	if label == "" {
		label = "cfgdata"
	}
	layout := ISOLayout{Label: label}
	for _, entry := range entries {
		name := entry.Name()
		layout.GraftPoints = append(layout.GraftPoints, name+"="+filepath.Join(source, name))
	}
	return layout
}

func (plan isoPlan) write() error {
	// Use a unique staging directory beside the destination so concurrent
	// writers can publish complete images with an atomic rename.
	dir, err := os.MkdirTemp(filepath.Dir(plan.output), ".iso-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	staging := filepath.Join(dir, filepath.Base(plan.output))
	if plan.placeholder {
		file, err := os.Create(staging)
		if err != nil {
			return err
		}
		if err := errors.Join(util.WriteBytes(file, 0, plan.size), file.Close()); err != nil {
			return err
		}
	} else {
		args := []string{
			"-output", staging, "-follow-links", "-volid", plan.layout.Label,
			"-joliet", "-rock", "-graft-points", "-partition_cyl_align", "on",
		}
		args = append(args, plan.layout.GraftPoints...)
		// #nosec Graft points are derived from the mounted volume entries.
		if err := exec.Command("xorrisofs", args...).Run(); err != nil {
			return err
		}
	}
	if err := chownToQEMU(staging); err != nil {
		return err
	}
	return os.Rename(staging, plan.output)
}

// chownToQEMU sets ISO ownership when virt-launcher runs as root.
func chownToQEMU(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return os.Chown(path, util.NonRootUID, util.NonRootUID)
}

func planPlaceholder(vmi *v1.VirtualMachineInstance, volumeName, output string) (isoPlan, error) {
	for _, status := range vmi.Status.VolumeStatus {
		if status.Name == volumeName {
			if status.Size < 0 {
				return isoPlan{}, fmt.Errorf("invalid ISO size %d for volume %s", status.Size, volumeName)
			}
			return isoPlan{output: output, placeholder: true, size: status.Size}, nil
		}
	}
	return isoPlan{}, fmt.Errorf("failed to find the status of volume %s", volumeName)
}

type volumeInfo interface {
	isValidType(*v1.Volume) bool
	getSourcePath(*v1.Volume) string
	getIsoPath(*v1.Volume) string
	getLabel(*v1.Volume) string
}

func configVolumesForDisks(vmi *v1.VirtualMachineInstance, info volumeInfo) []v1.Volume {
	volumes := make(map[string]v1.Volume)
	for _, volume := range vmi.Spec.Volumes {
		if info.isValidType(&volume) {
			volumes[volume.Name] = volume
		}
	}
	var selected []v1.Volume
	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if volume, ok := volumes[disk.Name]; ok {
			selected = append(selected, volume)
		}
	}
	return selected
}

func planConfigISO(vmi *v1.VirtualMachineInstance, volume *v1.Volume, emptyIso bool, info volumeInfo) (isoPlan, error) {
	output := info.getIsoPath(volume)
	if emptyIso {
		return planPlaceholder(vmi, volume.Name, output)
	}
	source := info.getSourcePath(volume)
	entries, err := os.ReadDir(source)
	if err != nil {
		return isoPlan{}, err
	}
	return isoPlan{output: output, layout: BuildISOLayout(info.getLabel(volume), source, entries)}, nil
}

func createIsoDisksForConfigVolumes(vmi *v1.VirtualMachineInstance, emptyIso bool, info volumeInfo) error {
	for _, volume := range configVolumesForDisks(vmi, info) {
		plan, err := planConfigISO(vmi, &volume, emptyIso, info)
		if err != nil {
			return err
		}
		if err := plan.write(); err != nil {
			return err
		}
	}
	return nil
}
