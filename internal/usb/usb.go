// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

// Package usb は Linux（usbfs）と Windows（WinUSB）で共通の USB バルク転送抽象を提供する。
package usb

import "fmt"

// ID は USB のベンダー ID と製品 ID。
type ID struct {
	Vendor  uint16
	Product uint16
}

func (id ID) String() string {
	return fmt.Sprintf("%04x:%04x", id.Vendor, id.Product)
}

// Device は開いた USB デバイスへのバルク転送インターフェース。
type Device interface {
	Path() string
	BulkWrite(ep uint8, data []byte, timeoutMS uint32) (int, error)
	BulkRead(ep uint8, data []byte, timeoutMS uint32) (int, error)
	Close() error
}
