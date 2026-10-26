# bero: reading and setting the Windows IME open state from another process

Research for [#50](https://github.com/wchoco/herdr-plugins/issues/50), under map [#43](https://github.com/wchoco/herdr-plugins/issues/43).
Question: how can a Windows process (the IME Agent) read and set the IME open state of the foreground window (Windows Terminal)?
Does that work with the new Microsoft IME and with Google Japanese Input?
Can the agent be told about IME changes instead of polling?

## Answer

- **Use `WM_IME_CONTROL` sent to the foreground window's default IME window.**
  The steps are `GetForegroundWindow`, then `ImmGetDefaultIMEWnd(hwnd)`, then `SendMessageTimeout(imeWnd, WM_IME_CONTROL, IMC_GETOPENSTATUS (5), 0)` to read.
  To set, send `IMC_SETOPENSTATUS (6)` with `lParam` 0 or 1.
  This is what every working prior-art tool does: zenhan, spzenhan, AutoHotkey `IME.ahk`, InputTip, winime_en.
  It needs no DLL injection, no COM and no elevation, so it fits a small Go program using only `syscall`.
- **`ImmGetContext`/`ImmGetOpenStatus`/`ImmSetOpenStatus` do not work across processes.**
  `ImmGetContext` returned NULL for Windows Terminal's windows in the probe below.
- **TSF (`GUID_COMPARTMENT_KEYBOARD_OPENCLOSE`) is not reachable from another process either.**
  Microsoft documents it as "specific to a thread manager object", meaning it belongs to the target's own thread.
- **It works against Windows Terminal with the new Microsoft IME.** This was measured on this machine; see the probe section.
  Each call takes about 0.5–2 ms.
- **Google Japanese Input is likely to work but was not tested.** The evidence is indirect; see below.
- **No notification exists for open-state changes from another process.**
  WinEvents can say *when to re-read*, such as a foreground or focus change, but not *that the IME toggled*.
  bero's design (read Saved State, set Off, later Restore) does not need change notifications.
- **The IME Agent must stay resident.** Spawning a Windows `.exe` per call from WSL took 355–736 ms here.

Confidence: high for the method and for WT plus the new MS IME (measured).
Medium for Google Japanese Input (old prior-art claims plus the TSF design, no test here).
High that no cross-process open-state notification exists (no documented API; the IME WinEvents cover IME window UI).

## The method in detail

### Which window to target

1. Get the foreground window with `GetForegroundWindow`.
2. Optionally get the focused child with `GetGUIThreadInfo(threadOf(fg)).hwndFocus`.
   `IME.ahk` and InputTip do this.
   For Windows Terminal it makes no difference: the focused child (`Windows.UI.Input.InputSite.WindowClass`) is on the same thread, so `ImmGetDefaultIMEWnd` returns the same IME window (see probe).
   Use the focused child anyway; it is cheap and covers apps whose focus is on another thread.
3. Call `ImmGetDefaultIMEWnd(hwnd)`.
   "The operating system creates a default IME window for every thread … The application can send the WM_IME_CONTROL message to this window."
   ([ImmGetDefaultIMEWnd](https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immgetdefaultimewnd))
   The IME window belongs to the target's thread, and messages to it cross the process boundary like any `SendMessage`.

### The message

- `WM_IME_CONTROL` = `0x0283`. It is documented as "sent by an application to direct the IME window to carry out the requested command".
  ([WM_IME_CONTROL](https://learn.microsoft.com/en-us/windows/win32/intl/wm-ime-control))
- The current desktop docs do **not** list `IMC_GETOPENSTATUS`/`IMC_SETOPENSTATUS` among the `wParam` values.
  The Windows CE docs do: "This message is sent by an application to an IME window to determine whether the IME is open … Nonzero indicates that the current IME is open. Zero indicates the current IME is closed."
  ([IMC_GETOPENSTATUS, Windows CE 5.0](https://learn.microsoft.com/en-us/previous-versions/windows/embedded/ms905960(v=msdn.10)))
  The values are 5 and 6. Wine's `immdev.h` defines `IMC_SETOPENSTATUS 0x0006`.
  Every prior-art tool hard-codes the constants for this reason.
  The commands are thus documented, if only in older docs, and have behaved the same on Windows from 2009 to 2026.
- `IMC_GETCONVERSIONMODE` = 1 and `IMC_SETCONVERSIONMODE` = 2 work the same way (InputTip uses them).
  bero leaves conversion mode untouched, but reading it can help diagnostics.
- Use `SendMessageTimeout` with `SMTO_ABORTIFHUNG` and about 200 ms, not `SendMessage`, so a hung target cannot freeze the agent.
  InputTip does the same with a 200 ms default.

### Why not the IMM32 HIMC functions

`ImmGetOpenStatus`/`ImmSetOpenStatus` take an `HIMC` from `ImmGetContext(hwnd)`.
An input context is per thread ("By default, the operating system creates and assigns an input context to each thread"; [Input Context](https://learn.microsoft.com/en-us/windows/win32/intl/input-context)).
In the probe, `ImmGetContext` on Windows Terminal's windows returned NULL from another process.
A 2025–26 Microsoft Q&A thread reports the same failure: "ImmGetContext was returning 0", so the calls "must be called from the same process or thread".
([Microsoft Q&A](https://learn.microsoft.com/en-us/answers/questions/5612690/detecting-the-ime-status-of-other-windows))
`ImmSetOpenStatus` raises `IMN_SETOPENSTATUS` in the owning app ([ImmSetOpenStatus](https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immsetopenstatus)).
Wine's implementation shows the same in-process path (`SendMessageW(ctx->hWnd, WM_IME_NOTIFY, IMN_SETOPENSTATUS, 0)` in `dlls/imm32/imm.c`).

### Why not TSF directly

`GUID_COMPARTMENT_KEYBOARD_OPENCLOSE` is "a DWORD that is nonzero if the keyboard is open … This compartment is specific to a thread manager object."
([Predefined Compartments](https://learn.microsoft.com/en-us/windows/win32/tsf/predefined-compartments))
A TSF team blog adds that it "applies to an entire thread".
([What's a Keyboard?, tsfaware blog](https://learn.microsoft.com/en-us/archive/blogs/tsfaware/whats-a-keyboard))
An outside process gets its own `ITfThreadMgr`, so it cannot read or write Windows Terminal's compartment.
Only global compartments are shared ([ITfThreadMgr::GetGlobalCompartment](https://learn.microsoft.com/en-us/windows/win32/api/msctf/nf-msctf-itfthreadmgr-getglobalcompartment)), and OPENCLOSE is not one of them.
Reaching it would mean injecting code into WindowsTerminal.exe, which is out of the question for bero.

TSF still matters indirectly. Modern IMEs are TSF text services, and `WM_IME_CONTROL` to the IME window reaches them through the IMM32-on-TSF compatibility layer in the target thread.
Mozc (Google Japanese Input) subscribes to `GUID_COMPARTMENT_KEYBOARD_OPENCLOSE` changes: `OnChange` → `TipEditSession::OnOpenCloseChangedAsync` in `src/win32/tip/tip_text_service.cc` ([google/mozc](https://github.com/google/mozc/blob/master/src/win32/tip/tip_text_service.cc)).
So an open-state change made by the compatibility layer reaches Mozc through that sink.
How IMM32 open status maps to the compartment is not documented; it is inferred from the probe and from prior art working.

## Probe on this machine (read only)

`docs/research/bero-windows-ime-api/probe/main.go` (`//go:build windows`, stdlib `syscall` only) prints, for the foreground window and its focused child:
- class and executable
- HKL
- the default IME window
- `IMC_GETOPENSTATUS` and `IMC_GETCONVERSIONMODE` with timings
- whether `ImmGetContext` works

It never sends a set command.

Environment:
- Windows 11 25H2, build 26200.
- Windows Terminal 1.24.12741.0 (Store).
- Japanese with the Microsoft IME profile `{03B5835F-…}{A76C93D9-…}`, the **new** Microsoft IME: no `NoTsf3Override` value under `HKCU\Software\Microsoft\input\TSF\Tsf3Override`.
- Run from WSL2 via interop while WT was in the foreground.

```
fg     class="CASCADIA_HOSTING_WINDOW_CLASS"  exe=…\WindowsTerminal.exe
       HKL=0x4110411  ImmGetDefaultIMEWnd=0x30298 class="IME"
       IMC_GETOPENSTATUS=1 (ok, 2.2ms)  IMC_GETCONVERSIONMODE=0x9 (ok, 0.5ms)
       ImmGetContext=NULL (cross-process)
focus  class="Windows.UI.Input.InputSite.WindowClass"  same tid/pid
       same IME window, IMC_GETOPENSTATUS=1, IMC_GETCONVERSIONMODE=0x9
       ImmGetContext=NULL (cross-process)
```

Conversion mode `0x9` is `IME_CMODE_NATIVE|IME_CMODE_FULLSHAPE` (hiragana), which matches an open Japanese IME.
Repeated samples returned the same values in about 0–0.5 ms each.
Setting was deliberately not exercised. The set path is the same message with command 6, used by zenhan, spzenhan, IME.ahk and winime_en.
winime_en was written in 2026 for exactly WT + WSL, with a Korean IME ([hdformat/winime_en](https://github.com/hdformat/winime_en), see `src/main.rs`; motivation in [microsoft/terminal#20653](https://github.com/microsoft/terminal/issues/20653)).

Process spawn cost from WSL: about 736 ms per `probe.exe` run from the WSL filesystem, and about 355 ms from `%TEMP%` on NTFS (average of five runs each).
A one-shot `.exe` per Trigger (the zenhan model) cannot meet the ~50 ms target. The IME Agent has to be a long-running process that bero talks to.
That belongs to the transport tickets, but it follows from this one.

## Compatibility

### Windows Terminal (and conhost)

- **WT:** works; measured above.
  WT runs its own TSF implementation (`src/tsf/Implementation.cpp`, which calls `ITfThreadMgrEx::AssociateFocus` on its hwnd; [microsoft/terminal](https://github.com/microsoft/terminal/tree/main/src/tsf)).
  The default IME window still answers `WM_IME_CONTROL`.
- **WT has no IME status interface of its own.** microsoft/terminal#20653 (open, 2026-09) asks for one, and it names herdr.
  A maintainer pointed at RLogin's private mode `?8441` (set/reset to enable/disable the IME, `DECRQM` to query) as a possible model.
  If WT ever adopts it, bero could drive the IME through a terminal escape sequence with no Windows agent. Worth watching; not usable today.
- **conhost:** not tested; no conhost window was in the foreground.
  The method is generic: the console window belongs to conhost.exe and has its own thread IME window. Expect it to work, but verify before claiming it.

### New Microsoft IME (Windows 10 2004+ / Windows 11)

- Works for reading on Windows 11 25H2 (measured).
- Reported problems and how they hold up:
  - **"Always returns 0 on Windows 11"** (Microsoft Q&A, target Notepad).
    The Microsoft moderator got 1/0 correctly on 24H2 with a Japanese IME in Notepad.
    The asker said switching to the previous IME fixed it on one machine. The thread was never resolved.
    The asker also called `GetLastError` after `SendMessage`, which tells nothing.
    Treat a constant 0 as a possible edge case and log `SendMessageTimeout` failures separately from a real 0. This is why the probe reports `ok`.
  - **`IMC_SETOPENSTATUS(0)` during composition discards the uncommitted text.**
    The awase project measured that the new MS-IME ignores `VK_IME_OFF` while a TSF composition is pending: conversion mode changes but the IME stays open, 0/10.
    `IMC_SETOPENSTATUS(0)` closed it 10/10 but "cancels the composition".
    ([cuzic/awase#450](https://github.com/cuzic/awase/pull/450), Chrome, not verified on real Windows 11 hardware.)
    For bero this is mostly harmless: a Trigger such as Esc to normal mode, or a herdr prefix, means the user is leaving input anyway.
    Note it in the spec as a known effect.
  - awase also warns that a synchronous `WM_IME_CONTROL` on a busy target can take up to about 150 ms, and about 5 s if the target hangs.
    `SendMessageTimeout` bounds this.

### Google Japanese Input (Mozc)

- Not tested; it is not installed here.
- Evidence it works:
  - The `IME.ahk` changelog (2009-12-03): "Google日本語入力β 向け調整 … 入力モード 及び 変換モードは取れないっぽい / IME_GET/SET() と IME_GetConverting()は有効".
    That is, open state get/set works and conversion mode does not. ([gist copy of IME.ahk](https://gist.github.com/Wintus/e21bc4aa89165692713ff2c54ca1ed54))
  - Mozc's TIP listens to `GUID_COMPARTMENT_KEYBOARD_OPENCLOSE` (see above).
- Gaps: the evidence is old (Mozc has been TSF-only since then) and there is no first-hand 2026 report.
  Medium confidence. Add a manual check with Google Japanese Input to the bero test plan.
  The open state is what bero needs; conversion mode, which may not work with Mozc, is out of scope.

### Elevation (UIPI)

`SendMessage`/`SendMessageTimeout` are subject to User Interface Privilege Isolation: a process can send messages only to processes of lower or equal integrity level.
([SendMessage remarks](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessage))
If the user runs WT as administrator, a non-elevated IME Agent cannot control it.
The agent should detect this case (`SendMessageTimeout` fails) and report it rather than fail silently.

## Change notification

There is no API that notifies an outside process when another window's IME opens or closes.

- **`IMN_SETOPENSTATUS` / `WM_IME_NOTIFY`** goes only to the owning app's window ([ImmSetOpenStatus remarks](https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immsetopenstatus)).
  Seeing it from outside needs a global `SetWindowsHookEx` message hook, which means a native DLL injected into every GUI process. Not acceptable for bero, and impractical in Go.
- **TSF `ITfCompartmentEventSink`** on OPENCLOSE is per thread manager, so it is in-process only (see above).
- **WinEvents (`SetWinEventHook`, out-of-context, no DLL):**
  - `EVENT_OBJECT_IME_SHOW` 0x8027, `EVENT_OBJECT_IME_HIDE` 0x8028 and `EVENT_OBJECT_IME_CHANGE` 0x8029 are about an IME *window* becoming visible, hidden, or changing size or position. They do not report the open state.
    ([Event Constants](https://learn.microsoft.com/en-us/windows/win32/winauto/event-constants))
    Whether the new MS IME's on-screen mode flash ("あ"/"A") fires them was not tested. Even if it does, that would be an undocumented side effect to avoid relying on.
  - `EVENT_SYSTEM_FOREGROUND` 0x0003 and `EVENT_OBJECT_FOCUS` 0x8005 are documented and reliable for "the target window changed". Use them to know when the window being controlled changes.
- **Polling** `IMC_GETOPENSTATUS` costs about 0.5 ms per read (measured), so even 10–20 Hz polling is cheap if a live view is ever needed.

**Implication for bero:** with the charted model (Trigger start: read Saved State, set Off; Trigger end: Restore; manual changes during a Trigger left alone), the agent only reads on request.
It never has to watch for changes, so no notification is needed for v1.
The one thing worth watching is foreground and focus changes (WinEvents above).
They tell the agent whether the window it would Restore into is still WT, which feeds the lifecycle-edge questions in #43.

## Recommendation for the IME Agent

1. Run a resident Windows process (Go, `GOOS=windows`, stdlib `syscall` only) that serves get/set requests from bero.
2. Get: take the foreground window and its focused child via `GetGUIThreadInfo`, then `ImmGetDefaultIMEWnd`, then `SendMessageTimeout(WM_IME_CONTROL, 5, 0, SMTO_ABORTIFHUNG, 200ms)`.
   Return open/closed, or an explicit error when there is no window, no IME window, or a timeout.
3. Set: the same path with command 6 and `lParam` 0 or 1.
   Optionally read the state back to confirm, and report a mismatch.
4. Optionally check that the foreground executable is `WindowsTerminal.exe` (or the configured terminal) before acting, so the agent never toggles the IME of an unrelated window that took focus.
5. Treat "elevated target" (UIPI) and "set had no effect" as reportable states.
6. Test on WT plus the new MS IME (done for get), WT plus the previous MS IME, and WT plus Google Japanese Input; test conhost if it is in scope.

## Sources

- Microsoft Learn: [ImmGetContext](https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immgetcontext), [ImmGetDefaultIMEWnd](https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immgetdefaultimewnd), [ImmSetOpenStatus](https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immsetopenstatus), [Input Context](https://learn.microsoft.com/en-us/windows/win32/intl/input-context), [WM_IME_CONTROL](https://learn.microsoft.com/en-us/windows/win32/intl/wm-ime-control), [IMC_GETOPENSTATUS (CE 5.0)](https://learn.microsoft.com/en-us/previous-versions/windows/embedded/ms905960(v=msdn.10)), [Predefined Compartments](https://learn.microsoft.com/en-us/windows/win32/tsf/predefined-compartments), [ITfThreadMgr::GetGlobalCompartment](https://learn.microsoft.com/en-us/windows/win32/api/msctf/nf-msctf-itfthreadmgr-getglobalcompartment), [Event Constants](https://learn.microsoft.com/en-us/windows/win32/winauto/event-constants), [SendMessage](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessage), [tsfaware: What's a Keyboard?](https://learn.microsoft.com/en-us/archive/blogs/tsfaware/whats-a-keyboard), [Q&A: Detecting the IME status of other windows](https://learn.microsoft.com/en-us/answers/questions/5612690/detecting-the-ime-status-of-other-windows).
- Prior-art source:
  - [iuchim/zenhan `main.cc`](https://github.com/iuchim/zenhan/blob/master/main.cc)
  - [kaz399/spzenhan.vim `main.cc`](https://github.com/kaz399/spzenhan.vim/blob/master/main.cc)
  - [karakaram/alt-ime-ahk `IME.ahk`](https://github.com/karakaram/alt-ime-ahk/blob/master/IME.ahk)
  - [abgox/InputTip `src/core/ime.ahk`](https://github.com/abgox/InputTip/blob/main/src/core/ime.ahk)
  - [hdformat/winime_en](https://github.com/hdformat/winime_en)
  - [daipeihust/im-select](https://github.com/daipeihust/im-select): its `win/` switches keyboard layout (HKL) via `WM_INPUTLANGCHANGEREQUEST`, not open state; `win-mspy/` scrapes the taskbar IME button via UI Automation. Neither applies to bero.
  - [google/mozc `tip_text_service.cc`](https://github.com/google/mozc/blob/master/src/win32/tip/tip_text_service.cc)
  - [microsoft/terminal `src/tsf`](https://github.com/microsoft/terminal/tree/main/src/tsf)
  - Wine `dlls/imm32/imm.c`, `include/immdev.h`.
- Issues and PRs: [microsoft/terminal#20653](https://github.com/microsoft/terminal/issues/20653), [cuzic/awase#450](https://github.com/cuzic/awase/pull/450).
