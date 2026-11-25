// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

//go:build linux

package usb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"unsafe"
)

const (
	iocNone  = 0
	iocWrite = 1
	iocRead  = 2
)

func ioc(dir, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | uintptr('U')<<8 | nr
}

type bulkTransfer struct {
	endpoint uint32
	length   uint32
	timeout  uint32
	data     uintptr
}

var (
	ioctlBulk             = ioc(iocRead|iocWrite, 2, unsafe.Sizeof(bulkTransfer{}))
	ioctlClaimInterface   = ioc(iocRead, 15, unsafe.Sizeof(uint32(0)))
	ioctlReleaseInterface = ioc(iocRead, 16, unsafe.Sizeof(uint32(0)))
)

func ioctl(fd int, request, arg uintptr) (uintptr, error) {
	for {
		r, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, arg)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return 0, errno
		}
		return r, nil
	}
}

// DeviceRoot は usbfs の機器ファイル配置先。
var DeviceRoot = "/dev/bus/usb"

// Find は指定した VID/PID を持つ USB 機器パスの昇順一覧を返す。
func Find(id ID) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(DeviceRoot, "[0-9][0-9][0-9]", "[0-9][0-9][0-9]"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var found []string
	for _, p := range paths {
		got, err := readID(p)
		if err == nil && got == id {
			found = append(found, p)
		}
	}
	return found, nil
}

func readID(path string) (ID, error) {
	f, err := os.Open(path)
	if err != nil {
		return ID{}, err
	}
	defer f.Close()
	var desc [18]byte
	if n, err := f.Read(desc[:]); err != nil || n < len(desc) {
		return ID{}, fmt.Errorf("%s: 機器記述子を読めない", path)
	}
	return ID{
		Vendor:  uint16(desc[8]) | uint16(desc[9])<<8,
		Product: uint16(desc[10]) | uint16(desc[11])<<8,
	}, nil
}

type linuxDevice struct {
	path    string
	fd      int
	claimed bool
}

// Open は指定パス（空なら id に一致する最初の空き機器）を開き、Interface 0 を取得する。
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
			return nil, fmt.Errorf("USB デバイス %s が見つかりません", id)
		}
		candidates = found
	}

	var lastErr error
	for _, p := range candidates {
		fd, err := syscall.Open(p, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
		if err != nil {
			lastErr = fmt.Errorf("%s を開けません: %w", p, err)
			continue
		}
		var ifNum uint32 = 0
		if _, err := ioctl(fd, ioctlClaimInterface, uintptr(unsafe.Pointer(&ifNum))); err != nil {
			_ = syscall.Close(fd)
			if errors.Is(err, syscall.EBUSY) && path == "" {
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("%s のインターフェース 0 を取得できません: %w", p, err)
		}
		return &linuxDevice{path: p, fd: fd, claimed: true}, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("USB デバイス %s を開けません", id)
}

func (d *linuxDevice) Path() string { return d.path }

func (d *linuxDevice) bulk(ep uint8, data []byte, timeoutMS uint32) (int, error) {
	xfer := bulkTransfer{
		endpoint: uint32(ep),
		length:   uint32(len(data)),
		timeout:  timeoutMS,
	}
	if len(data) > 0 {
		xfer.data = uintptr(unsafe.Pointer(&data[0]))
	}
	n, err := ioctl(d.fd, ioctlBulk, uintptr(unsafe.Pointer(&xfer)))
	runtime.KeepAlive(data)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (d *linuxDevice) BulkWrite(ep uint8, data []byte, timeoutMS uint32) (int, error) {
	return d.bulk(ep&0x7f, data, timeoutMS)
}

func (d *linuxDevice) BulkRead(ep uint8, data []byte, timeoutMS uint32) (int, error) {
	return d.bulk(ep|0x80, data, timeoutMS)
}

func (d *linuxDevice) Close() error {
	if d.claimed {
		var ifNum uint32 = 0
		_, _ = ioctl(d.fd, ioctlReleaseInterface, uintptr(unsafe.Pointer(&ifNum)))
		d.claimed = false
	}
	return syscall.Close(d.fd)
}
