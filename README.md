# Mezzo
Mezzo is a privacy-respecting front-end to Tenor.

This project is incomplete. The only supported endpoints are /view (gif) and /search (search).

# Instances
For public instances of Mezzo, see [mezzo-instances](https://foundry.fsky.io/fsky/mezzo-instances).

# Run your own instance

## Docker/Podman
We have a pre-build image. You can run it using:

```bash
docker run -p8006:8006 foundry.fsky.io/fsky/mezzo:latest
```

You can find a compose file in `contrib/compose/compose.yml`, and a Quadlet file in `contrib/quadlet/mezzo.container`

## Build from source
This program is written in Go. You need Go 1.25 or later.

You can build a binary using:
```bash
go build
```

## Environment
`PATCHES_URL` - Link to any patches that were applied. Necessary if there are any. Do not set if there aren't.

The following are optional.

`PORT` - What port to run on (default `8006`).
