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
