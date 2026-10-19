# Mezzo
Mezzo is an open-source, privacy-focused front-end for Tenor that allows you to browse and view GIFs without Google's tracking and advertising systems.

<details>
<summary>Click to see screenshots</summary>

![Showcase of the GIF view page](./docs/images/view.png)
*Showcase of the GIF view page*

![Showcase of the user profile page](./docs/images/user.png)
*Showcase of the user profile page*

![Showcase of the search page](./docs/images/search.png)
*Showcase of the search page*

![Showcase of the explore page](./docs/images/explore.png)
*Showcase of the explore page*
</details>

# Instances
For public instances of Mezzo, see [mezzo-instances](https://gitfield.org/fsky/mezzo-instances).

# Run your own instance

## With Docker/Podman
We have pre-built images you can run.

To run the latest stable version of Mezzo (recommended):

```bash
docker run -p8006:8006 gitfield.org/fsky/mezzo:latest
```

To run the canary (unstable) version of Mezzo built from the latest Git commit:

```bash
docker run -p8006:8006 gitfield.org/fsky/mezzo:canary
```

If you are using Podman, the process is the same. Just replace `docker` with `podman` in the command.

### Compose
You can find a compose file in [`contrib/compose/compose.yaml`](contrib/compose/compose.yaml). Simply download it and run:

```
docker compose up -d
```

### Quadlet
You can find a Quadlet file in [`contrib/quadlet/mezzo.container`](contrib/quadlet/mezzo.container). Download it and place it into `~/.config/containers/systemd/` or `/etc/containers/systemd/`. After that, run:

```
systemctl --user daemon-reload
systemctl --user start mezzo.service
```

## From a binary
Binaries are provided for Linux, Windows, macOS, FreeBSD, OpenBSD, NetBSD, and Illumos for amd64, arm64, and i386. You can download them from the [releases page](https://gitfield.org/fsky/mezzo/releases).

Once downloaded, run the binary directly. On Unix-like systems you may need to mark it as executable first:

```bash
chmod +x mezzo
```

## Linux packages
Linux packages are also published for Debian/Ubuntu (`.deb`), RPM-based distributions (`.rpm`), and Arch Linux (`.pkg.tar.zst`).

The packages install:
- the `mezzo` binary into `/usr/bin`
- a systemd service unit
- a distro-appropriate environment file:
  `/etc/default/mezzo` on Debian
  `/etc/sysconfig/mezzo` on RPM-based systems
  `/etc/conf.d/mezzo` on Arch Linux
- the man pages `mezzo(1)` and `mezzo-config(5)`

After installation, edit the environment file if needed, then enable the service:

```bash
sudo systemctl enable --now mezzo
```

### Running with systemd or other service managers
For systemd, you can find an example service file in [`contrib/systemd/mezzo.service`](contrib/systemd/mezzo.service).

For OpenRC, you can find an example service script and configuration file in [`contrib/openrc/`](contrib/openrc/).

## Build from source
This program is written in Go. You need Go 1.25 or later.

You can build a binary using:
```bash
go build -o mezzo ./cmd/mezzo
```

### Install from source
`make install` installs the binary, the man pages, and the README and license. Build first, then install as root:

```bash
make build
sudo make install
```

`make install` does not build, so the binary keeps the version that `make build` gave it.

| What | Where (default) |
|------|-----------------|
| `mezzo` | `/usr/local/bin` |
| `mezzo.1`, `mezzo-config.5` | `/usr/local/share/man/man1`, `/usr/local/share/man/man5` |
| `README.md`, `LICENSE` | `/usr/local/share/doc/mezzo` |

Set `PREFIX` to install elsewhere (`make install PREFIX=/usr`). Packagers can set `DESTDIR` to stage the files into a directory, and `BINDIR`, `MANDIR` and `DOCDIR` to override single locations. To install into your home directory without the docs, run only the parts you need:

```bash
make install-bin install-man PREFIX="$HOME/.local"
```

`make uninstall` removes what was installed; give it the same `PREFIX` and `DESTDIR`.

#### systemd
The systemd unit is not part of `make install`, because not every system runs systemd. On a systemd system, add `install-systemd`:

```bash
sudo make install install-systemd
```

This puts `mezzo.service` in `/usr/local/lib/systemd/system` (override with `UNITDIR`), with `ExecStart` pointing at the installed binary. To change settings, put them in `/etc/default/mezzo`, `/etc/sysconfig/mezzo` or `/etc/conf.d/mezzo`, whichever you prefer, as described in `man mezzo-config`. Then load the unit and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mezzo
```

On OpenRC, use the example service in [`contrib/openrc/`](contrib/openrc/). `make install` does not install the AppArmor profile (see [AppArmor](#apparmor)) or an environment file.

## AppArmor
An optional AppArmor profile is in [`packaging/apparmor/usr.bin.mezzo`](packaging/apparmor/usr.bin.mezzo). It lets Mezzo listen on a TCP port, resolve DNS, make HTTPS requests, and read and write its media cache directory. It does not let Mezzo run other programs or touch any other file. It has been tested on Ubuntu 24.04 and Debian 12.

To install it on Debian or Ubuntu and restart the service:

```bash
sudo install -m 644 packaging/apparmor/usr.bin.mezzo /etc/apparmor.d/usr.bin.mezzo
sudo apparmor_parser -r /etc/apparmor.d/usr.bin.mezzo
sudo systemctl restart mezzo
```

The profile applies to `/usr/bin/mezzo` and `/usr/local/bin/mezzo`, and allows the media cache in `/var/cache/mezzo`, where the systemd unit keeps it. If you use a different `MEZZO_CACHE_MEDIA_DIR` or a port below 1024, add rules to `/etc/apparmor.d/local/usr.bin.mezzo`, which is kept across upgrades, then reload the profile with the `apparmor_parser -r` command above:

```
# a different MEZZO_CACHE_MEDIA_DIR
/srv/mezzo-cache/ r,
/srv/mezzo-cache/** rw,

# a listening port below 1024 (the service needs the capability itself too)
capability net_bind_service,
```

If you delete the local file later, reload with `sudo apparmor_parser -r -T /etc/apparmor.d/usr.bin.mezzo`. Otherwise AppArmor may keep using a cached copy of the profile that still has the old rules.

To see what the profile would block without enforcing it, load it in complain mode with `sudo apparmor_parser -r -C /etc/apparmor.d/usr.bin.mezzo`. The kernel logs these accesses (`journalctl -k | grep apparmor`, or `/var/log/audit/audit.log` if auditd is installed). Without auditd the kernel may drop some of the messages.

For a container, load the profile the same way, start the container with `--security-opt apparmor=mezzo`, and allow the container's cache path (`/cache` in the compose and Quadlet files) in the local file. This was tested with rootful Podman, including with `no-new-privileges`.

# Configuration
The application can be configured using the following environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MEZZO_PORT` | No | `8006` | Port to run on. |
| `MEZZO_PATCHES_URL` | No | | Link to any patches that were applied. Only set this if your instance runs modified source code. |
| `MEZZO_CACHE_DISABLED` | No | `false` | Disable all caching, including the media cache on disk. |
| `MEZZO_CACHE_GIF_TTL` | No | `1h` | How long to cache GIF page metadata. Accepts Go duration strings (e.g. `30m`, `2h`). |
| `MEZZO_CACHE_GIF_MAX` | No | `1000` | Maximum number of GIF pages to keep in cache. |
| `MEZZO_CACHE_SEARCH_TTL` | No | `10m` | How long to cache search results, Explore page results, later pages of a profile's GIFs, stickers and memes, and later pages of related GIFs. Accepts Go duration strings. |
| `MEZZO_CACHE_SEARCH_MAX` | No | `500` | Maximum number of search result pages (and, separately, Explore pages) to keep in cache. Later profile pages and later pages of related GIFs share the search result slots. |
| `MEZZO_CACHE_PROFILE_TTL` | No | `30m` | How long to cache user profile pages. Accepts Go duration strings. |
| `MEZZO_CACHE_PROFILE_MAX` | No | `200` | Maximum number of user profiles to keep in cache. |
| `MEZZO_CACHE_MEDIA_DIR` | No | | Directory for the [media cache on disk](#media-cache-on-disk). The cache is off unless this is set. |
| `MEZZO_CACHE_MEDIA_MAX_MB` | No | `512` | Maximum total size of the media cache, in MiB. |
| `MEZZO_CACHE_MEDIA_MAX_FILE_MB` | No | `16` | Largest single file to store in the media cache, in MiB. |
| `MEZZO_CACHE_MEDIA_TTL` | No | `168h` | How long to keep GIFs, videos and thumbnails in the media cache. Accepts Go duration strings, which have no unit for days. |
| `MEZZO_CACHE_AVATAR_TTL` | No | `24h` | How long to keep avatars and banners in the media cache and in browsers. Accepts Go duration strings. |

> [!NOTE]
> For backward compatibility, `PORT` and `PATCHES_URL` (without the `MEZZO_` prefix) are still accepted as fallbacks.

A variable that is set but cannot be parsed (for example `MEZZO_CACHE_GIF_TTL=1hour`) stops Mezzo from starting, and the error names every such variable. Unset or empty variables use the default.

These settings are also documented in the `mezzo-config(5)` man page (`man mezzo-config`). `man mezzo` describes the command itself.

## Media cache on disk
Out of the box, Mezzo fetches every GIF, video, thumbnail and avatar from Tenor each time someone asks for it. Set `MEZZO_CACHE_MEDIA_DIR` to a directory Mezzo can write to and it keeps a copy there instead, so repeat views load faster and Tenor sees fewer requests from your instance.

- Use a directory that nothing else uses. Mezzo only ever deletes the files it created there.
- The cache never grows past `MEZZO_CACHE_MEDIA_MAX_MB`. When it is full, the least recently used files are deleted first. Files larger than `MEZZO_CACHE_MEDIA_MAX_FILE_MB` are passed through without being stored.
- GIFs, videos and thumbnails are kept for `MEZZO_CACHE_MEDIA_TTL`. Avatars and banners can change, so they are kept for `MEZZO_CACHE_AVATAR_TTL` instead, which is also how long browsers keep them.
- Responses to media requests have a `Mezzo-Cache` header that says whether the file was served from the cache (`HIT`), fetched and stored (`MISS`), or fetched but not stored (`BYPASS`).
- The directory is a record of what has been viewed on your instance, like an access log. Keep it private, and keep the TTLs short if that matters to your users. `MEZZO_CACHE_DISABLED=true` turns the media cache off too.

To enable it with the packages or the systemd unit, uncomment `MEZZO_CACHE_MEDIA_DIR` in the environment file; the unit provides `/var/cache/mezzo`. With containers, see the comments in the [compose file](contrib/compose/compose.yaml) and the [Quadlet file](contrib/quadlet/mezzo.container).

# License
Copyright © 2026-present FSKY. This software is distributed under the [AGPL-3.0 license](LICENSE).
