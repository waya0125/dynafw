// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

//go:build windows

package usb

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// GUID_DEVINTERFACE_USB_DEVICE = {A5DCBF10-6530-11D2-901F-00C04FB951ED}
var guidDevInterfaceUSBDevice = guid{
	Data1: 0xa5dcbf10,
	Data2: 0x6530,
	Data3: 0x11d2,
	Data4: [8]byte{0x90, 0x1f, 0x00, 0xc0, 0x4f, 0xb9, 0x51, 0xed},
}

const (
	digcfPresent         = 0x00000002
	digcfDeviceInterface = 0x00000010
	pipeTransferTimeout  = 0x03
	fileFlagOverlapped   = 0x40000000
)

type spDeviceInterfaceData struct {
	cbSize             uint32
	interfaceClassGuid guid
	flags              uint32
	reserved           uintptr
}

var (
	modSetupAPI                     = syscall.NewLazyDLL("setupapi.dll")
	procSetupDiGetClassDevsW        = modSetupAPI.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces = modSetupAPI.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDW = modSetupAPI.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfo    = modSetupAPI.NewProc("SetupDiDestroyDeviceInfoList")

	modWinUSB               = syscall.NewLazyDLL("winusb.dll")
	procWinUsbInitialize    = modWinUSB.NewProc("WinUsb_Initialize")
	procWinUsbFree          = modWinUSB.NewProc("WinUsb_Free")
	procWinUsbSetPipePolicy = modWinUSB.NewProc("WinUsb_SetPipePolicy")
	procWinUsbWritePipe     = modWinUSB.NewProc("WinUsb_WritePipe")
	procWinUsbReadPipe      = modWinUSB.NewProc("WinUsb_ReadPipe")
)

// Find は指定 VID/PID を持つ USB デバイスパス一覧を返す。
func Find(id ID) ([]string, error) {
	needle := fmt.Sprintf("vid_%04x&pid_%04x", id.Vendor, id.Product)

	hDevInfo, _, err := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&guidDevInterfaceUSBDevice)),
		0,
		0,
		digcfPresent|digcfDeviceInterface,
	)
	if hDevInfo == uintptr(syscall.InvalidHandle) {
		return nil, fmt.Errorf("SetupDiGetClassDevsW 失敗: %w", err)
	}
	defer procSetupDiDestroyDeviceInfo.Call(hDevInfo)

	// SP_DEVICE_INTERFACE_DETAIL_DATA_W の cbSize は 64bit で 8、32bit で 6。
	var detailCbSize uint32 = 6
	if unsafe.Sizeof(uintptr(0)) == 8 {
		detailCbSize = 8
	}

	var result []string
	var ifData spDeviceInterfaceData
	ifData.cbSize = uint32(unsafe.Sizeof(ifData))

	for idx := uint32(0); ; idx++ {
		r, _, _ := procSetupDiEnumDeviceInterfaces.Call(
			hDevInfo,
			0,
			uintptr(unsafe.Pointer(&guidDevInterfaceUSBDevice)),
			uintptr(idx),
			uintptr(unsafe.Pointer(&ifData)),
		)
		if r == 0 {
			break
		}

		var reqSize uint32
		procSetupDiGetDeviceInterfaceDW.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&ifData)),
			0,
			0,
			uintptr(unsafe.Pointer(&reqSize)),
			0,
		)
		if reqSize < 6 {
			continue
		}

		buf := make([]byte, reqSize)
		*(*uint32)(unsafe.Pointer(&buf[0])) = detailCbSize
		ok, _, _ := procSetupDiGetDeviceInterfaceDW.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&ifData)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(reqSize),
			0,
			0,
		)
		if ok == 0 {
			continue
		}

		u16Len := (len(buf) - 4) / 2
		u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[4])), u16Len)
		devPath := syscall.UTF16ToString(u16)
		if strings.Contains(strings.ToLower(devPath), needle) {
			result = append(result, devPath)
		}
	}

	sort.Strings(result)
	return result, nil
}

type winDevice struct {
	path   string
	handle syscall.Handle
	winusb uintptr
}

// Open は指定デバイスパス（空なら id に一致する最初の機器）を WinUSB で開く。
func Open(id ID, path string) (Device, error) {
	var candidates []string
	if path != "" {
		candidates = []string{path}
	} else {
		found, err := Find(id)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("USB デバイス %s が見つかりません（Zadig 等で WinUSB ドライバを割り当ててください）", id)
		}
		candidates = found
	}

	var lastErr error
	for _, p := range candidates {
		p16, err := syscall.UTF16PtrFromString(p)
		if err != nil {
			lastErr = err
			continue
		}
		h, err := syscall.CreateFile(
			p16,
			syscall.GENERIC_READ|syscall.GENERIC_WRITE,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
			nil,
			syscall.OPEN_EXISTING,
			syscall.FILE_ATTRIBUTE_NORMAL|fileFlagOverlapped,
			0,
		)
		if err != nil {
			lastErr = fmt.Errorf("CreateFileW(%s) 失敗: %w", p, err)
			continue
		}

		var wHandle uintptr
		r, _, wErr := procWinUsbInitialize.Call(uintptr(h), uintptr(unsafe.Pointer(&wHandle)))
		if r == 0 {
			_ = syscall.CloseHandle(h)
			lastErr = fmt.Errorf("WinUsb_Initialize(%s) 失敗（WinUSB ドライバが割り当てられているか確認してください）: %w", p, wErr)
			continue
		}
		return &winDevice{path: p, handle: h, winusb: wHandle}, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("USB デバイス %s を開けません", id)
}

func (d *winDevice) Path() string { return d.path }

func (d *winDevice) setTimeout(ep uint8, timeoutMS uint32) {
	t := timeoutMS
	procWinUsbSetPipePolicy.Call(
		d.winusb,
		uintptr(ep),
		pipeTransferTimeout,
		unsafe.Sizeof(t),
		uintptr(unsafe.Pointer(&t)),
	)
}

func (d *winDevice) BulkWrite(ep uint8, data []byte, timeoutMS uint32) (int, error) {
	ep &= 0x7f
	d.setTimeout(ep, timeoutMS)
	var transferred uint32
	var ptr uintptr
	if len(data) > 0 {
		ptr = uintptr(unsafe.Pointer(&data[0]))
	}
	r, _, err := procWinUsbWritePipe.Call(
		d.winusb,
		uintptr(ep),
		ptr,
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&transferred)),
		0,
	)
	runtime.KeepAlive(data)
	if r == 0 {
		return int(transferred), fmt.Errorf("WinUsb_WritePipe 失敗: %w", err)
	}
	return int(transferred), nil
}

func (d *winDevice) BulkRead(ep uint8, data []byte, timeoutMS uint32) (int, error) {
	ep |= 0x80
	d.setTimeout(ep, timeoutMS)
	var transferred uint32
	var ptr uintptr
	if len(data) > 0 {
		ptr = uintptr(unsafe.Pointer(&data[0]))
	}
	r, _, err := procWinUsbReadPipe.Call(
		d.winusb,
		uintptr(ep),
		ptr,
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&transferred)),
		0,
	)
	runtime.KeepAlive(data)
	if r == 0 {
		return int(transferred), fmt.Errorf("WinUsb_ReadPipe 失敗: %w", err)
	}
	return int(transferred), nil
}

func (d *winDevice) Close() error {
	if d.winusb != 0 {
		procWinUsbFree.Call(d.winusb)
		d.winusb = 0
	}
	if d.handle != syscall.InvalidHandle && d.handle != 0 {
		err := syscall.CloseHandle(d.handle)
		d.handle = syscall.InvalidHandle
		return err
	}
	return nil
}
