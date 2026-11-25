// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

//go:build windows

// Package extract は Windows 上で DAWIN DTV.exe (v1.0.0.8) のプロセスメモリから
// 純正ファームウェア 1.0.5-2 (272,780 bytes, CRC32 0x1282E9B8) を抽出する。
package extract

import (
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"github.com/waya0125/dynafw/internal/fw"
)

const (
	dawinBaseAddr      uintptr = 0x00400000
	dawinFirmwareOff   uintptr = 0x00345914
	themidaPackedCRC32 uint32  = 0xe6f3aa27
	readChunkSize              = 4096
)

var (
	modKernel32           = syscall.NewLazyDLL("kernel32.dll")
	procReadProcessMemory = modKernel32.NewProc("ReadProcessMemory")
)

// FromDawin1008 は指定された DAWIN DTV.exe（v1.0.0.8）を起動し、Themida による
// メモリ展開完了後にファームウェア 1.0.5-2 を読み出して返す。
func FromDawin1008(exePath string, timeout time.Duration) ([]byte, error) {
	absExe, err := filepath.Abs(exePath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(absExe); err != nil {
		return nil, fmt.Errorf("実行ファイルが見つかりません: %w", err)
	}
	workDir := filepath.Dir(absExe)

	app16, err := syscall.UTF16PtrFromString(absExe)
	if err != nil {
		return nil, err
	}
	dir16, err := syscall.UTF16PtrFromString(workDir)
	if err != nil {
		return nil, err
	}

	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi syscall.ProcessInformation

	if err := syscall.CreateProcess(
		app16,
		nil,
		nil,
		nil,
		false,
		syscall.CREATE_NEW_PROCESS_GROUP,
		nil,
		dir16,
		&si,
		&pi,
	); err != nil {
		return nil, fmt.Errorf("DAWIN DTV.exe の起動に失敗しました: %w", err)
	}
	defer func() {
		_ = syscall.TerminateProcess(pi.Process, 0)
		_ = syscall.CloseHandle(pi.Thread)
		_ = syscall.CloseHandle(pi.Process)
	}()

	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	buf := make([]byte, fw.Size105)
	var lastCRC uint32

	for {
		if err := readProcessBytes(pi.Process, dawinBaseAddr+dawinFirmwareOff, buf); err == nil {
			lastCRC = crc32.ChecksumIEEE(buf)
			if lastCRC == fw.CRC1052 {
				return buf, nil
			}
		}
		if time.Now().After(deadline) {
			if lastCRC == themidaPackedCRC32 {
				return nil, fmt.Errorf("Themida の展開が完了しませんでした（CRC=%#08x）。vcredist_x86.exe（VC++ 2005 ランタイム）が入っているか、ローカルドライブ上に配置されているか確認してください", lastCRC)
			}
			return nil, fmt.Errorf("タイムアウト: 抽出 CRC=%#08x（期待値 %#08x）。DAWIN DTV.exe が v1.0.0.8 か確認してください", lastCRC, fw.CRC1052)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func readProcessBytes(proc syscall.Handle, addr uintptr, dst []byte) error {
	for off := 0; off < len(dst); off += readChunkSize {
		n := min(readChunkSize, len(dst)-off)
		var read uintptr
		r, _, err := procReadProcessMemory.Call(
			uintptr(proc),
			addr+uintptr(off),
			uintptr(unsafe.Pointer(&dst[off])),
			uintptr(n),
			uintptr(unsafe.Pointer(&read)),
		)
		if r == 0 || int(read) != n {
			return fmt.Errorf("ReadProcessMemory(offset=%#x) 失敗: %w", off, err)
		}
	}
	return nil
}
