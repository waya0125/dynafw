// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

package dyud

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	// FirmwarePageSize は 1 回の 0x91 コマンドで送信するファームウェアデータ長（108 バイト）。
	FirmwarePageSize = 108

	fwWriteTimeoutMS uint32 = 10000
	fwEndTimeoutMS   uint32 = 30000
)

// ProgressFunc は書き込み進捗通知コールバック（送信済みバイト数、総バイト数）。
type ProgressFunc func(sent, total int)

// WriteFirmware は BonDriver_dyud_firmware.cpp と同一の手順でファームウェアを書き込む。
//
// 手順:
//  1. 0xe2 (0x01) + 0x90 (サイズ) でアップデート開始
//  2. 0x91 で 108 バイトずつページ番号・サイズ・ページ CRC32 付きで転送
//  3. 0x92 (サイズ・全体 CRC32) で検証＆フラッシュ書き込み完了
//  4. 0xf2 リセットコマンド + StopTS
func (d *Device) WriteFirmware(image []byte, onProgress ProgressFunc) error {
	if len(image) == 0 {
		return fmt.Errorf("ファームウェアが空です")
	}
	totalSize := uint32(len(image))
	totalCRC := crc32.ChecksumIEEE(image)

	// 1. 事前コマンド（リセット系）+ アップデート開始コマンド (0x90)
	cmdPre := []byte{0x1f, 0xf0, 0xe2, 0x04, 0x01, 0x00, 0x00, 0x00}
	if _, err := d.SendCommandTimeout(cmdPre, 2, fwWriteTimeoutMS); err != nil {
		return fmt.Errorf("書き込み準備コマンド(0xe2)失敗: %w", err)
	}

	var cmdBegin [8]byte
	cmdBegin[0], cmdBegin[1], cmdBegin[2], cmdBegin[3] = 0x1f, 0xf0, 0x90, 0x04
	binary.BigEndian.PutUint32(cmdBegin[4:8], totalSize)
	res, err := d.SendCommandTimeout(cmdBegin[:], 4, fwWriteTimeoutMS)
	if err != nil {
		return fmt.Errorf("アップデート開始コマンド(0x90)失敗: %w", err)
	}
	if res[0] != 0 {
		return fmt.Errorf("アップデート開始拒否(0x90): status=%#02x", res[0])
	}

	// 2. 108 バイト単位でページ転送 (0x91)
	var page uint16
	for off := 0; off < len(image); off += FirmwarePageSize {
		end := min(off+FirmwarePageSize, len(image))
		chunk := image[off:end]

		var cmdPage [124]byte
		cmdPage[0], cmdPage[1], cmdPage[2], cmdPage[3] = 0x1f, 0xf0, 0x91, 0x78
		binary.BigEndian.PutUint32(cmdPage[4:8], totalSize)
		binary.BigEndian.PutUint16(cmdPage[8:10], page)
		binary.BigEndian.PutUint16(cmdPage[10:12], uint16(len(chunk)))
		binary.BigEndian.PutUint32(cmdPage[12:16], crc32.ChecksumIEEE(chunk))
		copy(cmdPage[16:], chunk)

		res, err := d.SendCommandTimeout(cmdPage[:], 4, fwWriteTimeoutMS)
		if err != nil {
			return fmt.Errorf("ページ %d (offset %#x) 送信失敗: %w", page, off, err)
		}
		if res[0] != 0 {
			return fmt.Errorf("ページ %d (offset %#x) 書き込みエラー: status=%#02x", page, off, res[0])
		}
		page++
		if onProgress != nil {
			onProgress(end, len(image))
		}
	}

	// 3. アップデート終了・全体 CRC32 検証 (0x92)
	var cmdEnd [124]byte
	cmdEnd[0], cmdEnd[1], cmdEnd[2], cmdEnd[3] = 0x1f, 0xf0, 0x92, 0x78
	binary.BigEndian.PutUint32(cmdEnd[4:8], totalSize)
	binary.BigEndian.PutUint32(cmdEnd[12:16], totalCRC)
	res, err = d.SendCommandTimeout(cmdEnd[:], 4, fwEndTimeoutMS)
	if err != nil {
		return fmt.Errorf("アップデート終了コマンド(0x92)失敗: %w", err)
	}
	if res[0] != 0 {
		return fmt.Errorf("アップデート終了検証エラー(0x92): status=%#02x", res[0])
	}

	// 4. 完了後リセット (0xf2) + StopTS
	cmdPost := []byte{0x1f, 0xf0, 0xf2, 0x04, 0x00, 0x00, 0x00, 0x00}
	_, _ = d.SendCommandTimeout(cmdPost, 2, fwWriteTimeoutMS)
	_ = d.StopTS()
	return nil
}
