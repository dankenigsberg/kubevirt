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
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/util"
)

const cloudInitLocalDir = "/var/run/libvirt/cloud-init-dir"

// ReadSecretData reads the requested mounted-secret files and directories.
// Read failures are recorded for ResolveVolumeData to interpret; this layer
// does not choose aliases or decide whether a missing payload is acceptable.
func ReadSecretData(root string, requirements SecretReadRequirements) SecretData {
	secretData := SecretData{
		SourceDir:   root,
		Files:       make(map[string]SecretFile),
		Directories: make(map[string]SecretDirectory),
	}
	for _, name := range requirements.Directories {
		entries, err := os.ReadDir(filepath.Join(root, name))
		directory := SecretDirectory{Err: err}
		for _, entry := range entries {
			observed := SecretEntry{Name: entry.Name(), IsDir: entry.IsDir()}
			// Projected-secret bookkeeping and real directories have no key
			// content. Preserve their entry observations without opening them.
			if secretEntryHasContent(observed.Name, observed.IsDir) {
				observed.Data, observed.Err = readSecretFile(filepath.Join(root, name, observed.Name))
			}
			directory.Entries = append(directory.Entries, observed)
		}
		secretData.Directories[name] = directory
	}
	for _, name := range requirements.Files {
		content, err := readSecretFile(filepath.Join(root, name))
		secretData.Files[name] = SecretFile{Data: content, Err: err}
	}
	return secretData
}

func readSecretFile(name string) (string, error) {
	// #nosec G304 -- paths identify the VMI's mounted secret files.
	content, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// ReadCloudInitVolumeDataSource reads mounted secrets and returns decoded data
// and metadata for the first cloud-init volume. It does not modify vmi.
func ReadCloudInitVolumeDataSource(vmi *v1.VirtualMachineInstance, secretSourceDir string) (*CloudInitData, error) {
	requirements := planSecretReads(vmi)
	secretData := ReadSecretData(secretSourceDir, requirements)
	return ResolveVolumeData(vmi, secretData)
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
	if err := os.MkdirAll(domainRoot, 0755); err != nil {
		return err
	}
	dataRoot, err := os.MkdirTemp(domainRoot, "cloud-init-data-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dataRoot)
	iso := filepath.Join(domainRoot, plan.FileName)
	staging := iso + ".staging"
	if err := removeFilesIfExist(staging); err != nil {
		return err
	}
	defer os.Remove(staging)
	if err := writeIsoPlan(dataRoot, plan); err != nil {
		return err
	}
	if err := dirToIso(dataRoot, staging, plan.VolumeID); err != nil {
		return err
	}
	if err := chownToQEMU(staging); err != nil {
		return err
	}
	if err := os.Rename(staging, iso); err != nil {
		log.Log.Reason(err).Errorf("Cloud-init failed to rename file %s to %s", staging, iso)
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
	return writeIsoPlaceholder(domainBasePath(cloudInitLocalDir, vmiName, namespace), fileName, size)
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

// writeIsoPlaceholder writes a zero-filled file with qemu ownership.
func writeIsoPlaceholder(domainRoot, fileName string, size int64) error {
	iso := filepath.Join(domainRoot, fileName)
	staging := iso + ".staging"
	if err := removeFilesIfExist(staging); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(staging), 0755); err != nil {
		return err
	}
	file, err := os.Create(staging)
	if err != nil {
		return fmt.Errorf("failed to create empty iso: '%s'", staging)
	}
	writeErr := util.WriteBytes(file, 0, size)
	// Close on both success and error, without changing the returned write error.
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := chownToQEMU(staging); err != nil {
		return err
	}
	if err := os.Rename(staging, iso); err != nil {
		log.Log.Reason(err).Errorf("Cloud-init failed to rename file %s to %s", staging, iso)
		return err
	}
	log.Log.V(2).Infof("generated empty iso file %s", iso)
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

func chownToQEMU(path string) error {
	owner, err := user.Lookup("qemu")
	if err != nil {
		return fmt.Errorf("failed to look up qemu user: %w", err)
	}
	uid, err := strconv.Atoi(owner.Uid)
	if err != nil {
		return fmt.Errorf("invalid qemu UID %q: %w", owner.Uid, err)
	}
	gid, err := strconv.Atoi(owner.Gid)
	if err != nil {
		return fmt.Errorf("invalid qemu GID %q: %w", owner.Gid, err)
	}
	return os.Chown(path, uid, gid)
}
