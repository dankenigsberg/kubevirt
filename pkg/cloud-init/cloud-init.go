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
	"encoding/base64"
	"encoding/json"
	"fmt"

	v1 "kubevirt.io/api/core/v1"
)

// Locations of data source disk files
const (
	noCloudFile     = "noCloud.iso"
	configDriveFile = "configdrive.iso"
)

type DataSourceType string
type DeviceMetadataType string

const (
	DataSourceNoCloud     DataSourceType     = "noCloud"
	DataSourceConfigDrive DataSourceType     = "configDrive"
	NICMetadataType       DeviceMetadataType = "nic"
	HostDevMetadataType   DeviceMetadataType = "hostdev"
)

// CloudInitData is a data source independent struct that
// holds cloud-init user and network data
type CloudInitData struct {
	DataSource          DataSourceType
	NoCloudMetaData     *NoCloudMetadata
	ConfigDriveMetaData *ConfigDriveMetadata
	UserData            string
	NetworkData         string
	DevicesData         *[]DeviceData
	VolumeName          string
}

type NoCloudMetadata struct {
	InstanceType  string            `json:"instance-type,omitempty"`
	InstanceID    string            `json:"instance-id"`
	LocalHostname string            `json:"local-hostname,omitempty"`
	PublicSSHKeys map[string]string `json:"public-keys,omitempty"`
}

type ConfigDriveMetadata struct {
	InstanceType  string            `json:"instance_type,omitempty"`
	InstanceID    string            `json:"instance_id"`
	LocalHostname string            `json:"local_hostname,omitempty"`
	Hostname      string            `json:"hostname,omitempty"`
	UUID          string            `json:"uuid,omitempty"`
	Devices       *[]DeviceData     `json:"devices,omitempty"`
	PublicSSHKeys map[string]string `json:"public_keys,omitempty"`
}

type DeviceData struct {
	Type        DeviceMetadataType `json:"type"`
	Bus         string             `json:"bus"`
	Address     string             `json:"address"`
	MAC         string             `json:"mac,omitempty"`
	Serial      string             `json:"serial,omitempty"`
	NumaNode    uint32             `json:"numaNode,omitempty"`
	AlignedCPUs []uint32           `json:"alignedCPUs,omitempty"`
	Tags        []string           `json:"tags"`
}

// IsValidCloudInitData checks if the given CloudInitData object is valid in the sense that GenerateIso can be called with it.
func IsValidCloudInitData(cloudInitData *CloudInitData) bool {
	return cloudInitData != nil && cloudInitData.UserData != "" && (cloudInitData.NoCloudMetaData != nil || cloudInitData.ConfigDriveMetaData != nil)
}

func readRawOrBase64Data(rawData, base64Data string) (string, error) {
	if rawData != "" {
		return rawData, nil
	} else if base64Data != "" {
		bytes, err := base64.StdEncoding.DecodeString(base64Data)
		return string(bytes), err
	}
	return "", nil
}

// readCloudInitData reads user and network data raw or in base64 encoding,
// regardless from which data source they are coming from
func readCloudInitData(userData, userDataBase64, networkData, networkDataBase64 string) (string, string, error) {
	readUserData, err := readRawOrBase64Data(userData, userDataBase64)
	if err != nil {
		return "", "", err
	}

	readNetworkData, err := readRawOrBase64Data(networkData, networkDataBase64)
	if err != nil {
		return "", "", err
	}

	if readUserData == "" && readNetworkData == "" {
		return "", "", fmt.Errorf("userDataBase64, userData, networkDataBase64 or networkData is required for a cloud-init data source")
	}

	return readUserData, readNetworkData, nil
}

func readCloudInitNoCloudSource(source *v1.CloudInitNoCloudSource) (*CloudInitData, error) {
	userData, networkData, err := readCloudInitData(source.UserData,
		source.UserDataBase64, source.NetworkData, source.NetworkDataBase64)
	if err != nil {
		return &CloudInitData{}, err
	}

	return &CloudInitData{
		DataSource:  DataSourceNoCloud,
		UserData:    userData,
		NetworkData: networkData,
	}, nil
}

func readCloudInitConfigDriveSource(source *v1.CloudInitConfigDriveSource) (*CloudInitData, error) {
	userData, networkData, err := readCloudInitData(source.UserData,
		source.UserDataBase64, source.NetworkData, source.NetworkDataBase64)
	if err != nil {
		return &CloudInitData{}, err
	}

	return &CloudInitData{
		DataSource:  DataSourceConfigDrive,
		UserData:    userData,
		NetworkData: networkData,
	}, nil
}

func readCloudInitNoCloudMetaData(hostname, instanceId string, instanceType string, keys map[string]string) *NoCloudMetadata {
	return &NoCloudMetadata{
		InstanceType:  instanceType,
		InstanceID:    instanceId,
		LocalHostname: hostname,
		PublicSSHKeys: keys,
	}
}

func readCloudInitConfigDriveMetaData(name, uuid, hostname, namespace string, keys map[string]string, instanceType string) *ConfigDriveMetadata {
	return &ConfigDriveMetadata{
		InstanceType:  instanceType,
		UUID:          uuid,
		InstanceID:    fmt.Sprintf("%s.%s", name, namespace),
		Hostname:      hostname,
		PublicSSHKeys: keys,
	}
}

func credentialSelected(credential v1.AccessCredential, source DataSourceType) bool {
	if credential.SSHPublicKey == nil {
		return false
	}
	switch source {
	case DataSourceNoCloud:
		return credential.SSHPublicKey.PropagationMethod.NoCloud != nil
	case DataSourceConfigDrive:
		return credential.SSHPublicKey.PropagationMethod.ConfigDrive != nil
	}
	return false
}

func secretVolume(vmi *v1.VirtualMachineInstance, source DataSourceType) *v1.Volume {
	for i := range vmi.Spec.Volumes {
		volume := &vmi.Spec.Volumes[i]
		switch source {
		case DataSourceNoCloud:
			if volume.CloudInitNoCloud != nil && (volume.CloudInitNoCloud.UserDataSecretRef != nil || volume.CloudInitNoCloud.NetworkDataSecretRef != nil) {
				return volume
			}
		case DataSourceConfigDrive:
			if volume.CloudInitConfigDrive != nil && (volume.CloudInitConfigDrive.UserDataSecretRef != nil || volume.CloudInitConfigDrive.NetworkDataSecretRef != nil) {
				return volume
			}
		}
	}
	return nil
}

func secretRefs(volume *v1.Volume, source DataSourceType) (bool, bool) {
	if source == DataSourceNoCloud {
		return volume.CloudInitNoCloud.UserDataSecretRef != nil, volume.CloudInitNoCloud.NetworkDataSecretRef != nil
	}
	return volume.CloudInitConfigDrive.UserDataSecretRef != nil, volume.CloudInitConfigDrive.NetworkDataSecretRef != nil
}

// IsoFile is content at a relative path inside the ISO input root.
type IsoFile struct {
	Path string
	Data []byte
}

// IsoPlan describes complete ISO contents, without absolute host paths.
type IsoPlan struct {
	VolumeID string
	FileName string
	DataDir  string
	Files    []IsoFile
}

// BuildIsoPlan chooses the ISO layout and serializes metadata without IO.
// Identity metadata comes from ReadCloudInitVolumeDataSource and may be
// changed by hooks.
// ConfigDrive devices are added to a local metadata copy; data is not modified.
func BuildIsoPlan(data *CloudInitData) (*IsoPlan, error) {
	plan := &IsoPlan{}
	var metaFile, userFile, networkFile string
	var metadata []byte
	var err error
	switch data.DataSource {
	case DataSourceNoCloud:
		plan.VolumeID, plan.FileName = "cidata", noCloudFile
		metaFile, userFile, networkFile = "meta-data", "user-data", "network-config"
		metadata, err = json.Marshal(data.NoCloudMetaData)
	case DataSourceConfigDrive:
		plan.VolumeID, plan.FileName, plan.DataDir = "config-2", configDriveFile, "openstack/latest"
		metaFile, userFile, networkFile = "openstack/latest/meta_data.json", "openstack/latest/user_data", "openstack/latest/network_data.json"
		configDriveMetadata := data.ConfigDriveMetaData
		if configDriveMetadata != nil {
			withDevices := *configDriveMetadata
			withDevices.Devices = data.DevicesData
			configDriveMetadata = &withDevices
		}
		metadata, err = json.Marshal(configDriveMetadata)
	default:
		return nil, fmt.Errorf("Invalid cloud-init data source: '%v'", data.DataSource)
	}
	if err != nil {
		return nil, err
	}
	if data.UserData == "" && data.NetworkData == "" {
		return plan, fmt.Errorf("UserData or NetworkData is required for cloud-init data source")
	}
	plan.Files = []IsoFile{{Path: userFile, Data: []byte(data.UserData)}, {Path: metaFile, Data: metadata}}
	if data.NetworkData != "" {
		plan.Files = append(plan.Files, IsoFile{Path: networkFile, Data: []byte(data.NetworkData)})
	}
	return plan, nil
}
