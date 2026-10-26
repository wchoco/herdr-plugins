# bero: transports from WSL and ssh to a Windows IME Agent, and their latency

Research for [#51](https://github.com/wchoco/herdr-plugins/issues/51) (map [#43](https://github.com/wchoco/herdr-plugins/issues/43), bero). Feeds [#53](https://github.com/wchoco/herdr-plugins/issues/53).

Measured on this machine: WSL 3.0.3.0, kernel 6.18.54.1-microsoft-standard-WSL2, Ubuntu, i7-1260P (16 threads), Go 1.27.3, **WSL networking mode NAT** (`wslinfo --networking-mode` prints `nat`; no `.wslconfig`). System settings and firewall rules were left unchanged. The IME was not touched. The benchmark code is in [`bero-transport-latency-bench/`](bero-transport-latency-bench/). It has `//go:build ignore` so `scripts/check.sh` skips it.

## Question

How can bero reach an IME Agent on the Windows client when bero runs beside herdr in WSL, over ssh, or WSL→ssh? The candidates are a .exe spawned through WSL interop per call, a long-lived interop process over stdin/stdout, localhost TCP or a named pipe, ssh `-R` reverse forwarding, and terminal escape-sequence passthrough. Which ones meet the target that IME Off lands within ~50 ms of the Trigger, before the next keystroke?

## Short answer

| Path | Topology | Round trip (this machine) | vs 50 ms |
|---|---|---|---|
| Spawn a .exe per call through interop | WSL | **measured** p50 143–171 ms, p95 287–371 ms (exe on `/mnt/c`); p50 667 ms, p95 1643 ms (exe on the Linux FS) | **Fails** |
| Long-lived interop process, line protocol over stdin/stdout | WSL | **measured** p50 0.51–0.62 ms, p95 1.0–2.4 ms, p99 1.7–9.6 ms, max 10–23 ms; one-off start-up 120–480 ms | **Passes** easily |
| Localhost TCP, WSL dials a Windows listener on 127.0.0.1 | WSL, NAT mode | **not reachable**: `connection refused` on 127.0.0.1, `i/o timeout` on the host IP | n/a in NAT |
| Same, mirrored mode | WSL, mirrored | not measured (would need a settings change); estimate ≈ the next row | Passes (est.) |
| Localhost TCP, Windows agent dials a WSL listener on 127.0.0.1 (localhost forwarding) | WSL, NAT or mirrored | **measured** p50 0.84–0.98 ms, p95 1.8–4.0 ms, p99 5.6–7.0 ms, max ~40 ms | **Passes** |
| ssh `-R` (RemoteForward of a Unix socket or port on the herdr host back to the agent on Windows) | ssh | **estimated** network RTT + ~1 ms | Passes only if RTT ≲ 40 ms |
| ssh `-R` into WSL, then one of the WSL hops above to the agent | WSL→ssh | **estimated** network RTT + ~1–2 ms | Passes only if RTT ≲ 40 ms |
| Terminal escape-sequence passthrough | any | no channel exists (see 6) | n/a |

Recommendation for #53:

- **WSL:** spawn the IME Agent once and keep it. bero starts `ime-agent.exe` through interop at plugin start and speaks a line protocol over its stdin/stdout. This works in NAT and mirrored mode, needs no port, firewall rule or auth token, and the pipe belongs to bero alone. The 120–480 ms start-up cost is paid once, and again only on a respawn.
- **ssh and WSL→ssh:** a long-lived agent on Windows, reached through an ssh `RemoteForward`. bero dials a Unix socket on the herdr host. The latency floor is the network RTT, and the spec should call out links where RTT is over ~40 ms as missing the target.
- **Never** spawn a process per call.

## Findings

### 1. Per-call spawn through interop is 3–30× over budget

WSL runs Windows executables directly from the Linux shell, "piping, redirects, and even backgrounding work as expected" ([Microsoft: Working across file systems, "Run Windows tools from Linux"](https://learn.microsoft.com/en-us/windows/wsl/filesystems#run-windows-tools-from-linux)). The `WSLInterop` binfmt entry hands them to `/init` (`/proc/sys/fs/binfmt_misc/WSLInterop`: `interpreter /init`).

Measured with `bench -mode spawn` (`exec.Command(...).Run()` of a Go no-op exe, 200 runs back to back):

| Binary | min | p50 | p95 | p99 | max (ms) |
|---|---|---|---|---|---|
| `/bin/true` (Linux baseline) | 0.58 | 0.83 | 1.50 | 1.82 | 2.24 |
| no-op exe on `/mnt/c` (run 1) | 78.5 | 143.0 | 287.0 | 769.8 | 1290.3 |
| no-op exe on `/mnt/c` (run 2) | 48.8 | 171.4 | 371.5 | 571.0 | 606.1 |
| `HOSTNAME.EXE` from System32 (n=100) | 69.1 | 148.6 | 280.7 | 321.2 | 360.7 |
| no-op exe on the Linux FS (`/tmp`) | 340.4 | 666.7 | 1642.8 | 1899.2 | 2412.4 |

- Even the minimum (~50–80 ms) is over the target, so no tuning of the exe gets below it. The signed System32 binary is no faster than the unsigned Go exe, so the cost is in interop process creation, not in our binary.
- An exe stored on the Linux file system is ~4× slower again. Windows has to read it back over the `\\wsl$` share. Microsoft's guidance is the same: keep files on the file system of the OS that uses them ([filesystems, "File storage and performance across file systems"](https://learn.microsoft.com/en-us/windows/wsl/filesystems#file-storage-and-performance-across-file-systems)). If any exe is spawned at all, it belongs on a Windows drive.
- This rules out the im-select / zenhan pattern for bero. Those tools are one-shot CLIs: `zenhan.exe {im}` is called once per switch ([iuchim/zenhan](https://github.com/iuchim/zenhan); [iuchi's Qiita article](https://qiita.com/iuchi/items/9ddcfb48063fc5ab626c)). The same goes for clipboard helpers in the style of `win32yank -i` / `-o` ([equalsraf/win32yank](https://github.com/equalsraf/win32yank)). That is fine for a clipboard, but not for an IME switch that must beat the next keystroke.
- Caveat: these numbers likely include real-time antivirus scanning of each process start, which was not measured or changed. Other machines may be faster, but the 50 ms floor is not plausibly reachable.

### 2. A long-lived interop process over stdin/stdout is ~0.5 ms

Measured with `bench -mode pipe`: one `winhelper.exe echo` process, one line written and one read back per round trip, 5 ms between round trips (1000 runs). A second run used 200 ms gaps (300 runs) to mimic idle time between Triggers.

| Run | start + first round trip | min | p50 | p95 | p99 | max (ms) |
|---|---|---|---|---|---|---|
| exe on `/mnt/c`, 5 ms gap | 205.5 | 0.17 | 0.51 | 1.00 | 1.66 | 10.19 |
| exe on `/mnt/c`, 5 ms gap (run 2) | 208.6 | 0.19 | 0.59 | 1.10 | 2.42 | 12.94 |
| exe on Linux FS, 5 ms gap | 482.4 | 0.22 | 0.54 | 0.98 | 2.23 | 3.95 |
| exe on `/mnt/c`, 200 ms gap | 121.7 | 0.24 | 0.62 | 2.44 | 9.61 | 23.37 |

- Steady-state cost is ~0.5 ms, about 100× under budget. Even the worst sample (23 ms) fits.
- The file system the exe lives on only changes start-up, not steady state.
- The 200 ms-gap run has a fatter tail (p99 9.6 ms). Idle wake-up costs a few ms, but it stays well inside 50 ms.
- Design consequences:
  - bero owns the agent's lifetime. It spawns at plugin start, respawns on EOF, and the agent exits when stdin closes, so no orphans are left. The first Trigger after a (re)spawn can miss the target, because start-up takes 120–480 ms.
  - Auth is implicit: only bero holds the pipe ends.
  - It only works where bero runs in the same WSL instance as an interop-capable session. Interop can be disabled per session (`echo 0 > /proc/sys/fs/binfmt_misc/WSLInterop`, [filesystems, "Disable interoperability"](https://learn.microsoft.com/en-us/windows/wsl/filesystems#disable-interoperability)), and bero should detect that.
  - The agent runs "as the active Windows user" ([filesystems](https://learn.microsoft.com/en-us/windows/wsl/filesystems#run-windows-tools-from-linux)) in the interactive session, which is what an IME call needs. Whether a process started this way can set the foreground window's IME is #50's question, not this one.
- Prior art: npiperelay is the same shape, a Windows exe that relays a named pipe over its stdin/stdout to WSL ([jstarks/npiperelay](https://github.com/jstarks/npiperelay)). wsl-ssh-agent's WSL2 recipe has socat listen on a Unix socket and relay each connection through `npiperelay.exe`, because "AF_UNIX interop does not work reliably with WSL2 VMs" ([rupor-github/wsl-ssh-agent](https://github.com/rupor-github/wsl-ssh-agent)). Note that socat `EXEC`-ing npiperelay per connection pays the spawn cost from 1 on every connection. bero should keep one connection, or one process, open.

### 3. Localhost TCP: only Windows→WSL works in NAT mode

What Microsoft documents for the default NAT mode ([Accessing network applications with WSL](https://learn.microsoft.com/en-us/windows/wsl/networking)):

- Windows→Linux: "you can access it from a Windows app … using `localhost`" (localhost forwarding).
- Linux→Windows: "you need to use the IP address of your host machine" (the default gateway). Such connections "will be treated as connections from the Local Area Network", so the server may need to bind `0.0.0.0` instead of `127.0.0.1`.
- Mirrored mode (Windows 11 22H2+, `networkingMode=mirrored`) lets you "Connect to Windows servers from within Linux using the localhost address `127.0.0.1`" (not `::1`). The Hyper-V firewall is on by default since WSL 2.0.9 on 22H2+.

Measured on this NAT machine:

- **WSL→Windows:** `winhelper.exe tcp 127.0.0.1:47651` (bound to loopback only; exits after 60 s). Dialling `127.0.0.1:47651` from WSL gave `connection refused`. Dialling the host IP `172.22.96.1:47651` gave `i/o timeout`: the loopback-bound listener is not on that interface, and the firewall drops the packets. Making it work would need a `0.0.0.0` / vEthernet bind plus a firewall rule. That exposes the agent beyond loopback, so it was not done. Treat this direction as unavailable in NAT mode for a zero-config agent.
- **Windows→WSL (localhost forwarding):** `bench -mode accept` listens on `127.0.0.1` in WSL and starts `winhelper.exe dialecho`. The exe dials `127.0.0.1:<port>` from Windows and echoes; WSL does the timing.

| Run | spawn + connect | min | p50 | p95 | p99 | max (ms) |
|---|---|---|---|---|---|---|
| 5 ms gap, n=1000 | 1283.9 | 0.38 | 0.98 | 1.78 | 5.64 | 40.53 |
| 200 ms gap, n=200 | 169.6 | 0.43 | 0.84 | 4.01 | 7.00 | 39.72 |

- ~1 ms p50, about twice the pipe, and the tail is longer (max ~40 ms in both runs, close to the budget).
- This direction fits an agent that is started on Windows independently of bero and then finds bero. bero listens and the agent dials in. It works in both modes, but it needs a rendezvous (port) and an auth token, because any local Windows process can dial `127.0.0.1`.
- A first try timed on the Windows side read 0 µs for 38 of 500 samples: Go's Windows clock is too coarse for sub-ms timing. That is why all numbers here are timed from WSL.
- Mirrored-mode WSL→Windows `127.0.0.1` was not measured, because switching modes changes system settings. Expect it to sit close to the Windows→WSL figure, but that is an estimate.

### 4. Named pipes and AF_UNIX across the VM boundary need a relay

WSL2 cannot open a Windows named pipe directly. npiperelay exists for this ([npiperelay](https://github.com/jstarks/npiperelay)). wsl-ssh-agent reports AF_UNIX interop as unreliable on WSL2 ([wsl-ssh-agent](https://github.com/rupor-github/wsl-ssh-agent)). Either way the hop is a long-lived interop process, i.e. section 2 with one more hop. A named pipe only adds value if the agent must also serve native Windows clients.

### 5. ssh topologies: RemoteForward bounds latency by the network RTT

OpenSSH `RemoteForward` "Specifies that a TCP port or Unix-domain socket on the remote machine be forwarded over the secure channel". The destination can be `host:hostport` or a Unix socket path ([ssh_config(5)](https://man.openbsd.org/ssh_config)). On the server side:

- `AllowStreamLocalForwarding` and `AllowTcpForwarding` default to `yes`.
- `StreamLocalBindUnlink` defaults to `no`, so a stale socket file blocks the forward until it is removed. Set it on the server, or use a per-session path.
- `GatewayPorts no` keeps TCP forwards on loopback.
- (All [sshd_config(5)](https://man.openbsd.org/sshd_config).) The client can set `ExitOnForwardFailure`.

Windows ships OpenSSH (`C:\Windows\System32\OpenSSH\`), so the Windows-side `ssh` can carry the forward.

Shapes:

- **herdr over ssh from Windows:** `RemoteForward /run/user/<uid>/bero.sock 127.0.0.1:<agent-port>` on the Windows ssh client. The agent listens on Windows `127.0.0.1` (Windows-local loopback works without any WSL networking caveats), and bero dials the Unix socket. A Unix socket on the remote side gets file-permission auth for free (`StreamLocalBindMask`).
- **WSL→ssh:** the ssh client runs in WSL, so the forward's destination is in WSL. A small bridge in WSL then reaches the agent through section 2 (a long-lived interop process) or section 3 (the agent dials in). This adds ~0.5–1 ms.

Latency model (estimate, from the measurements above):

- In every ssh shape the Trigger is caused by a keystroke typed on Windows. One example is Esc for vim normal mode. The key travels to the herdr host (RTT/2). herdr and bero detect the change, and the Off command travels back (RTT/2). The Off lands about `RTT + detection + ~1 ms` after the keystroke.
- For it to beat a next keystroke ~50 ms later, the RTT must be ≲ 40 ms, leaving headroom for detection, which is #47–#49's concern.
- Fire-and-forget Off is enough. Waiting for an ack adds no user-visible latency but must not block the next command.
- Ballpark RTTs: LAN or Tailscale direct is ~1–10 ms (passes), same region ~10–40 ms (marginal), cross-region over 50 ms (misses; the spec should call this out).
- The ssh/TCP path adds little on top of the RTT (Nagle should be off: Go sets `TCP_NODELAY` by default; ssh sets it for interactive sessions). This is not measured here, since no external host was used.

Caveats:

- bero runs in the herdr server process tree. The yubi clipboard research found that the server env has no `SSH_*` even when the user is attached over ssh ([clipboard-paste-paths.md, short answer 4](https://github.com/wchoco/herdr-plugins/blob/research/clipboard-paste-paths/docs/research/clipboard-paste-paths.md)). So bero cannot tell from its env which client is attached, or where. The forward's socket path has to be a fixed, configured location.
- With several ssh sessions, the last forward to bind wins (when unlink is on). v1 assumes one client.
- The user has to add the `RemoteForward` to their ssh config. That is a one-time setup, and the only non-zero-config piece for ssh.
- herdr's own remote attach (`herdr --remote <ssh-target>`, client local, server remote) was not checked for whether it exposes ssh forwarding options. Open question for #53.

### 6. Terminal escape-sequence passthrough is not a channel

The idea is that bero writes a sequence to its pane, herdr forwards it to the client, and the client's terminal acts on it. Two links are missing:

- herdr's only known pane→client side-effect forwarding is OSC 52 clipboard writes, sent to the foreground client ([docs/herdr.md "Clipboard"](../herdr.md); [clipboard-paste-paths.md §1](https://github.com/wchoco/herdr-plugins/blob/research/clipboard-paste-paths/docs/research/clipboard-paste-paths.md)). No generic passthrough was found.
- No common Windows terminal defines a sequence that sets the IME.

Abusing OSC 52 to signal a Windows clipboard watcher would clobber the user's clipboard and show a "copied" toast. It is rejected.

## Reproducing

```sh
cd docs/research/bero-transport-latency-bench
GOOS=windows GOARCH=amd64 go build -o /mnt/c/Users/$WINUSER/AppData/Local/Temp/bero-bench/winhelper.exe winhelper.go
go build -o /tmp/bench bench.go
W=/mnt/c/Users/$WINUSER/AppData/Local/Temp/bero-bench/winhelper.exe
/tmp/bench -mode spawn -n 200 -exe $W                       # 1
/tmp/bench -mode pipe  -n 1000 -exe $W                      # 2
/tmp/bench -mode pipe  -n 300 -gap 200ms -exe $W            # 2, idle gaps
$W tcp 127.0.0.1:47651 & /tmp/bench -mode tcp -addr 127.0.0.1:47651   # 3, WSL->Windows
/tmp/bench -mode accept -n 1000 -addr 127.0.0.1:47653 -exe $W         # 3, Windows->WSL
```

Delete the `bero-bench` folder afterwards. It was deleted after these runs.
