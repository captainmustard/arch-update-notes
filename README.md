# arch-update-notes

A terminal UI that gathers the **notes for your most recent system update** on
Arch-based distributions (CachyOS, Arch, EndeavourOS, …). After running
`cachy-update` / `arch-update` / `pacman -Syu`, launch it to see — in one place —
what changed and what might need your attention.

It pulls together four things:

- **Packages** — every package upgraded, installed, removed or downgraded in the
  last update session, parsed from `/var/log/pacman.log`, with old → new versions.
- **Changelogs** — per-package changelog via `pacman -Qc` (loaded on demand; many
  packages ship none).
- **News** — recent Arch Linux and CachyOS announcements, with a `[NEW]` tag for
  anything published around the time of the selected update. These are where
  manual-intervention and breaking-change warnings live.
- **Config files** — pending `.pacnew` / `.pacsave` files (via `pacdiff -o`) that
  the update left for you to merge, with the command to do it.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Bubbles](https://github.com/charmbracelet/bubbles).

## Install / build

Requires Go 1.24+ and (for full functionality) `pacman-contrib` for `pacdiff`.

```sh
go build -o arch-update-notes .
./arch-update-notes
```

Optionally install it onto your PATH:

```sh
go install github.com/ianataylor42/arch-update-notes@latest
```

## Usage

```
arch-update-notes [flags]

  --log <path>   path to the pacman log (default /var/log/pacman.log)
  --no-news      skip fetching online news feeds (fully offline)
```

The app groups pacman transactions that happened within 15 minutes of each other
into a single "update session" — so a `cachy-update` run that updates repo
packages and then AUR packages shows up as one update. It defaults to the most
recent session; use `[` / `]` to browse earlier ones.

## Keys

| Key | Action |
| --- | --- |
| `↑` / `↓`, `j` / `k` | Move selection |
| `tab` / `shift+tab`, `←` / `→` | Switch section (Packages / News / Config files) |
| `1` `2` `3` | Jump to a section |
| `[` / `]`, `p` / `n` | Previous / next update session |
| `/` | Filter the current list |
| `PgUp` / `PgDn` | Scroll the detail pane |
| `q`, `ctrl+c` | Quit |

## Notes

- Reading the pacman log, changelogs and `.pacnew` files needs no root.
- News fetching reaches `archlinux.org` and `discuss.cachyos.org`; use
  `--no-news` to stay offline.
- The app is read-only. It never modifies your system — to merge `.pacnew`
  files it just shows you the `sudo pacdiff` command.

## License

MIT
