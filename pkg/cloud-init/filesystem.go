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

package cloudinit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/util"
	"kubevirt.io/kubevirt/pkg/util/net/dns"
)

const cloudInitLocalDir = "/var/run/kubevirt-ephemeral-disks/cloud-init-data"

// ReadCloudInitVolumeDataSource reads mounted secrets and returns decoded data
// and metadata for the first cloud-init volume. It does not modify vmi.
func ReadCloudInitVolumeDataSource(vmi *v1.VirtualMachineInstance, secretSourceDir string) (cloudInitData *CloudInitData, err error) {
	// ClusterInstancetypeAnnotation will take precedence over a namespaced Instancetype
	// for setting instance_type in the metadata
	instancetype := vmi.Annotations[v1.ClusterInstancetypeAnnotation]
	if instancetype == "" {
		instancetype = vmi.Annotations[v1.InstancetypeAnnotation]
	}

	hostname := dns.SanitizeHostname(vmi)

	for i := range vmi.Spec.Volumes {
		volume := &vmi.Spec.Volumes[i]
		if volume.CloudInitNoCloud != nil {
			keys, err := readSSHPublicKeys(vmi, secretSourceDir, DataSourceNoCloud)
			if err != nil {
				return nil, err
			}

			payloadVolume := secretVolume(vmi, DataSourceNoCloud)
			userData, networkData, err := readSecretPayloads(payloadVolume, secretSourceDir, DataSourceNoCloud)
			if err != nil {
				return nil, err
			}
			source := *volume.CloudInitNoCloud
			if payloadVolume == volume {
				if userData != "" {
					source.UserData = userData
				}
				if networkData != "" {
					source.NetworkData = networkData
				}
			}
			cloudInitData, err = readCloudInitNoCloudSource(&source)
			cloudInitData.NoCloudMetaData = readCloudInitNoCloudMetaData(hostname, string(vmi.Spec.Domain.Firmware.UUID), instancetype, keys)
			cloudInitData.VolumeName = volume.Name
			return cloudInitData, err
		}
		if volume.CloudInitConfigDrive != nil {

			keys, err := readSSHPublicKeys(vmi, secretSourceDir, DataSourceConfigDrive)
			if err != nil {
				return nil, err
			}

			payloadVolume := secretVolume(vmi, DataSourceConfigDrive)
			userData, networkData, err := readSecretPayloads(payloadVolume, secretSourceDir, DataSourceConfigDrive)
			if err != nil {
				return nil, err
			}
			source := *volume.CloudInitConfigDrive
			if payloadVolume == volume {
				if userData != "" {
					source.UserData = userData
				}
				if networkData != "" {
					source.NetworkData = networkData
				}
			}
			uuid := string(vmi.Spec.Domain.Firmware.UUID)
			cloudInitData, err = readCloudInitConfigDriveSource(&source)
			cloudInitData.ConfigDriveMetaData = readCloudInitConfigDriveMetaData(vmi.Name, uuid, hostname, vmi.Namespace, keys, instancetype)
			cloudInitData.VolumeName = volume.Name
			return cloudInitData, err
		}
	}
	return nil, nil
}

func readSecretFile(name string) (string, error) {
	// #nosec G304 -- paths identify the VMI's mounted secret files.
	content, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func readFirstSecretFile(root, volume, first, second string) (string, error) {
	content, err := readSecretFile(filepath.Join(root, volume, first))
	if err == nil {
		return content, nil
	}
	return readSecretFile(filepath.Join(root, volume, second))
}

func readSSHPublicKeys(vmi *v1.VirtualMachineInstance, root string, source DataSourceType) (map[string]string, error) {
	keys := make(map[string]string)
	for _, credential := range vmi.Spec.AccessCredentials {
		if !credentialSelected(credential, source) || credential.SSHPublicKey.Source.Secret == nil || credential.SSHPublicKey.Source.Secret.SecretName == "" {
			continue
		}
		directory := filepath.Join(root, credential.SSHPublicKey.Source.Secret.SecretName+"-access-cred")
		entries, err := os.ReadDir(directory)
		if err != nil {
			return keys, err
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), "..") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			content, err := readSecretFile(path)
			if err != nil {
				return keys, fmt.Errorf("Unable to read public keys found at volume: %s error: %v", path, err)
			}
			if content != "" {
				keys[strconv.Itoa(len(keys))] = content
			}
		}
	}
	return keys, nil
}

func readSecretPayloads(volume *v1.Volume, root string, source DataSourceType) (string, string, error) {
	if volume == nil {
		return "", "", nil
	}
	userRef, networkRef := secretRefs(volume, source)
	var userData, networkData string
	var userErr, networkErr error
	if userRef {
		userData, userErr = readFirstSecretFile(root, volume.Name, "userdata", "userData")
	}
	if networkRef {
		networkData, networkErr = readFirstSecretFile(root, volume.Name, "networkdata", "networkData")
	}
	if userErr != nil && networkErr != nil {
		return "", "", fmt.Errorf("no cloud-init data-source found at volume: %s", volume.Name)
	}
	return userData, networkData, nil
}

func domainBasePath(root, domain, namespace string) string {
	return fmt.Sprintf("%s/%s/%s", root, namespace, domain)
}

func GetIsoFilePath(source DataSourceType, domain, namespace string) string {
	fileName := noCloudFile
	if source == DataSourceConfigDrive {
		fileName = configDriveFile
	}
	return fmt.Sprintf("%s/%s", domainBasePath(cloudInitLocalDir, domain, namespace), fileName)
}

func PrepareLocalPath(vmiName, namespace string) error {
	return os.MkdirAll(domainBasePath(cloudInitLocalDir, vmiName, namespace), 0755)
}

// writeIsoPlan writes planned input files into a fresh directory.
// Callers own the input directory and its eventual cleanup.
func writeIsoPlan(dataRoot string, plan *IsoPlan) error {
	if err := os.MkdirAll(filepath.Join(dataRoot, plan.DataDir), 0700); err != nil {
		return err
	}
	for _, file := range plan.Files {
		filePath := filepath.Join(dataRoot, file.Path)
		if err := os.WriteFile(filePath, file.Data, 0600); err != nil {
			return err
		}
	}
	return nil
}

func GenerateIso(vmi *v1.VirtualMachineInstance, data *CloudInitData) error {
	return GenerateIsoInDir(cloudInitLocalDir, vmi, data)
}

// GenerateIsoInDir writes cloud-init contents into a fresh directory, creates the
// ISO, sets qemu ownership, and publishes it by renaming the staging file.
func GenerateIsoInDir(cloudInitLocalDir string, vmi *v1.VirtualMachineInstance, data *CloudInitData) error {
	plan, err := BuildIsoPlan(data)
	if err != nil {
		return err
	}
	domainRoot := domainBasePath(cloudInitLocalDir, vmi.Name, vmi.Namespace)
	iso := filepath.Join(domainRoot, plan.FileName)
	if err := writeIso(iso, func(staging string) error {
		dataRoot, err := os.MkdirTemp(domainRoot, "cloud-init-data-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dataRoot)
		if err := writeIsoPlan(dataRoot, plan); err != nil {
			return err
		}
		return dirToIso(dataRoot, staging, plan.VolumeID)
	}); err != nil {
		return err
	}
	log.Log.V(2).Infof("generated cloud-init iso file %s", iso)
	return nil
}

// GenerateIsoPlaceholder writes a zero-filled migration placeholder.
func GenerateIsoPlaceholder(vmiName, namespace string, data *CloudInitData, size int64) error {
	fileName, err := selectIsoFileName(data.DataSource)
	if err != nil {
		return err
	}
	iso := filepath.Join(domainBasePath(cloudInitLocalDir, vmiName, namespace), fileName)
	if err := writeIso(iso, func(staging string) error {
		file, err := os.Create(staging)
		if err != nil {
			return fmt.Errorf("failed to create empty iso: '%s'", staging)
		}
		writeErr := util.WriteBytes(file, 0, size)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}); err != nil {
		return err
	}
	log.Log.V(2).Infof("generated empty iso file %s", iso)
	return nil
}

func selectIsoFileName(source DataSourceType) (string, error) {
	switch source {
	case DataSourceNoCloud:
		return noCloudFile, nil
	case DataSourceConfigDrive:
		return configDriveFile, nil
	default:
		return "", fmt.Errorf("invalid cloud-init data source: '%v'", source)
	}
}

// writeIso generates a staging file and publishes it with qemu ownership.
func writeIso(iso string, generate func(staging string) error) error {
	staging := iso + ".staging"
	if err := removeFilesIfExist(staging); err != nil {
		return err
	}
	defer os.Remove(staging)
	if err := os.MkdirAll(filepath.Dir(iso), 0755); err != nil {
		return err
	}
	if err := generate(staging); err != nil {
		return err
	}
	if err := chownToQEMU(staging); err != nil {
		return err
	}
	if err := os.Rename(staging, iso); err != nil {
		log.Log.Reason(err).Errorf("Cloud-init failed to rename file %s to %s", staging, iso)
		return err
	}
	return nil
}

func removeFilesIfExist(paths ...string) error {
	for _, name := range paths {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func dirToIso(inDir, isoOutFile, volumeID string) error {

	var args []string

	args = append(args, "-output")
	args = append(args, isoOutFile)
	args = append(args, "-volid")
	args = append(args, volumeID)
	args = append(args, "-joliet")
	args = append(args, "-rock")
	args = append(args, "-partition_cyl_align")
	args = append(args, "on")
	args = append(args, inDir)

	const isoBinary = "xorrisofs"

	// #nosec No risk for attacker injection. Parameters are predefined strings
	cmd := exec.Command(isoBinary, args...)

	err := cmd.Start()
	if err != nil {
		log.Log.Reason(err).Errorf("%s cmd failed to start while generating iso file %s", isoBinary, isoOutFile)
		return err
	}

	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-timeout:
			log.Log.Errorf("Timed out generating cloud-init iso at path %s", isoOutFile)
			cmd.Process.Kill()
		case err := <-done:
			if err != nil {
				log.Log.Reason(err).Errorf("%s returned non-zero exit code while generating iso file %s with args '%s'", isoBinary, isoOutFile, strings.Join(cmd.Args, " "))
				return err
			}
			return nil
		}
	}
}

// chownToQEMU is needed if virt-launcher runs as featuregate.Root
func chownToQEMU(path string) error {
	const qemuUID = 107
	if os.Geteuid() != 0 {
		return nil
	}
	return os.Chown(path, qemuUID, qemuUID)
}
