// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package universalpayload

import (
	"bufio"
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/u-root/u-root/pkg/align"
	"github.com/u-root/u-root/pkg/boot/kexec"
	"github.com/u-root/u-root/pkg/dt"
)

var sysfsCPUInfoPath = "/proc/cpuinfo"

// Properties to be fetched from device tree.
const (
	FirstLevelNodeName     = "images"
	SecondLevelNodeName    = "tianocore"
	LoadAddrPropertyName   = "load"
	EntryAddrPropertyName  = "entry-start"
	DataOffsetPropertyName = "data-offset"
	DataSizePropertyName   = "data-size"
)

// Memory Region layout when loading universalpayload image.
// Please do not change the layout, since components have dependencies:
//   TRAMPOLINE CODE depends on base address of:
//     TEMP STACK, Device Tree Info, ACPI DATA, UPL FIT IMAGE
//   Device Tree Info depends on base address of:
//     HoBs, ACPI DATA, UPL FIT IMAGE
//
// |------------------------| <-- Memory Region top
// |     TRAMPOLINE CODE    |
// |------------------------| <-- loadAddr + trampolineOffset
// |      TEMP STACK        |
// |------------------------| <-- loadAddr + tmpStackOffset
// |    Device Tree Info    |
// |------------------------| <-- loadAddr + fdtDtbOffset
// |  BOOTLOADER PARAMETER  |
// |  HoBs (Handoff Blocks) |
// |------------------------| <-- loadAddr + tmpHobOffset
// |       ACPI DATA        |
// |------------------------| <-- loadAddr + rsdpTableOffset
// |     UPL FIT IMAGE      |
// |------------------------| <-- loadAddr which is 2MB aligned
//
// During runtime, we need to find a available Memory Region to place all
// above components, size of each components should be updated at runtime.
//
// uplImageOffset is always set to be Zero. We keep it here in case
// anything more needs to be placed before UPL Image.
// Components should be placed by above sequence, once component is placed,
// offset of next component should be updated at once to ensure all offset
// information are updated correctly.

var (
	uplImageOffset   uint64
	rsdpTableOffset  uint64
	tmpHobOffset     uint64
	fdtDtbOffset     uint64
	tmpStackOffset   uint64
	trampolineOffset uint64
)

// componentsSize is used to check whether reversed size, which is defined in
// const variable 'sizeForComponents', is enough for us to place all required
// components.
var (
	componentsSize uint
)

const (
	sizeForComponents int  = 0x100000
	uplImageAlignment uint = 0x200000
)

const (
	// Relocation Types
	IMAGE_REL_BASED_ABSOLUTE = 0
	IMAGE_REL_BASED_HIGHLOW  = 3
	IMAGE_REL_BASED_DIR64    = 10
)

const (
	sysfsFbPath  = "/dev/fb0"
	sysfsDrmPath = "/sys/class/drm"
)

// Definitions for ioctl and framebuffer structures in Go
const (
	FbIotclVscreeninfo    = 0x4600
	FbIoctlGetFscreenInfo = 0x4602
)

const (
	currentVersion    = 0x11
	lastCompVersion   = 0x10
	bootPhysicalCPUID = 0x0
)

var (
	pageSize = uint(os.Getpagesize())
)

type FbBitfield struct {
	Offset   uint32 // beginning of bitfield
	Length   uint32 // length of bitfield
	MsbRight uint32 // != 0 : Most significant bit is right
}

type FbVarScreenInfo struct {
	Xres         uint32
	Yres         uint32
	XresVirtual  uint32
	YresVirtual  uint32
	Xoffset      uint32
	Yoffset      uint32
	BitsPerPixel uint32
	Grayscale    uint32
	Red          FbBitfield
	Green        FbBitfield
	Blue         FbBitfield
	Transp       FbBitfield
	Nonstd       uint32
	Activate     uint32
	Height       uint32
	Width        uint32
	AccelFlags   uint32
	PixClock     uint32
	LeftMargin   uint32
	RightMargin  uint32
	UpperMargin  uint32
	LowerMargin  uint32
	HsyncLen     uint32
	VsyncLen     uint32
	Sync         uint32
	Vmode        uint32
	Rotate       uint32
	Colorspace   uint32
	Reserved     [4]uint32
}

type FbFixScreenInfo struct {
	ID           [16]byte
	SmemStart    uint64
	SmemLen      uint32
	Type         uint32
	TypeAux      uint32
	Visual       uint32
	Xpanstep     uint16
	Ypanstep     uint16
	Ywrapstep    uint16
	LineLength   uint32
	MmioStart    uint64
	MmioLen      uint32
	Accel        uint32
	Capabilities uint16
	Reserved     [2]uint16
}

type MCFGBaseAddressAllocation struct {
	BaseAddr  EFIPhysicalAddress
	PCISegGrp uint16
	StartBus  uint8
	EndBus    uint8
	Reserved  uint32
}

type ResourceInfo struct {
	Type        string
	BaseAddress string
	EndAddress  string
	Attributes  string
}

type ResourceRegions struct {
	MMIO64Base  uint64
	MMIO64Limit uint64
	MMIO32Base  uint64
	MMIO32Limit uint64
	IOPortBase  uint64
	IOPortLimit uint64
}

const (
	ACPIMCFGSysFilePath                 = "/sys/firmware/acpi/tables/MCFG"
	ACPIMCFGPciSegInfoStructureSize     = 0xC
	ACPIMCFGPciSegInfoDataLength        = 0xA
	ACPIMCFGBaseAddressAllocationLenth  = 0x10
	ACPIMCFGBaseAddressAllocationOffset = 0x2c
	ACPIMCFGSignature                   = "MCFG"
)

const (
	PCISearchPath   = "/sys/bus/pci/devices/"
	PCIMMIO64Attr   = 0x140204
	PCIMMIO32Attr   = 0x40200
	PCIIOPortAttr   = 0x40100
	PCIMMIOReadOnly = 0x4000
	PCIMMIO64Type   = "MMIO64"
	PCIMMIO32Type   = "MMIO32"
	PCIIOPortType   = "IOPORT"

	PCIMMIO64InvalidBase = 0xFFFF_FFFF_FFFF_FFFF
	PCIMMIO32InvalidBase = 0xFFFF_FFFF
	PCIIOPortInvalidBase = 0xFFFF
)

type FdtLoad struct {
	Load       uint64
	EntryStart uint64
	DataOffset uint32
	DataSize   uint32
}

// Errors returned by utilities
var (
	ErrFailToReadFdtFile           = errors.New("failed to read fdt file")
	ErrNodeImagesNotFound          = fmt.Errorf("failed to find '%s' node", FirstLevelNodeName)
	ErrNodeTianocoreNotFound       = fmt.Errorf("failed to find '%s' node", SecondLevelNodeName)
	ErrNodeLoadNotFound            = fmt.Errorf("failed to find get '%s' property", LoadAddrPropertyName)
	ErrNodeEntryStartNotFound      = fmt.Errorf("failed to find get '%s' property", EntryAddrPropertyName)
	ErrNodeDataOffsetNotFound      = fmt.Errorf("failed to find get '%s' property", DataOffsetPropertyName)
	ErrNodeDataSizeNotFound        = fmt.Errorf("failed to find get '%s' property", DataSizePropertyName)
	ErrFailToConvertLoad           = fmt.Errorf("failed to convert property '%s' to u64", LoadAddrPropertyName)
	ErrFailToConvertEntryStart     = fmt.Errorf("failed to convert property '%s' to u64", EntryAddrPropertyName)
	ErrFailToConvertDataOffset     = fmt.Errorf("failed to convert property '%s' to u32", DataOffsetPropertyName)
	ErrFailToConvertDataSize       = fmt.Errorf("failed to convert property '%s' to u32", DataSizePropertyName)
	ErrPeFailToGetPageRVA          = fmt.Errorf("failed to read pagerva during pe file relocation")
	ErrPeFailToGetBlockSize        = fmt.Errorf("failed to read block size during pe file relocation")
	ErrPeFailToGetEntry            = fmt.Errorf("failed to get entry during pe file relocation")
	ErrPeFailToCreatePeFile        = fmt.Errorf("failed to create pe file")
	ErrPeFailToGetRelocData        = fmt.Errorf("failed to get .reloc section data")
	ErrPeUnsupportedPeHeader       = fmt.Errorf("unsupported pe header format")
	ErrPeRelocOutOfBound           = fmt.Errorf("relocation address out of bounds during pe file relocation")
	ErrFBOpenFileFailed            = fmt.Errorf("failed opening framebuffer device")
	ErrFBGetVscreenInfoFailed      = fmt.Errorf("error getting variable screen info")
	ErrFBGetFscreenInfoFailed      = fmt.Errorf("error getting fixed screen info")
	ErrFBGetDevResrouceFailed      = fmt.Errorf("failed to get framebuffer mmio resource")
	ErrGfxOpenFileFailed           = fmt.Errorf("failed opening graphic device")
	ErrGfxReadVendorIDFailed       = fmt.Errorf("failed to read vendor id")
	ErrGfxReadDeviceIDFailed       = fmt.Errorf("failed to device vendor id")
	ErrGfxReadRevisionIDFailed     = fmt.Errorf("failed to revision vendor id")
	ErrGfxReadSubSysVendorIDFailed = fmt.Errorf("failed to read subsystem vendor id")
	ErrGfxReadSubSysDeviceIDFailed = fmt.Errorf("failed to read subsystem device id")
	ErrGfxNoDeviceInfoFound        = fmt.Errorf("no graphic device info found")
	ErrSMBIOS3NotFound             = fmt.Errorf("no smbios3 region found")
	ErrDTRsdpLenOverBound          = fmt.Errorf("rsdp table length too large")
	ErrDTRsdpTableNotFound         = fmt.Errorf("no acpi rsdp table found")
	ErrAlignPadRange               = errors.New("failed to align pad size, out of range")
	ErrCPUAddressConvert           = errors.New("failed to convert physical bits size")
	ErrCPUAddressRead              = errors.New("failed to read 'address sizes'")
	ErrCPUAddressNotFound          = errors.New("'address sizes' information not found")
	ErrMcfgDataLenthTooShort       = errors.New("acpi mcfg data lenth too short")
	ErrMcfgSignatureMismatch       = errors.New("acpi mcfg signature mismatch")
	ErrMcfgBaseAddrAllocCorrupt    = errors.New("acpi mcfg base address allocation data corrupt")
	ErrMcfgBaseAddrAllocDecode     = errors.New("failed to decode mcfg base address allocation structure")
)

func parseUint64ToUint32(val uint64) uint32 {
	if val > 0 && val <= math.MaxUint32 {
		return uint32(val)
	}
	return math.MaxUint32

}

// GetFdtInfo Device Tree Blob resides at the start of FIT binary. In order to
// get the expected load and entry point address, need to walk through
// DTB to get value of properties 'load' and 'entry-start'.
//
// The simplified device tree layout is:
//
//	/{
//	    images {
//	        tianocore {
//	            data-offset = <0x00001000>;
//	            data-size = <0x00010000>;
//	            entry-start = <0x00000000 0x00805ac3>;
//	            load = <0x00000000 0x00800000>;
//	        }
//	    }
//	 }
func GetFdtInfo(name string) (*FdtLoad, error) {
	return getFdtInfo(name, nil)
}

func getFdtInfo(name string, dtb io.ReaderAt) (*FdtLoad, error) {
	var fdt *dt.FDT
	var err error

	if dtb != nil {
		fdt, err = dt.ReadFDT(io.NewSectionReader(dtb, 0, math.MaxInt64))
	} else {
		fdt, err = dt.ReadFile(name)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: fdt file: %s, err: %w", ErrFailToReadFdtFile, name, err)
	}

	firstLevelNode, succeed := fdt.NodeByName(FirstLevelNodeName)
	if !succeed {
		return nil, ErrNodeImagesNotFound
	}

	secondLevelNode, succeed := firstLevelNode.NodeByName(SecondLevelNodeName)
	if !succeed {
		return nil, ErrNodeTianocoreNotFound
	}

	loadAddrProp, succeed := secondLevelNode.LookProperty(LoadAddrPropertyName)
	if !succeed {
		return nil, ErrNodeLoadNotFound
	}

	loadAddr, err := loadAddrProp.AsU64()
	if err != nil {
		return nil, errors.Join(ErrFailToConvertLoad, err)
	}

	entryAddrProp, succeed := secondLevelNode.LookProperty(EntryAddrPropertyName)
	if !succeed {
		return nil, ErrNodeEntryStartNotFound
	}

	entryAddr, err := entryAddrProp.AsU64()
	if err != nil {
		return nil, errors.Join(ErrFailToConvertEntryStart, err)
	}

	dataOffsetProp, succeed := secondLevelNode.LookProperty(DataOffsetPropertyName)
	if !succeed {
		return nil, ErrNodeDataOffsetNotFound
	}

	dataOffset, err := dataOffsetProp.AsU32()
	if err != nil {
		return nil, errors.Join(ErrFailToConvertDataOffset, err)
	}

	dataSizeProp, succeed := secondLevelNode.LookProperty(DataSizePropertyName)
	if !succeed {
		return nil, ErrNodeDataSizeNotFound
	}

	dataSize, err := dataSizeProp.AsU32()
	if err != nil {
		return nil, errors.Join(ErrFailToConvertDataSize, err)
	}

	return &FdtLoad{
		Load:       loadAddr,
		EntryStart: entryAddr,
		DataOffset: dataOffset,
		DataSize:   dataSize,
	}, nil
}

// alignHOBLength writes pad bytes at the end of a HOB buf
// It's because we calculate HOB length with golang, while write bytes to the buf with actual length
func alignHOBLength(expectLen uint64, bufLen int, buf *bytes.Buffer) error {
	if expectLen < uint64(bufLen) {
		return ErrAlignPadRange
	}

	if expectLen > math.MaxInt {
		return ErrAlignPadRange
	}
	if padLen := int(expectLen) - bufLen; padLen > 0 {
		pad := make([]byte, padLen)
		if err := binary.Write(buf, binary.LittleEndian, pad); err != nil {
			return err
		}
	}
	return nil
}

// Walk through .reloc section, update expected address to actual address
// which is calculated with recloation offset. Currently, only type of
// IMAGE_REL_BASED_DIR64(10) found in .reloc setcion, update this type
// of address only.
func relocatePE(relocData []byte, delta uint64, data []byte) error {
	r := bytes.NewReader(relocData)

	for {
		// Read relocation block header
		var pageRVA uint32
		var blockSize uint32

		err := binary.Read(r, binary.LittleEndian, &pageRVA)
		if err == io.EOF {
			break // End of relocations
		}
		if err != nil {
			return fmt.Errorf("%w: err: %w", ErrPeFailToGetPageRVA, err)
		}
		if err = binary.Read(r, binary.LittleEndian, &blockSize); err != nil {
			return fmt.Errorf("%w: err: %w", ErrPeFailToGetBlockSize, err)
		}

		// Block size includes the header, so the number of entries is (blockSize - 8) / 2
		entryCount := (blockSize - 8) / 2
		for i := 0; i < int(entryCount); i++ {
			var entry uint16
			if err := binary.Read(r, binary.LittleEndian, &entry); err != nil {
				return fmt.Errorf("%w: err: %w", ErrPeFailToGetEntry, err)
			}

			// Type is in the high 4 bits, offset is in the low 12 bits
			entryType := entry >> 12
			entryOffset := entry & 0xfff

			// Only type IMAGE_REL_BASED_DIR64(10) found
			if entryType == IMAGE_REL_BASED_DIR64 {
				// Perform relocation
				relocAddr := pageRVA + uint32(entryOffset)
				if relocAddr >= uint32(len(data)) {
					return ErrPeRelocOutOfBound
				}
				originalValue := binary.LittleEndian.Uint64(data[relocAddr:])
				relocatedValue := originalValue + delta
				binary.LittleEndian.PutUint64(data[relocAddr:], relocatedValue)
			}
		}
	}
	return nil
}

func relocateFdtData(dst uint64, fdtLoad *FdtLoad, data []byte) error {
	// Get the region of universalpayload binary from FIT image
	start := fdtLoad.DataOffset
	end := fdtLoad.DataOffset + fdtLoad.DataSize

	reader := bytes.NewReader(data[start:end])

	peFile, err := pe.NewFile(reader)
	if err != nil {
		return ErrPeFailToCreatePeFile
	}
	defer peFile.Close()

	optionalHeader, success := peFile.OptionalHeader.(*pe.OptionalHeader64)
	if !success {
		return ErrPeUnsupportedPeHeader
	}

	preBase := optionalHeader.ImageBase
	delta := dst + uint64(fdtLoad.DataOffset) - preBase

	for _, section := range peFile.Sections {
		if section.Name == ".reloc" {
			relocData, err := section.Data()
			if err != nil {
				return ErrPeFailToGetRelocData
			}

			if err := relocatePE(relocData, delta, data[start:end]); err != nil {
				return err
			}
		}
	}

	fdtLoad.EntryStart = dst + (fdtLoad.EntryStart - fdtLoad.Load)
	fdtLoad.Load = dst

	return nil
}

func buildDtMemoryNode(mem *kexec.Memory) []*dt.Node {
	// Get the RAM ranges from the memory map
	ramRanges := mem.Phys.RAM()
	var memNodes []*dt.Node

	// Iterate over each RAM range and create a dt.Node
	for _, ram := range ramRanges {
		// Create node name using the start address
		nodeName := fmt.Sprintf("memory@0x%x", ram.Start)

		// Create the node with the "reg" property containing start address and size
		node := dt.NewNode(nodeName, dt.WithProperty(
			dt.PropertyRegion("reg", uint64(ram.Start), uint64(align.UpPage(ram.Size))),
		))

		// Append the created node to the list
		memNodes = append(memNodes, node)
	}

	return memNodes
}

func fetchResourceRange(filepath string) (string, string, error) {
	// Open the file
	file, err := os.Open(filepath)
	if err != nil {
		return "", "", ErrFBOpenFileFailed
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Iterate over the lines
	for scanner.Scan() {
		line := scanner.Text()

		// Split the line into fields
		fields := strings.Fields(line)

		// We expect at least two values in each line
		if len(fields) >= 2 {
			// Return the first two hex values
			return fields[0], fields[1], nil
		}
	}

	// If we reached here, it means no valid lines were found
	return "", "", fmt.Errorf("no valid hex values found in the file")
}

func buildFrameBufferNode() (*dt.Node, error) {
	// Open the framebuffer device
	fb, err := os.Open(sysfsFbPath)
	if err != nil {
		return nil, ErrFBOpenFileFailed
	}

	// Get variable screen info
	var vinfo FbVarScreenInfo
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fb.Fd(), FbIotclVscreeninfo, uintptr(unsafe.Pointer(&vinfo)))
	if errno != 0 {
		return nil, ErrFBGetVscreenInfoFailed
	}

	// Get fixed screen info
	var finfo FbFixScreenInfo
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fb.Fd(), FbIoctlGetFscreenInfo, uintptr(unsafe.Pointer(&finfo)))
	if errno != 0 {
		return nil, ErrFBGetFscreenInfoFailed
	}

	defer fb.Close()

	format := "a8r8g8b8"
	if vinfo.Red.Offset == 16 && vinfo.Green.Offset == 8 && vinfo.Blue.Offset == 0 {
		format = "a8b8g8r8"
	}

	var base, limit uint32

	if finfo.SmemStart == 0 {
		// Try to get base and limit from resource device node
		filePath := "/sys/class/graphics/fb0/device/resource"
		baseStr, limitStr, err := fetchResourceRange(filePath)
		if err != nil {
			return nil, ErrFBGetFscreenInfoFailed
		}

		baseTmp, _ := strconv.ParseUint(baseStr[2:], 16, 64)
		limitTmp, _ := strconv.ParseUint(limitStr[2:], 16, 64)

		base = parseUint64ToUint32(baseTmp)
		limit = parseUint64ToUint32(limitTmp - baseTmp)
	} else {
		base = uint32(finfo.SmemStart)
		limit = finfo.SmemLen
	}

	framebufferNodeName := fmt.Sprintf("framebuffer@0x%x", base)
	return dt.NewNode(framebufferNodeName, dt.WithProperty(
		dt.PropertyU32Array("reg", []uint32{base, limit}),
		dt.PropertyU32("width", vinfo.Xres),
		dt.PropertyU32("height", vinfo.Yres),
		dt.PropertyString("format", format),
		dt.PropertyU32("pixelsperscanline", finfo.LineLength/((vinfo.BitsPerPixel+7)/8)),
	)), nil
}

// readHexValue reads a hexadecimal value from a sysfs file
func readHexValue(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// GetDisplayDeviceInfo retrieves information about the display device from sysfs
func GetDisplayDeviceInfo() ([]map[string]string, error) {
	var devices []map[string]string

	// List all devices in /sys/class/drm/
	drmDevices, err := os.ReadDir(sysfsDrmPath)
	if err != nil {
		return nil, ErrGfxOpenFileFailed
	}

	for _, dev := range drmDevices {
		deviceName := dev.Name()

		// There exsits device nodes like 'version', skip this kind of device nodes
		info, _ := os.Stat(filepath.Join(sysfsDrmPath, deviceName))
		if !(info.IsDir()) {
			continue
		}

		// Skip any device that doesn't have a "device" folder (not a PCI device)
		deviceDir := filepath.Join(sysfsDrmPath, deviceName, "device")
		if _, err = os.Stat(deviceDir); os.IsNotExist(err) {
			continue
		}

		// Check whether device node 'vendor' exists, skip this directory if 'vendor' node
		// does not exist.
		vendorPath := filepath.Join(deviceDir, "vendor")
		if _, err := os.Stat(vendorPath); os.IsNotExist(err) {
			continue
		}

		// Read device details from the PCI device folder
		vendorID, err := readHexValue(filepath.Join(deviceDir, "vendor"))
		if err != nil {
			return nil, ErrGfxReadVendorIDFailed
		}
		deviceID, err := readHexValue(filepath.Join(deviceDir, "device"))
		if err != nil {
			return nil, ErrGfxReadDeviceIDFailed
		}
		revisionID, err := readHexValue(filepath.Join(deviceDir, "revision"))
		if err != nil {
			return nil, ErrGfxReadRevisionIDFailed
		}
		subsystemVendorID, err := readHexValue(filepath.Join(deviceDir, "subsystem_vendor"))
		if err != nil {
			return nil, ErrGfxReadSubSysVendorIDFailed
		}
		subsystemID, err := readHexValue(filepath.Join(deviceDir, "subsystem_device"))
		if err != nil {
			return nil, ErrGfxReadSubSysDeviceIDFailed
		}

		// Collect device info in a map
		deviceInfo := map[string]string{
			"name":                deviceName,
			"vendor-id":           vendorID,
			"device-id":           deviceID,
			"revision-id":         revisionID,
			"subsystem-vendor-id": subsystemVendorID,
			"subsystem-id":        subsystemID,
		}
		devices = append(devices, deviceInfo)
	}

	return devices, nil
}

func buildGraphicNode() (*dt.Node, error) {
	// Retrieve the display device information
	devices, err := GetDisplayDeviceInfo()
	if err != nil {
		return nil, err
	}

	if len(devices) == 0 {
		return nil, ErrGfxNoDeviceInfoFound
	}

	// Print out the device information
	device := devices[0]

	vendorID, _ := strconv.ParseUint(device["vendor-id"][2:], 16, 64)
	deviceID, _ := strconv.ParseUint(device["device-id"][2:], 16, 64)
	revisionID, _ := strconv.ParseUint(device["revision-id"][2:], 16, 64)
	subVendorID, _ := strconv.ParseUint(device["subsystem-vendor-id"][2:], 16, 64)
	subsysID, _ := strconv.ParseUint(device["subsystem-id"][2:], 16, 64)

	return dt.NewNode("Gma", dt.WithProperty(
		dt.PropertyU32("vendor-id", parseUint64ToUint32(vendorID)),
		dt.PropertyU32("device-id", parseUint64ToUint32(deviceID)),
		dt.PropertyU32("revision-id", parseUint64ToUint32(revisionID)),
		dt.PropertyU32("subsystem-vendor-id", parseUint64ToUint32(subVendorID)),
		dt.PropertyU32("subsystem-id", parseUint64ToUint32(subsysID)),
	)), nil
}

func constructSerialPortNode() *dt.Node {
	// Serial port settings
	var isIOPort uint32 = 0x1
	var baudRate uint32 = UniversalPayloadSerialPortBaudRate
	var regBase uint32 = UniversalPayloadSerialPortRegisterBase

	return dt.NewNode("serial@", dt.WithProperty(
		dt.PropertyString("compatible", "isa"),
		dt.PropertyU32("current-speed", baudRate),
		dt.PropertyU32Array("reg", []uint32{isIOPort, regBase}),
	))
}

func constructOptionNode(loadAddr uint64) (*dt.Node, error) {
	phyAddrSize, err := getPhysicalAddressSizes()
	if err != nil {
		return nil, err
	}

	return dt.NewNode("options", dt.WithChildren(
		dt.NewNode("upl-images@", dt.WithProperty(
			dt.PropertyU64("addr", loadAddr+uplImageOffset),
		)),
		dt.NewNode("upl-params", dt.WithProperty(
			dt.PropertyU32("addr-width", uint32(phyAddrSize)),
			dt.PropertyU32("pci-enum-done", 1),
			dt.PropertyString("boot-mode", "normal"),
		)),
		dt.NewNode("upl-custom", dt.WithProperty(
			dt.PropertyU64("hoblistptr", loadAddr+tmpHobOffset),
		)),
	)), nil
}

func constructSMBIOS3Node() (*dt.Node, error) {
	smbiosTableBase, size, err := getSMBIOSBase()

	// According to EDK2 UPL implementation, only SMBIOS3 is supported in FDT.
	if (err != nil) || (size != getSMBIOS3HdrSize()) {
		return nil, errors.Join(ErrSMBIOS3NotFound, err)
	}

	return dt.NewNode("smbios", dt.WithProperty(
		dt.PropertyString("compatible", "smbios"),
		dt.PropertyRegion("reg", uint64(smbiosTableBase), uint64(pageSize)),
	)), nil
}

func constructReservedMemoryNode(rsdpBase uint64) *dt.Node {
	var rsvdNodes []*dt.Node

	acpiChildNode := dt.NewNode("acpi", dt.WithProperty(
		dt.PropertyString("compatible", "acpi"),
		dt.PropertyRegion("reg", rsdpBase, uint64(pageSize)),
	))

	rsvdNodes = append(rsvdNodes, acpiChildNode)

	if smbios3ChildNode, err := constructSMBIOS3Node(); err != nil {
		// If we failed to retrieve SMBIOS3 information, prompt error
		// message to indicate error message, and continue construct DTB.
		warningMsg = append(warningMsg, err)
	} else {
		rsvdNodes = append(rsvdNodes, smbios3ChildNode)
	}

	return dt.NewNode("reserved-memory", dt.WithChildren(rsvdNodes...))
}

func retrieveRootBridgeResources(path string, item MCFGBaseAddressAllocation) (*ResourceRegions, error) {
	domainIDHex := fmt.Sprintf("%04x", item.PCISegGrp)

	var MMIO64Base uint64 = PCIMMIO64InvalidBase
	var MMIO32Base uint64 = PCIMMIO32InvalidBase
	var IOPortBase uint64 = PCIIOPortInvalidBase
	var mmio64End uint64
	var mmio32End uint64
	var ioPortEnd uint64

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		deviceName := filepath.Base(path)
		parts := strings.Split(deviceName, ":")
		// deviceName fetched from filepath can be separated into 3 parts:
		// 0000:00:00.0 which is DOMAIN_ID:BUS_ID:DEVICE_ID:FUNCTION_ID
		// To retrieve the memory resource regions for 64-bit/32-bit MMIO
		// and IO, we need to ensure:
		// 1. Domain ID matches
		// 2. Bus ID is valid
		if len(parts) != 3 || parts[0] != domainIDHex {
			// Skip unmatched Bus number
			return nil
		}

		if bus, err := strconv.ParseUint(parts[1], 16, 64); err != nil {
			// Should not happen, if failed to parse Bus number, return error directly
			return err
		} else if (bus >= uint64(item.StartBus)) && (bus <= uint64(item.EndBus)) {
			resourcePath := filepath.Join(path, "resource")
			resources, err := retrieveDeviceResources(resourcePath)
			if err != nil {
				return nil // Continue scanning other devices
			}
			for _, res := range resources {
				if base, err := strconv.ParseUint(res.BaseAddress, 0, 64); err != nil {
					// Should not happen, if failed to parse uint, skip this region
					continue
				} else if end, err := strconv.ParseUint(res.EndAddress, 0, 64); err != nil {
					// Should not happen, if failed to parse uint, skip this region
					continue
				} else {
					// Region found, merge it to domain resource region
					switch res.Type {
					case PCIMMIO64Type:
						MMIO64Base = min(base, MMIO64Base)
						mmio64End = max(end, mmio64End)
					case PCIMMIO32Type:
						MMIO32Base = min(base, MMIO32Base)
						mmio32End = max(end, mmio32End)
					case PCIIOPortType:
						IOPortBase = min(base, IOPortBase)
						ioPortEnd = max(end, ioPortEnd)
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &ResourceRegions{
		MMIO64Base:  MMIO64Base,
		MMIO64Limit: mmio64End - MMIO64Base + 1,
		MMIO32Base:  MMIO32Base,
		MMIO32Limit: mmio32End - MMIO32Base + 1,
		IOPortBase:  IOPortBase,
		IOPortLimit: ioPortEnd - IOPortBase + 1,
	}, nil
}

func retrieveDeviceResources(resourcePath string) ([]ResourceInfo, error) {
	contentBytes, err := os.ReadFile(resourcePath)
	if err != nil {
		return nil, err
	}

	content := string(contentBytes)
	lines := strings.Split(content, "\n")
	var resources []ResourceInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")

		if len(parts) == 3 {
			base := strings.TrimSpace(parts[0])
			end := strings.TrimSpace(parts[1])
			attr := strings.TrimSpace(parts[2])

			if attrInt, err := strconv.ParseUint(attr, 0, 64); err != nil {
				// Should not happen, skip this line in case it happens
				continue
			} else {
				var resourceType string
				if attrInt&PCIMMIOReadOnly != 0 {
					// Skip ReadOnly MMIO, this is ROM region
					continue
				}

				if attrInt&PCIMMIO64Attr == PCIMMIO64Attr {
					base64, err := strconv.ParseUint(base, 0, 64)
					if err == nil {
						// Some platform provides 64-bit MMIO with all zero in high 32 bits,
						// it leads to huge MMIO region for 64-bit, and triggers assertation
						// in EDK2 since it is actual a 32-bit address, not a 64-bit address.
						//
						// Skip this scenario to address above issue.
						if base64>>32 == 0 {
							continue
						}
					}
					resourceType = PCIMMIO64Type
				} else if attrInt&PCIMMIO32Attr == PCIMMIO32Attr {
					resourceType = PCIMMIO32Type
				} else if attrInt&PCIIOPortAttr == PCIIOPortAttr {
					// Do not care about IRQ specific bits (BITS 0~5)
					resourceType = PCIIOPortType
				} else {
					continue // Skip unknown resource types
				}

				resources = append(resources, ResourceInfo{
					Type:        resourceType,
					BaseAddress: base,
					EndAddress:  end,
					Attributes:  attr,
				})
			}
		}
	}
	return resources, nil
}

func fetchACPIMCFGData(data []byte) ([]MCFGBaseAddressAllocation, error) {
	var mcfgDataArray []MCFGBaseAddressAllocation

	// Check if the data is long enough to contain data from offset 0x2c.
	if len(data) <= ACPIMCFGBaseAddressAllocationOffset {
		return nil, ErrMcfgDataLenthTooShort
	}

	// Check if the magic word is "MCFG".
	if !bytes.Equal(data[:4], []byte(ACPIMCFGSignature)) {
		return nil, ErrMcfgSignatureMismatch
	}

	segInfoContent := data[ACPIMCFGBaseAddressAllocationOffset:]

	// Check whether content in Base Address Allocation Structure is valid
	if len(segInfoContent)%ACPIMCFGBaseAddressAllocationLenth != 0 {
		return nil, ErrMcfgBaseAddrAllocCorrupt
	}

	for i := 0; i < len(segInfoContent); i += ACPIMCFGBaseAddressAllocationLenth {
		mcfgDataBytes := segInfoContent[i : i+ACPIMCFGBaseAddressAllocationLenth]
		mcfgData := MCFGBaseAddressAllocation{}
		reader := bytes.NewReader(mcfgDataBytes)

		err := binary.Read(reader, binary.LittleEndian, &mcfgData)
		if err != nil {
			return nil, ErrMcfgBaseAddrAllocDecode
		}

		mcfgDataArray = append(mcfgDataArray, mcfgData)
	}
	return mcfgDataArray, nil
}

func createPCIRootBridgeNode(path string, item MCFGBaseAddressAllocation) (*dt.Node, error) {
	high64 := func(val uint64) uint32 {
		return uint32(val >> 32)
	}

	low64 := func(val uint64) uint32 {
		return uint32(val & 0x0000_0000_FFFF_FFFF)
	}

	resource, err := retrieveRootBridgeResources(path, item)
	if err != nil {
		return nil, err
	}

	return dt.NewNode("pci-rb", dt.WithProperty(
		dt.PropertyString("compatible", "pci-rb"),
		dt.PropertyU64("reg", uint64(item.BaseAddr)),
		dt.PropertyU32Array("bus-range", []uint32{uint32(item.StartBus), uint32(item.EndBus)}),
		dt.PropertyU32Array("ranges", []uint32{
			0x300_0000, // 64BITS
			high64(resource.MMIO64Base), low64(resource.MMIO64Base),
			0x0, 0x0,
			high64(resource.MMIO64Limit), low64(resource.MMIO64Limit),
			0x200_0000, // 32BITS
			high64(resource.MMIO32Base), low64(resource.MMIO32Base),
			0x0, 0x0,
			high64(resource.MMIO32Limit), low64(resource.MMIO32Limit),
			0x100_0000, // IO
			high64(resource.IOPortBase), low64(resource.IOPortBase),
			0x0, 0x0,
			high64(resource.IOPortLimit), low64(resource.IOPortLimit),
		}),
	)), nil
}

func constructPCIRootBridgeNodes() ([]*dt.Node, error) {
	var rbNodes []*dt.Node

	fileData, err := os.ReadFile(ACPIMCFGSysFilePath)
	if err != nil {
		return nil, err
	}

	mcfgData, err := fetchACPIMCFGData(fileData)
	if err != nil {
		return nil, err
	}

	for _, item := range mcfgData {
		rbNode, err := createPCIRootBridgeNode(PCISearchPath, item)
		if err != nil {
			return nil, err
		}

		rbNodes = append(rbNodes, rbNode)

	}
	return rbNodes, nil
}

func buildDeviceTreeInfo(buf io.Writer, mem *kexec.Memory, loadAddr uint64, rsdpBase uint64) error {
	memNodes := buildDtMemoryNode(mem)

	rsvdMemNode := constructReservedMemoryNode(rsdpBase)

	optionsNode, err := constructOptionNode(loadAddr)
	if err != nil {
		// Break here if failed to construct option node since option node
		// is required to boot UPL.
		return err
	}

	serialPortNode := constructSerialPortNode()

	dtNodes := append(memNodes, rsvdMemNode)
	dtNodes = append(dtNodes, optionsNode)
	dtNodes = append(dtNodes, serialPortNode)

	if gmaNode, err := buildGraphicNode(); err != nil {
		// If we failed to retrieve Graphic configurations, prompt error
		// message to indicate error message, and continue construct DTB.
		warningMsg = append(warningMsg, err)
	} else {
		dtNodes = append(dtNodes, gmaNode)
	}

	if fbNode, err := buildFrameBufferNode(); err != nil {
		// If we failed to retrieve Frame Buffer configurations, prompt error
		// message to indicate error message, and continue construct DTB.
		warningMsg = append(warningMsg, err)
	} else {
		dtNodes = append(dtNodes, fbNode)

	}

	if pciRbNodes, err := constructPCIRootBridgeNodes(); err != nil {
		// If we failed to construct PCI Root Bridge info, prompt error
		// message to indicate error message, and continue construct DTB.
		// In this case, allows UPL to scan PCI Bus itself.
		warningMsg = append(warningMsg, err)
	} else {
		if pciRbNodes != nil {
			dtNodes = append(dtNodes, pciRbNodes...)
		}
	}

	dtHeader := dt.Header{
		Magic:           dt.Magic,
		TotalSize:       0x1000,
		OffDtStruct:     uint32(unsafe.Sizeof(dt.Header{})),
		OffMemRsvmap:    0x30,
		Version:         currentVersion,
		LastCompVersion: lastCompVersion,
		BootCpuidPhys:   bootPhysicalCPUID,
		//SizeDtStruct: 0x310,
	}

	dtRootNode := dt.NewNode("/", dt.WithChildren(dtNodes...))

	fdt := &dt.FDT{
		Header:   dtHeader,
		RootNode: dtRootNode,
	}

	// Write the FDT to the provided io.Writer
	_, err = fdt.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write FDT: %w", err)
	}

	return nil
}

func mockCPUTempInfoFile(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	tempFile, err := os.CreateTemp(tmpDir, "cpuinfo")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	sysfsCPUInfoPath = tempFile.Name()

	if _, err := tempFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	tempFile.Close()
	return tempFile.Name()
}
