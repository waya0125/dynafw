// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

//go:build !windows

package extract

import (
	"fmt"
	"time"
)

// FromDawin1008 は Windows 専用機能のため非 Windows 環境ではエラーを返す。
func FromDawin1008(exePath string, timeout time.Duration) ([]byte, error) {
	return nil, fmt.Errorf("extract サブコマンドは Windows 専用です（DAWIN DTV.exe の実行時メモリから抽出するため）")
}
