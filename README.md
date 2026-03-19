# Mezzo
Mezzo is a privacy-respecting front-end to Tenor.

This project is incomplete. The only supported endpoints are /view (gif) and /search (search).

# Instances
For public instances of Mezzo, see [mezzo-instances](https://foundry.fsky.io/fsky/mezzo-instances).

# Run your own instance

## With Docker/Podman
We have a pre-build image. You can run it using:

```bash
docker run -p8006:8006 foundry.fsky.io/fsky/mezzo:latest
```

If you are using Podman, the process is the same. Just replace `docker` with `podman` in the command.

### Compose
You can find a compose file in `contrib/compose/compose.yaml`. Simply download it and run:

```
docker compose up -d
```

### Quadlet
You can find a Quadlet file in `contrib/quadlet/mezzo.container`. Download it and place it into `.config/containers/systemd/`. After that, run:

```
systemctl --user daemon-reload
systemctl --user start mezzo.service
```

## From a binary
Binaries are provided for Windows, macOS, and Linux for amd64 and arm64. You can download them from the [releases page](https://foundry.fsky.io/fsky/mezzo/releases).

Once downloaded, simply extract the tarball or zip file, and run the executable. 

### Running with systemd or other service managers
For systemd, you can find an example service file in `contrib/systemd/mezzo.service`.

For OpenRC, you can find an example service script and configuration file in `contrib/openrc/`.

## Build from source
This program is written in Go. You need Go 1.25 or later.

You can build a binary using:
```bash
go build -o mezzo ./cmd/mezzo
```

## Environment
`PATCHES_URL` - Link to any patches that were applied. Necessary if there are any. Do not set if there aren't.

The following are optional.

`PORT` - What port to run on (default `8006`).
