/*
 * This file is part of the kubevirt project
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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "kubevirt.io/api/core/v1"
)

// Assuming windows does not care what's the exact label.
const sysprepVolumeLabel = "unattendCD"

// GetSysprepSourcePath returns a path to the Sysprep volume mounted on a pod
func GetSysprepSourcePath(volumeName string) string {
	return filepath.Join(SysprepSourceDir, volumeName)
}

// GetSysprepDiskPath returns a path to a Sysprep iso image created based on a volume name
func GetSysprepDiskPath(volumeName string) string {
	return filepath.Join(SysprepDisksDir, volumeName+".iso")
}

func sysprepVolumeHasContents(sysprepVolume *v1.SysprepSource) bool {
	return sysprepVolume.ConfigMap != nil || sysprepVolume.Secret != nil
}

// Explained here: https://docs.microsoft.com/en-us/windows-hardware/manufacture/desktop/windows-setup-automation-overview
const autounattendFilename = "autounattend.xml"
const unattendFilename = "unattend.xml"

func validateUnattendPresence(entries []os.DirEntry) error {
	for _, entry := range entries {
		if name := strings.ToLower(entry.Name()); name == autounattendFilename || name == unattendFilename {
			return nil
		}
	}
	return fmt.Errorf("Sysprep drive should contain %s or %s but neither were found.", autounattendFilename, unattendFilename)
}

// CreateSysprepDisks creates Sysprep iso disks which are attached to vmis from either ConfigMap or Secret as a source
func CreateSysprepDisks(vmi *v1.VirtualMachineInstance, emptyIso bool) error {
	for _, volume := range vmi.Spec.Volumes {
		if !shouldCreateSysprepDisk(volume.Sysprep) {
			continue
		}
		plan, err := planSysprepISO(vmi, volume.Name, emptyIso)
		if err != nil {
			return err
		}
		if err := plan.write(); err != nil {
			return err
		}
	}
	return nil
}

func shouldCreateSysprepDisk(volumeSysprep *v1.SysprepSource) bool {
	return volumeSysprep != nil && sysprepVolumeHasContents(volumeSysprep)
}

func planSysprepISO(vmi *v1.VirtualMachineInstance, volumeName string, emptyIso bool) (isoPlan, error) {
	output := GetSysprepDiskPath(volumeName)
	size, err := findIsoSize(vmi, volumeName, emptyIso)
	if err != nil {
		return isoPlan{}, err
	}
	source := GetSysprepSourcePath(volumeName)
	entries, err := os.ReadDir(source)
	if err != nil {
		return isoPlan{}, fmt.Errorf("reading Sysprep source %s: %w", source, err)
	}
	if err := validateUnattendPresence(entries); err != nil {
		return isoPlan{}, err
	}
	return isoPlan{output: output, layout: BuildISOLayout(sysprepVolumeLabel, source, entries), placeholder: size != 0, size: size}, nil
}
