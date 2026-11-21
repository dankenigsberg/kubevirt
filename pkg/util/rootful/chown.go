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

package rootful

import (
	"os"
	"path/filepath"

	"kubevirt.io/kubevirt/pkg/safepath"
	"kubevirt.io/kubevirt/pkg/util"
)

// ChownToQemu sets the QEMU file owner and group when running as root.
// It rejects symlinks in the path and changes ownership on the opened file.
func ChownToQemu(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if path == "" {
		return &os.PathError{Op: "chown", Path: path, Err: os.ErrNotExist}
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	file, err := safepath.NewFileNoFollow(absolutePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return os.Chown(file.SafePath(), util.NonRootUID, util.NonRootUID)
}
