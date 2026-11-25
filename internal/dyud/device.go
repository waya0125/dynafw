// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

package dyud

import (
	"fmt"
	"strings"
	"time"

	"github.com/waya0125/dynafw/internal/usb"
)

// DeviceID は DY-UD200 の USB VID/PID。
var DeviceID = usb.ID{Vendor: 0x1c11, Product: 0x1004}

const (
	epCmdOut uint8 = 0x02
	epResIn  uint8 = 0x84

	defaultTimeoutMS uint32 = 2000
)

// FirmwareVersion は DY-UD200 のファームウェアバージョン（major.minor1.minor2-minor3）。
type FirmwareVersion struct {
	Major  byte
	Minor1 byte
	Minor2 byte
	Minor3 byte
}

func (f FirmwareVersion) String() string {
	return fmt.Sprintf("%d.%d.%d-%d", f.Major, f.Minor1, f.Minor2, f.Minor3)
}

func (f FirmwareVersion) Uint32() uint32 {
	return uint32(f.Major)<<24 | uint32(f.Minor1)<<16 | uint32(f.Minor2)<<8 | uint32(f.Minor3)
}

// NeedsNewInit は新ファーム（1.0.5-2 / 1.0.5-3 等）用の再初期化シーケンスが必要かを返す。
func (f FirmwareVersion) NeedsNewInit() bool {
	return f.Major == 1 && f.Minor1 == 0 && f.Minor2 == 5 && f.Minor3 >= 2
}

var (
	cmdInit = []byte{
		0x0f, 0xf0, 0xe0, 0x20, 0x00, 0x00, 0x00, 0x00, 0x23, 0x8d, 0xc0, 0xd0, 0x0e, 0xd0, 0x38, 0x75,
		0xe8, 0x38, 0xdf, 0xeb, 0x33, 0x7d, 0x39, 0x52, 0xe3, 0x78, 0xeb, 0xd1, 0xb7, 0x84, 0xef, 0x65,
		0x00, 0x9a,
	}
	cmdInitNew = []byte{
		0x0f, 0xf0, 0xe0, 0x20, 0x00, 0x00, 0x00, 0x00, 0x7b, 0x36, 0x51, 0xe5, 0x21, 0x7c, 0x7b, 0x1c,
		0xe8, 0x38, 0xdf, 0xeb, 0x33, 0x7d, 0x39, 0x52, 0xe3, 0x78, 0xeb, 0xd1, 0xb7, 0x84, 0xef, 0x65,
		0x00, 0x00,
	}
	cmdInitNF = []byte{0x1f, 0xf0, 0xe3, 0x04, 0x00, 0x00, 0x00, 0x00}
	cmdClose  = []byte{0x1f, 0xf0, 0xe2, 0x04, 0x00, 0x00, 0x00, 0x00}
	cmdFWVer  = []byte{0x1f, 0xf0, 0x9f, 0x04, 0x00, 0x00, 0x00, 0x00}
	cmdSerial = []byte{0x1f, 0xf0, 0x95, 0x04, 0x00, 0x00, 0x00, 0x00}
	cmdTSOff  = []byte{0x1f, 0xf0, 0xb5, 0x04, 0x00, 0x00, 0x00, 0x00}
)

// Device は開いた DY-UD200 機器。
type Device struct {
	USB    usb.Device
	FW     FirmwareVersion
	Serial string
	cipher *CmdCipher
}

// Open は指定デバイスパス（空なら最初の空き DY-UD200）を開く。
func Open(path string) (*Device, error) {
	u, err := usb.Open(DeviceID, path)
	if err != nil {
		return nil, err
	}
	return &Device{
		USB:    u,
		cipher: NewCmdCipher(),
	}, nil
}

// SendCommand はコマンドを暗号化して送信し、応答を復号・CRC 検証して先頭 resLen バイトを返す。
func (d *Device) SendCommand(cmd []byte, resLen int) ([]byte, error) {
	return d.SendCommandTimeout(cmd, resLen, defaultTimeoutMS)
}

// SendCommandTimeout は指定タイムアウト（ミリ秒）でコマンドを送受信する。
func (d *Device) SendCommandTimeout(cmd []byte, resLen int, timeoutMS uint32) ([]byte, error) {
	enc, err := d.cipher.EncryptCommand(cmd)
	if err != nil {
		return nil, err
	}
	n, err := d.USB.BulkWrite(epCmdOut, enc[:], timeoutMS)
	if err != nil {
		return nil, fmt.Errorf("コマンド送信失敗: %w", err)
	}
	if n != len(enc) {
		return nil, fmt.Errorf("コマンド送信長不足: %d/%d", n, len(enc))
	}

	var resEnc [128]byte
	var recvErr error
	for retry := range 10 {
		rn, err := d.USB.BulkRead(epResIn, resEnc[:], timeoutMS)
		if err == nil && rn == len(resEnc) {
			recvErr = nil
			break
		}
		if err != nil {
			recvErr = err
		} else {
			recvErr = fmt.Errorf("応答受信長不足: %d/%d", rn, len(resEnc))
		}
		if retry < 9 {
			time.Sleep(time.Millisecond)
		}
	}
	if recvErr != nil {
		return nil, fmt.Errorf("応答受信失敗: %w", recvErr)
	}

	plain, err := d.cipher.DecryptResponse(resEnc[:])
	if err != nil {
		return nil, err
	}
	if resLen <= 0 || resLen > len(plain) {
		resLen = len(plain)
	}
	out := make([]byte, resLen)
	copy(out, plain[:resLen])
	return out, nil
}

// Init はデバイスを初期化し、現在の FW バージョンとシリアル番号を取得する。
func (d *Device) Init() error {
	if _, err := d.SendCommand(cmdInit, 128); err != nil {
		return fmt.Errorf("初期化コマンド失敗: %w", err)
	}
	if err := d.StopTS(); err != nil {
		return err
	}
	if _, err := d.SendCommand(cmdInitNF, 2); err != nil {
		return fmt.Errorf("新FW初期化コマンド失敗: %w", err)
	}
	fv, err := d.GetFirmwareVersion()
	if err != nil {
		return fmt.Errorf("FWバージョン取得失敗: %w", err)
	}
	d.FW = fv

	if fv.NeedsNewInit() {
		if _, err := d.SendCommand(cmdClose, 2); err != nil {
			return fmt.Errorf("再初期化前クローズ失敗: %w", err)
		}
		if _, err := d.SendCommand(cmdInitNew, 128); err != nil {
			return fmt.Errorf("新FW再初期化失敗: %w", err)
		}
		if err := d.StopTS(); err != nil {
			return err
		}
		if _, err := d.SendCommand(cmdInitNF, 2); err != nil {
			return fmt.Errorf("新FW再初期化(e3)失敗: %w", err)
		}
	}

	serial, err := d.GetSerial()
	if err != nil {
		return fmt.Errorf("シリアル取得失敗: %w", err)
	}
	d.Serial = serial
	return nil
}

// GetFirmwareVersion は FW バージョンを取得する。
func (d *Device) GetFirmwareVersion() (FirmwareVersion, error) {
	res, err := d.SendCommand(cmdFWVer, 6)
	if err != nil {
		return FirmwareVersion{}, err
	}
	return FirmwareVersion{
		Major:  res[2],
		Minor1: res[3],
		Minor2: res[4],
		Minor3: res[5],
	}, nil
}

// GetSerial は本体のシリアル番号を取得する。
func (d *Device) GetSerial() (string, error) {
	res, err := d.SendCommand(cmdSerial, 18)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 0, 16)
	for _, b := range res[2:18] {
		if b == 0 {
			break
		}
		if b >= 0x20 && b <= 0x7e {
			buf = append(buf, b)
		}
	}
	return strings.TrimSpace(string(buf)), nil
}

// StopTS は TS ストリーミングを停止する。
func (d *Device) StopTS() error {
	res, err := d.SendCommand(cmdTSOff, 1)
	if err != nil {
		return fmt.Errorf("StopTS 失敗: %w", err)
	}
	if res[0] != 0 {
		return fmt.Errorf("StopTS エラー: %#02x", res[0])
	}
	return nil
}

// Close はデバイスを閉じる。
func (d *Device) Close() error {
	if d.USB == nil {
		return nil
	}
	_ = d.StopTS()
	_, _ = d.SendCommand(cmdClose, 2)
	err := d.USB.Close()
	d.USB = nil
	return err
}
