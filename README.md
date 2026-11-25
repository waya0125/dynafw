# dynafw

ダイナコネクティブ（Dynaconnective）の USB 地デジチューナー **DY-UD200**（`1c11:1004`）用のクロスプラットフォーム（Linux / Windows）ファームウェア管理ツールです。  
Go 単一バイナリ（`CGO_ENABLED=0`）で動作し、Linux では `usbfs`、Windows では `WinUSB` を直接叩きます。

有志ツール群（`dyud_firm_extractor` / `dyud_firmpatcher_1.0.5-2` / `dyud_firmware_writer`）の機能を 1 つの CLI に整理・再実装しています。

## できること

| コマンド | 対応 OS | 内容 |
|---|---|---|
| `dynafw info` | Linux / Windows | 接続中の DY-UD200 の現在の FW バージョン・シリアル番号を確認 |
| `dynafw verify <fw.bin>` | Linux / Windows | サイズ・ARMeb 例外ベクタ・IEEE CRC32・既知 FW 判定 |
| `dynafw patch <1.0.5-2.bin> <1.0.5-3.bin>` | Linux / Windows | 純正 `1.0.5-2` に Drop 低減（936）＋XOR 無効化（870）パッチを適用して `1.0.5-3` を生成 |
| `dynafw extract <DAWIN DTV.exe> <1.0.5-2.bin>` | Windows 専用 | `DAWIN DTV.exe`（v1.0.0.8）の実行時メモリから純正 `1.0.5-2` を抽出し、完了後にプロセスを終了 |
| `dynafw write <fw.bin>` | Linux / Windows | DY-UD200 本体へファームウェアを書き込む（`1.0.5-2` への書き戻しも同じ手順） |

## ビルド

Go 1.24 以上が必要です。

```bash
# Linux (amd64)
CGO_ENABLED=0 go build -o bin/dynafw ./cmd/dynafw

# Windows (x64 / x86)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/dynafw-windows-amd64.exe ./cmd/dynafw
CGO_ENABLED=0 GOOS=windows GOARCH=386   go build -o dist/dynafw-windows-386.exe   ./cmd/dynafw
```

## 準備

- **Linux**: 一般ユーザーで操作する場合は `recdyna` と同じ udev ルール（`1c11:1004`）を配置します。
- **Windows**: `info` / `write` を使う場合は [Zadig](https://zadig.akeo.ie/) 等で `1c11:1004` に **WinUSB** ドライバを割り当てます（`extract` / `verify` / `patch` には不要）。

## 使い方

```bash
# 1. 純正 1.0.5-2 を抽出（Windows のみ、DAWIN DTV 1.0.0.8 + vcredist_x86 が必要）
dynafw extract "DAWIN DTV.exe" DY-UD200_firmware_1.0.5-2.bin

# 2. Drop 低減 + XOR 無効化パッチを当てて 1.0.5-3 を生成
dynafw patch DY-UD200_firmware_1.0.5-2.bin DY-UD200_firmware_1.0.5-3.bin

# 3. バイナリの検証
dynafw verify DY-UD200_firmware_1.0.5-3.bin

# 4. 実機の現在のバージョンを確認
dynafw info

# 5. 実機へ書き込み（純正 1.0.5-2 へ戻すときも同じコマンドで 1.0.5-2.bin を指定）
dynafw write DY-UD200_firmware_1.0.5-3.bin
```

## 注意事項

- ファームウェアの書き換え中に USB を抜いたり電源を切ったりすると、機器が起動不能（文鎮化）になるおそれがあります。安定した電源・USB ポートで実行してください。
- `1.0.5-3` には XOR 無効化パッチ（870 パッチ）が含まれるため、対応していない録画ソフトでは `--no-xor`（`NoXor=1`）相当の設定が必要です（最新の [recdyna](https://github.com/waya0125/recdyna) / [BonDriver_Dyna](https://github.com/waya0125/BonDriver_Dyna) は `1.0.5-3` を自動判定します）。

## 関連プロジェクト

- Linux 用録画コマンド: [recdyna](https://github.com/waya0125/recdyna)
- Windows 用 BonDriver: [BonDriver_Dyna](https://github.com/waya0125/BonDriver_Dyna)

## 出典・ライセンス

- **プロトコル・ツール参考**: 有志ドライバー **BonDriver_dyud v8.5.3**（`BonDriver_dyud_firmware.cpp` / `dyud_firmware_writer.cpp` / `dyud_firm_extractor.cpp` / `dyud_firmpatcher_1.0.5-2`、2010 年公開、旧配布元 `http://dyud200.zxq.net/`、[Wayback Machine 2013-05-27 アーカイブ](http://web.archive.org/web/20130527152129/http://dyud200.zxq.net/)）
- **参考資料**: [USB地デジチューナ DY-UD200 まとめwiki](https://w.atwiki.jp/dy-ud200/)
- **純正インストーラー（`DAWIN DTV 1.0.0.8`）**: 旧ダイナコネクティブ公式配布（[Wayback Machine 2010-11-28 アーカイブ](https://web.archive.org/web/20101128030533id_/http://dynaconnective.co.jp/support/download/ud200/DAWIN%20DTV_setup_1.0.0.8.zip)）
- **本ツールの実装**: Copyright (c) 2026 waya0125（MIT License、[LICENSE](LICENSE)）
