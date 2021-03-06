/**
 * Copyright 2020-present Facebook. All Rights Reserved.
 *
 * This program file is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the
 * Free Software Foundation; version 2 of the License.
 *
 * This program is distributed in the hope that it will be useful, but WITHOUT
 * ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
 * FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License
 * for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program in a file named COPYING; if not, write to the
 * Free Software Foundation, Inc.,
 * 51 Franklin Street, Fifth Floor,
 * Boston, MA 02110-1301 USA
 */

package devices

import (
	"fmt"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/facebook/openbmc/tools/flashy/lib/fileutils"
	"github.com/facebook/openbmc/tools/flashy/lib/utils"
	"github.com/facebook/openbmc/tools/flashy/lib/validate"
	"github.com/pkg/errors"
)

func init() {
	registerFlashDeviceFactory(MtdType, getMTD)
}

func getMTD(deviceSpecifier string) (FlashDevice, error) {
	mtdMap, err := utils.GetMTDMapFromSpecifier(deviceSpecifier)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join("/dev", mtdMap["dev"])
	fileSize, err := strconv.ParseUint(mtdMap["size"], 16, 64)
	if err != nil {
		return nil, errors.Errorf("Found MTD entry for flash device 'mtd:%v' but got error '%v'",
			deviceSpecifier, err)
	}

	return &MemoryTechnologyDevice{
		deviceSpecifier,
		filePath,
		fileSize,
	}, nil
}

// WritableMountedMTD contains information of a mounted MTD that is writable.
type WritableMountedMTD struct {
	Device     string
	Mountpoint string
}

// MemoryTechnologyDevice contains information of an MTD, and implements FlashDevice.
type MemoryTechnologyDevice struct {
	Specifier string
	FilePath  string
	FileSize  uint64
}

// MtdType represents a constant for the type of the flash device.
const MtdType = "mtd"

func (m *MemoryTechnologyDevice) GetType() string {
	return MtdType
}

func (m *MemoryTechnologyDevice) GetSpecifier() string {
	return m.Specifier
}

func (m *MemoryTechnologyDevice) GetFilePath() string {
	return m.FilePath
}

func (m *MemoryTechnologyDevice) GetFileSize() uint64 {
	return m.FileSize
}

// MmapRO mmaps the whole file in readonly mode.
// A Munmap call is required to release the buffer.
func (m *MemoryTechnologyDevice) MmapRO() ([]byte, error) {
	// use mmap
	mmapFilePath, err := GetMTDBlockFilePath(m.FilePath)
	if err != nil {
		return nil, err
	}
	return fileutils.MmapFileRange(mmapFilePath, 0, int(m.FileSize), syscall.PROT_READ, syscall.MAP_SHARED)
}

func (m *MemoryTechnologyDevice) Munmap(buf []byte) error {
	return fileutils.Munmap(buf)
}

func (m *MemoryTechnologyDevice) Validate() error {
	data, err := m.MmapRO()
	if err != nil {
		return errors.Errorf("Can't mmap flash device: %v", err)
	}
	defer m.Munmap(data)
	return validate.Validate(data)
}

// GetMTDBlockFilePath gets the /dev/mtdblock[0-9]+ file path
// from the /dev/mtd[0-9]+ file path.
// /dev/mtd5 cannot be mmap-ed, but /dev/mtdblock5 can.
var GetMTDBlockFilePath = func(filepath string) (string, error) {
	regEx := `^(?P<devmtdpath>/dev/mtd)(?P<mtdnum>[0-9]+)$`

	mtdPathMap, err := utils.GetRegexSubexpMap(regEx, filepath)
	if err != nil {
		return "", errors.Errorf("Unable to get block file path for '%v': %v",
			filepath, err)
	}

	return fmt.Sprintf(
		"%vblock%v",
		mtdPathMap["devmtdpath"],
		mtdPathMap["mtdnum"],
	), nil
}

// GetWritableMountedMTDs returns all writable mounted MTDs as specified in /proc/mounts.
var GetWritableMountedMTDs = func() ([]WritableMountedMTD, error) {
	writableMountedMTDs := []WritableMountedMTD{}

	procMountsBuf, err := fileutils.ReadFile("/proc/mounts")
	if err != nil {
		return writableMountedMTDs,
			errors.Errorf("Unable to get writable mounted MTDs: Cannot read /proc/mounts: %v", err)
	}
	procMounts := string(procMountsBuf)

	// device, mountpoint, filesystem, options, dump_freq, fsck_pass
	regEx := `(?m)^(?P<device>/dev/mtd(?:block)?[0-9]+) (?P<mountpoint>[^ ]+) [^ ]+ [^ ]*rw[^ ]* [0-9]+ [0-9]+$`

	allMTDMaps, err := utils.GetAllRegexSubexpMap(regEx, procMounts)
	if err != nil {
		return writableMountedMTDs,
			errors.Errorf("Unable to get writable mounted MTDs: %v", err)
	}

	for _, mtdMap := range allMTDMaps {
		writableMountedMTDs =
			append(writableMountedMTDs,
				WritableMountedMTD{
					mtdMap["device"],
					mtdMap["mountpoint"],
				},
			)
	}

	return writableMountedMTDs, nil
}
