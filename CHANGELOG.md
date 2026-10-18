# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.5.0] - 2026-10-18

### Added

- Explore page at `/explore` with trending searches and featured GIFs, linked from the homepage.
- SD, HD and MP4 format tabs on GIF pages. With JavaScript the media is swapped in place; without it the tabs are plain `?format=` links.
- Download button on GIF pages. It becomes a menu when more than one format is available.
- Related GIFs section on GIF pages, with pagination.
- Hover overlay on GIF tiles in search results, profiles, Explore and related GIFs. It shows the first tags and a download menu.
- Memes and Stickers rows on profile pages, each with a "See all" page at `/users/{username}/{kind}` (and `/official/{username}/{kind}`). Profile uploads are paginated.
- Optional media cache on disk for GIFs, videos, thumbnails, avatars and banners. It is off unless `MEZZO_CACHE_MEDIA_DIR` is set.
  - New settings: `MEZZO_CACHE_MEDIA_MAX_MB` (default `512`), `MEZZO_CACHE_MEDIA_MAX_FILE_MB` (`16`), `MEZZO_CACHE_MEDIA_TTL` (`168h`) and `MEZZO_CACHE_AVATAR_TTL` (`24h`).
  - Least recently used files are evicted when the cache is full, and `Range` requests are served from stored files.
  - Media responses carry a `Mezzo-Cache` header with the value `HIT`, `MISS` or `BYPASS`.
- AppArmor profile at `packaging/apparmor/usr.bin.mezzo`, tested on Ubuntu 24.04 and Debian 12.
- Media cache directory support in the deployment files. The systemd units set `CacheDirectory=mezzo`, the OpenRC init script creates the directory, and the compose and Quadlet files include a commented `tmpfs` example. The `MEZZO_CACHE_MEDIA_*` options are in the packaged environment files and in `.env.example`.

### Changed

- Media is now served from `/media/{host}/{path}` (GIFs, videos, thumbnails) and `/avatar/{host}/{path}` (avatars, banners). URLs end in the real file name and extension, so Discord can embed a link as a GIF.
- Media is sent with `Cache-Control: public, max-age=604800, immutable`. Avatars and banners use `MEZZO_CACHE_AVATAR_TTL` as their `max-age`.
- `MEZZO_CACHE_DISABLED` now also disables the media cache.
- `MEZZO_CACHE_SEARCH_TTL` and `MEZZO_CACHE_SEARCH_MAX` also apply to Explore pages, later pages of a profile's uploads, and later pages of related GIFs.
- The project moved from `foundry.fsky.io` to `gitfield.org`. The container image is now `gitfield.org/fsky/mezzo`, and the releases page and documentation links point to the new location.

### Removed

- The `/proxy.gif?url=` endpoint. Links to it no longer work; use the `/media/` and `/avatar/` paths.

### Fixed

- Environment variables that are set but cannot be parsed (for example `MEZZO_CACHE_GIF_TTL=1hour`) are no longer silently replaced by the default. Mezzo now exits at startup and reports every invalid variable. Unset or empty variables still use the default. Check existing deployments for typos before upgrading.

## [1.4.0] - 2026-06-11

### Added

- Linux packages (`deb`, `rpm` and Arch Linux) with a systemd unit and an environment file at `/etc/default/mezzo`, `/etc/sysconfig/mezzo` or `/etc/conf.d/mezzo`.
- Call-to-action button on partner profiles.
- "Open in Tenor" button on error pages.
- Avatar placeholder on GIF pages when the uploader has no avatar.
- `Content-Disposition` header on media, so browsers save GIFs under their original file name.
- `Makefile` for building, testing, linting and releasing.

### Changed

- Error pages have specific headings and messages. A Tenor 404 is reported as 404, other Tenor failures as 502.
- Pasting a Tenor URL into the search bar also works without a scheme and on `tenor.com` subdomains. The redirect is now temporary (302) instead of permanent (301).
- Social links on profiles get better labels, such as TikTok, Youtube, Instagram and Twitter, instead of generic ones.
- Release binaries are published as bare binaries instead of `tar.gz` and `zip` archives.
- Secondary button styling and minor UI tweaks.

## [1.3.0] - 2026-04-12

### Added

- `-verbose` flag that enables debug logging.
- Release binaries for FreeBSD, OpenBSD, NetBSD and Illumos.
- Explicit `width` and `height` on images and ARIA labels, to reduce layout shift and improve accessibility.

### Changed

- Logging is structured (`log/slog`). Each request logs its method, path, status and duration, and requests for static files are logged at debug level.
- On GIF pages, the GIF is shown beside the metadata.
- Minor CSS tweaks.

### Fixed

- Mezzo now listens on IPv4 and IPv6 separately, so it is reachable on hosts where a single dual-stack socket does not serve both.

## [1.2.0] - 2026-03-25

### Added

- Pagination for search results.
- In-memory cache for GIF pages and search results, configured with `MEZZO_CACHE_DISABLED`, `MEZZO_CACHE_GIF_TTL`, `MEZZO_CACHE_GIF_MAX`, `MEZZO_CACHE_SEARCH_TTL` and `MEZZO_CACHE_SEARCH_MAX`. Profile caching is configured with `MEZZO_CACHE_PROFILE_TTL` and `MEZZO_CACHE_PROFILE_MAX`.
- User profile pages at `/users/{username}` and `/official/{username}`, including social links.
- Partner badge on GIF pages.
- `.env.example`, and updated environment variables in the deployment files.
- Shared HTTP client for requests to Tenor, with timeouts and support for the standard proxy environment variables.
- Experimental CSS masonry layout.

### Changed

- Configuration variables use the `MEZZO_` prefix. `PORT` and `PATCHES_URL` are still accepted as fallbacks.
- Cache busting for the stylesheet.
- Homepage wording.

### Fixed

- Embeds on Discord now work. `og:image` is an absolute URL, and Twitter card tags were added. The scheme honours `X-Forwarded-Proto`.
- Social media links on profiles.

## [1.1.0] - 2026-03-19

### Added

- Metadata on GIF pages: uploader, avatar, upload date, tags, description, file size, duration and dimensions.
- Redirect to the GIF page when a Tenor URL is entered in the search bar.
- systemd unit and OpenRC init script, with deployment instructions.

### Changed

- The entry point moved to `cmd/mezzo`. Build from source with `go build -o mezzo ./cmd/mezzo`.
- Updated homepage, a wider search bar, and a border around the metadata container.

### Fixed

- Container image: corrected the `Containerfile`, added `-trimpath` and version flags to its `go build`, and fixed the OCI labels on `ko` builds.

## [1.0.0] - 2026-03-16

### Added

- First release. Mezzo is a privacy-respecting front-end for Tenor with GIF pages (`/view`) and search (`/search`).

[Unreleased]: https://gitfield.org/fsky/mezzo/compare/v1.5.0...HEAD
[1.5.0]: https://gitfield.org/fsky/mezzo/compare/v1.4.0...v1.5.0
[1.4.0]: https://gitfield.org/fsky/mezzo/compare/v1.3.0...v1.4.0
[1.3.0]: https://gitfield.org/fsky/mezzo/compare/v1.2.0...v1.3.0
[1.2.0]: https://gitfield.org/fsky/mezzo/compare/v1.1.0...v1.2.0
[1.1.0]: https://gitfield.org/fsky/mezzo/compare/v1.0.0...v1.1.0
[1.0.0]: https://gitfield.org/fsky/mezzo/releases/tag/v1.0.0
