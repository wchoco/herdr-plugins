# Does IMC_SETOPENSTATUS take effect in Windows Terminal?

Answers [bero: verify IMC_SETOPENSTATUS takes effect in Windows Terminal](https://github.com/wchoco/herdr-plugins/issues/55).

## Setup

- Windows 11, Windows Terminal, new Microsoft IME (Japanese).
- Probe: [`bero-imeset-verify/main.go`](bero-imeset-verify/main.go), built with `GOOS=windows GOARCH=amd64 go build -o bero-imeset.exe main.go`.
- It resolves the foreground window's focused child (`GetGUIThreadInfo`), takes `ImmGetDefaultIMEWnd`, and sends `WM_IME_CONTROL` with `IMC_SETOPENSTATUS` through `SendMessageTimeoutW` (200 ms, `SMTO_ABORTIFHUNG`). It reads back `IMC_GETOPENSTATUS` 100 ms later.
- Run from WSL via interop, inside a herdr pane in Windows Terminal (the user's own setup), then typing `aiu` at the prompt.

## Result

| Step | Probe output | Typed `aiu` became | Taskbar indicator |
|---|---|---|---|
| IME On by hand, `imeset off` | before On, sent ok, after Off | `aiu` | matched |
| `imeset on` | before Off, sent ok, after On | `あいう` | matched |

`IMC_SETOPENSTATUS` changes real input in Windows Terminal with the new Microsoft IME. herdr's source comment that it has no effect in terminal hosts does not hold for this combination. The `SendInput` key-injection fallback (`VK_IME_OFF`/`VK_IME_ON`/`VK_KANJI`, also in the probe) was not needed and was not tested.

## Not covered

- Google Japanese Input, conhost, and an elevated Windows Terminal.
- Turning the IME Off mid-composition (expected to drop uncommitted text).
