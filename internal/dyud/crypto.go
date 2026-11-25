// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

// Package dyud はダイナコネクティブ DY-UD200（1c11:1004）の制御コマンド暗号化と
// ファームウェア書き換えプロトコルを実装する。
package dyud

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

// defaultAESKey は DY-UD200 の制御コマンド初期化・応答復号に使われる AES-256 鍵。
var defaultAESKey = [32]byte{
	0x75, 0x93, 0xee, 0x38, 0x0d, 0x99, 0x44, 0x95,
	0x9f, 0xc7, 0x67, 0x2c, 0x59, 0xeb, 0x1f, 0x69,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// CmdCipher は DY-UD200 の USB 制御コマンド（132B 送信 / 128B 受信）の AES-ECB + CRC32 変換を行う。
type CmdCipher struct {
	defaultBlock cipher.Block // AES-256
	cmdBlock     cipher.Block // AES-128（初期化コマンドの [16:32] で更新される）
}

// NewCmdCipher は既定鍵で初期化した CmdCipher を返す。
func NewCmdCipher() *CmdCipher {
	def, err := aes.NewCipher(defaultAESKey[:])
	if err != nil {
		panic(err)
	}
	cmd, err := aes.NewCipher(defaultAESKey[:16])
	if err != nil {
		panic(err)
	}
	return &CmdCipher{
		defaultBlock: def,
		cmdBlock:     cmd,
	}
}

// EncryptCommand はコマンド列（最大 128 バイト）を 132 バイトの暗号化フレームに変換する。
func (c *CmdCipher) EncryptCommand(cmd []byte) ([132]byte, error) {
	var plain, enc [132]byte
	if len(cmd) == 0 || len(cmd) > 128 {
		return enc, fmt.Errorf("不正なコマンド長: %d", len(cmd))
	}
	copy(plain[:], cmd)
	crc := crc32.ChecksumIEEE(plain[:128])
	binary.BigEndian.PutUint32(plain[128:132], crc)

	var blk cipher.Block
	if plain[0]&0x10 != 0 {
		blk = c.cmdBlock
	} else {
		blk = c.defaultBlock
		next, err := aes.NewCipher(plain[16:32])
		if err != nil {
			return enc, err
		}
		c.cmdBlock = next
	}

	enc[0], enc[1] = plain[0], plain[1]
	enc[130], enc[131] = plain[130], plain[131]
	for off := 2; off < 130; off += aes.BlockSize {
		blk.Encrypt(enc[off:off+aes.BlockSize], plain[off:off+aes.BlockSize])
	}
	return enc, nil
}

// DecryptResponse は 128 バイトの応答フレームを既定 AES-256 鍵で復号し、CRC32 を検証する。
func (c *CmdCipher) DecryptResponse(enc []byte) ([128]byte, error) {
	var plain [128]byte
	if len(enc) != 128 {
		return plain, fmt.Errorf("応答長が 128 バイトではない: %d", len(enc))
	}
	for off := 0; off < 128; off += aes.BlockSize {
		c.defaultBlock.Decrypt(plain[off:off+aes.BlockSize], enc[off:off+aes.BlockSize])
	}
	want := binary.BigEndian.Uint32(plain[124:128])
	got := crc32.ChecksumIEEE(plain[:124])
	if got != want {
		return plain, fmt.Errorf("応答 CRC32 不一致: got=%#08x want=%#08x", got, want)
	}
	return plain, nil
}
