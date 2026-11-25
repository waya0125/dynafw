// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

// dynafw はダイナコネクティブ DY-UD200（1c11:1004）用のクロスプラットフォーム
// ファームウェア管理ツール（抽出・検証・パッチ適用・書き込み）です。
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/waya0125/dynafw/internal/dyud"
	"github.com/waya0125/dynafw/internal/extract"
	"github.com/waya0125/dynafw/internal/fw"
)

const rootUsage = `使い方: dynafw <コマンド> [オプション] [引数...]

コマンド:
  info      接続中の DY-UD200 のファームウェア版・シリアルを表示
  verify    ファームウェアバイナリのサイズ・ARMeb ベクタ・CRC32 を検証
  patch     純正 1.0.5-2 バイナリに Drop 低減 + XOR 無効化パッチを当てて 1.0.5-3 を生成
  extract   DAWIN DTV.exe (v1.0.0.8) の実行時メモリから純正 1.0.5-2 を抽出（Windows 専用）
  write     DY-UD200 本体へファームウェアを書き込む（中断厳禁・自己責任）

各コマンドの詳細は "dynafw <コマンド> -h" を参照してください。
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "エラー:", err)
		}
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, rootUsage)
		return errors.New("コマンドを指定してください")
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, rootUsage)
		return nil
	case "info":
		return cmdInfo(args[1:], stdout, stderr)
	case "verify":
		return cmdVerify(args[1:], stdout, stderr)
	case "patch":
		return cmdPatch(args[1:], stdout, stderr)
	case "extract":
		return cmdExtract(args[1:], stdout, stderr)
	case "write":
		return cmdWrite(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprint(stderr, rootUsage)
		return fmt.Errorf("不明なコマンド: %q", args[0])
	}
}

func cmdInfo(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dynafw info", flag.ContinueOnError)
	fs.SetOutput(stderr)
	devicePath := fs.String("device", "", "使用する機器パス（省略時は自動選択）")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dev, err := dyud.Open(*devicePath)
	if err != nil {
		return err
	}
	defer dev.Close()

	if err := dev.Init(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Device:   %s\n", dev.USB.Path())
	fmt.Fprintf(stdout, "Firmware: %s (%#08x)\n", dev.FW, dev.FW.Uint32())
	fmt.Fprintf(stdout, "Serial:   %s\n", dev.Serial)
	return nil
}

func cmdVerify(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dynafw verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("使い方: dynafw verify <firmware.bin>")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	info, err := fw.Verify(data)
	fmt.Fprintf(stdout, "File:   %s\n", fs.Arg(0))
	fmt.Fprintf(stdout, "Size:   %d bytes\n", info.Size)
	fmt.Fprintf(stdout, "CRC32:  %#08x\n", info.CRC32)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ARMeb:  OK\n")
	fmt.Fprintf(stdout, "Known:  %s\n", info.KnownName)
	return nil
}

func cmdPatch(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dynafw patch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("使い方: dynafw patch <input-1.0.5-2.bin> <output-1.0.5-3.bin>")
	}
	src, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	out, err := fw.Patch1052To1053(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(fs.Arg(1), out, 0o644); err != nil {
		return err
	}
	info, _ := fw.Verify(out)
	fmt.Fprintf(stdout, "パッチ適用完了: %s\n", fs.Arg(1))
	fmt.Fprintf(stdout, "Size:   %d bytes\n", info.Size)
	fmt.Fprintf(stdout, "CRC32:  %#08x (%s)\n", info.CRC32, info.KnownName)
	return nil
}

func cmdExtract(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dynafw extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	timeout := fs.Duration("timeout", 10*time.Second, "Themida 展開待機の最大時間")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("使い方: dynafw extract [--timeout 10s] <DAWIN DTV.exe> <output-1.0.5-2.bin>")
	}
	data, err := extract.FromDawin1008(fs.Arg(0), *timeout)
	if err != nil {
		return err
	}
	if err := os.WriteFile(fs.Arg(1), data, 0o644); err != nil {
		return err
	}
	info, _ := fw.Verify(data)
	fmt.Fprintf(stdout, "抽出完了: %s\n", fs.Arg(1))
	fmt.Fprintf(stdout, "Size:   %d bytes\n", info.Size)
	fmt.Fprintf(stdout, "CRC32:  %#08x (%s)\n", info.CRC32, info.KnownName)
	return nil
}

func cmdWrite(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dynafw write", flag.ContinueOnError)
	fs.SetOutput(stderr)
	devicePath := fs.String("device", "", "使用する機器パス（省略時は自動選択）")
	yes := fs.Bool("yes", false, "確認プロンプトを省略して書き込む")
	force := fs.Bool("force", false, "未登録の CRC32 でも書き込みを許可する")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("使い方: dynafw write [--device PATH] [--yes] [--force] <firmware.bin>")
	}

	imagePath := fs.Arg(0)
	image, err := os.ReadFile(imagePath)
	if err != nil {
		return err
	}
	info, err := fw.Verify(image)
	if err != nil {
		return fmt.Errorf("ファームウェア検証失敗: %w", err)
	}
	if info.CRC32 != fw.CRC1052 && info.CRC32 != fw.CRC1053 && !*force {
		return fmt.Errorf("既知の FW (1.0.5-2 / 1.0.5-3) ではない CRC32 (%#08x) です。意図したカスタム FW であれば --force を指定してください", info.CRC32)
	}

	dev, err := dyud.Open(*devicePath)
	if err != nil {
		return err
	}
	defer dev.Close()

	if err := dev.Init(); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "対象機器:       %s (Serial: %s)\n", dev.USB.Path(), dev.Serial)
	fmt.Fprintf(stdout, "現在の FW:      %s\n", dev.FW)
	fmt.Fprintf(stdout, "書き込みファイル: %s\n", imagePath)
	fmt.Fprintf(stdout, "サイズ / CRC32: %d bytes / %#08x (%s)\n", info.Size, info.CRC32, info.KnownName)

	if !*yes {
		fmt.Fprint(stdout, "\n警告: 書き込み中の USB 切断や電源断は機器の破損（文鎮化）につながります。\n")
		fmt.Fprint(stdout, "続行するには \"yes\" と入力してください: ")
		sc := bufio.NewScanner(stdin)
		if !sc.Scan() || strings.TrimSpace(sc.Text()) != "yes" {
			return errors.New("キャンセルしました")
		}
	}

	// 書き込み中の誤操作 (Ctrl+C 等) による中断を防ぐ
	signal.Ignore(syscall.SIGINT, syscall.SIGTERM)

	fmt.Fprintln(stdout, "書き込みを開始します（完了まで USB を抜かないでください）...")
	lastPct := -1
	err = dev.WriteFirmware(image, func(sent, total int) {
		pct := sent * 100 / total
		if pct/10 != lastPct/10 || sent == total {
			lastPct = pct
			fmt.Fprintf(stdout, "  転送中: %3d%% (%d / %d bytes)\n", pct, sent, total)
		}
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "書き込みが完了しました。数秒後に自動再認識されます（認識しない場合は USB を抜き差ししてください）。その後 dynafw info で新しいバージョンを確認してください。")
	return nil
}
