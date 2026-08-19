# SteamOS / Steam Deck — toolchain that survives updates

SteamOS A/B updates **replace `/usr`**. Pacman packages, `steamos-readonly
disable`, and anything else on the rootfs are gone. **`/home/deck` stays.**

Do not `pacman -S go`. Put the compiler in `$HOME/.local/go`. `make` is already
on the image (base-devel); if an update drops it, this kit does not fight
host pacman — use distrobox, or call us.

## Once

```bash
./scripts/steamos/bootstrap.sh
```

Then **log out of Desktop (or reboot)** so Konsole / Cursor pick up PATH.
Confirm:

```bash
steamos-doctor
go version    # go1.26.x from ~/.local/go
make tools    # rebuilds golangci-lint with *this* Go (GOTOOLCHAIN=local)
```

## After a SteamOS update

Usually nothing. Go lives in home. A user systemd oneshot (`steamos-go-guard`)
reinstalls `~/.local/go` if it is missing at login.

If Cursor still cannot see `go`: fully quit Cursor and open it from Desktop
again (not an old AppImage session).

## Layout

| Path | Role |
| --- | --- |
| `~/.local/go` | Go SDK (official tarball) |
| `~/go/bin` | `go install` tools (`make tools`) |
| `~/.local/bin/steamos-*` | copies of these scripts |
| `~/.config/environment.d/99-steamos-dev.conf` | PATH for systemd user / GUI |
| `~/.config/plasma-workspace/env/steamos-dev.sh` | PATH for KDE Konsole |
| `~/.config/steamos-dev/go-version` | pin for the guard (from `go.mod`) |

## Distrobox (optional)

If you want a real Arch `pacman` that also lives in home:

```bash
distrobox create --name dev --image archlinux:latest
distrobox enter dev
sudo pacman -Syu --needed base-devel git
```

The box’s storage is under `~/.local/share/containers`. Host `/usr` can burn;
the box does not.
