// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

// Package fw は DY-UD200 ファームウェアバイナリの検証とパッチ適用を行う。
package fw

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	// Size105 は FW 1.0.5 系（1.0.5-2 / 1.0.5-3）の正確なファイルサイズ（272,780 バイト）。
	Size105 = 272780

	// CRC1052 は純正 FW 1.0.5-2 の IEEE CRC32。
	CRC1052 uint32 = 0x1282e9b8

	// CRC1053 は Drop 低減＋XOR 無効化パッチ適用済み FW 1.0.5-3 の IEEE CRC32。
	CRC1053 uint32 = 0xe25ee536

	// MinSize / MaxSize は安全チェック用の許容範囲（搭載フラッシュ S29AL008D は 1 MiB）。
	MinSize = 128 * 1024
	MaxSize = 512 * 1024
)

// Info はファームウェアバイナリの検証結果。
type Info struct {
	Size       int
	CRC32      uint32
	KnownName  string
	ValidARMeb bool
}

// Verify はファームウェアバイナリのサイズ・ARMeb 例外ベクタ・CRC32 を検証する。
func Verify(data []byte) (Info, error) {
	info := Info{
		Size:  len(data),
		CRC32: crc32.ChecksumIEEE(data),
	}

	if len(data) < MinSize || len(data) > MaxSize {
		return info, fmt.Errorf("ファイルサイズが想定範囲外です: %d bytes（想定: %d〜%d bytes）", len(data), MinSize, MaxSize)
	}

	// ARM ビッグエンディアン例外ベクタの先頭命令を確認:
	// 0x00: 0xEAxxxxxx (B reset)
	// 0x04..0x1C: 0xE59FF0xx (LDR PC, [PC, #imm])
	w0 := binary.BigEndian.Uint32(data[0:4])
	w1 := binary.BigEndian.Uint32(data[4:8])
	info.ValidARMeb = (w0&0xff000000) == 0xea000000 && (w1&0xfffff000) == 0xe59ff000
	if !info.ValidARMeb {
		return info, fmt.Errorf("ARMeb 例外ベクタが一致しません（先頭 8 バイト: %x）。DY-UD200 用ファームウェアではない可能性があります", data[:8])
	}

	switch info.CRC32 {
	case CRC1052:
		if len(data) == Size105 {
			info.KnownName = "1.0.5-2 (純正)"
		}
	case CRC1053:
		if len(data) == Size105 {
			info.KnownName = "1.0.5-3 (Drop低減 + XOR無効化パッチ適用済み)"
		}
	default:
		info.KnownName = "未登録 / カスタム"
	}

	return info, nil
}

// patchEntry は 1.0.5-2 -> 1.0.5-3 の 1 箇所分の変更定義。
type patchEntry struct {
	offset uint32
	before []byte
	after  []byte
	desc   string
}

// patches1052To1053 は dyud_firmpatcher_1.0.5-2 (870 + 936 パッチ + バージョン変更) と完全一致する 7 箇所。
var patches1052To1053 = []patchEntry{
	{0x00d360, []byte{0xeb, 0x00, 0xb9, 0x7f}, []byte{0xe1, 0xa0, 0x00, 0x00}, "936パッチ: printf('X') を NOP 化"},
	{0x00dbc4, []byte{0xeb, 0x00, 0xb7, 0x66}, []byte{0xe1, 0xa0, 0x00, 0x00}, "936パッチ: printf('Smart Card CMD (%d)') を NOP 化"},
	{0x00dbe4, []byte{0xeb, 0x00, 0xb7, 0x5e}, []byte{0xe1, 0xa0, 0x00, 0x00}, "936パッチ: printf('RSC(%d)') を NOP 化"},
	{0x00deec, []byte{0xe3, 0xa0, 0x00, 0x02}, []byte{0xe3, 0xa0, 0x00, 0x03}, "バージョン表記: 1.0.5-2 -> 1.0.5-3"},
	{0x00e044, []byte{0xeb, 0x00, 0xb6, 0x46}, []byte{0xe1, 0xa0, 0x00, 0x00}, "936パッチ: printf('.') を NOP 化"},
	{0x00e2c0, []byte{0xe5, 0x97, 0x00, 0x18}, []byte{0xea, 0x00, 0x00, 0xf8}, "870パッチ #1: PAT/PMT解析+XOR暗号化ループをスキップ"},
	{0x00e6dc, []byte{0xe5, 0x9d, 0x00, 0x0c}, []byte{0xea, 0xff, 0xfe, 0xca}, "870パッチ #2: 端数処理(printf+XOR)をスキップ"},
}

// Patch1052To1053 は純正 FW 1.0.5-2 に対して Drop 低減＋XOR 無効化パッチを適用し、
// 1.0.5-3 バイナリを生成する。入力・各パッチ箇所・出力の CRC32 をすべて厳密に検証する。
func Patch1052To1053(src []byte) ([]byte, error) {
	if len(src) != Size105 {
		return nil, fmt.Errorf("入力サイズが 1.0.5-2 (%d bytes) と一致しません: %d bytes", Size105, len(src))
	}
	if got := crc32.ChecksumIEEE(src); got != CRC1052 {
		return nil, fmt.Errorf("入力 CRC32 が 1.0.5-2 (%#08x) と一致しません: %#08x", CRC1052, got)
	}

	out := make([]byte, len(src))
	copy(out, src)

	for _, p := range patches1052To1053 {
		cur := out[p.offset : p.offset+uint32(len(p.before))]
		if !bytes.Equal(cur, p.before) {
			return nil, fmt.Errorf("offset %#x (%s) の元バイトが不一致: got=%x want=%x", p.offset, p.desc, cur, p.before)
		}
		copy(cur, p.after)
	}

	if got := crc32.ChecksumIEEE(out); got != CRC1053 {
		return nil, fmt.Errorf("パッチ後の CRC32 が 1.0.5-3 (%#08x) と一致しません: %#08x", CRC1053, got)
	}
	return out, nil
}
