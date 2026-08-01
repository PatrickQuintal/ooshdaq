# $OOSHDAQ

$OOSHDAQ is a small, self-hosted collectible swap matcher for trusted
communities.

Mark what you own, what you can swap, and what you want. $OOSHDAQ finds people
worth talking to across collections, including reciprocal swaps and genuine
giveaways. Messaging and the physical exchange happen elsewhere.

The application is one Go process with an embedded browser UI and a SQLite
database. It is distributed as source: build the binary or container yourself.

## Run locally

With the Go version listed in [`go.mod`](go.mod):

```sh
make run
```

Open <http://localhost:8080> and choose one of the fictional local members.
Data is stored in `./data/ooshdaq.sqlite`.

Docker Compose is also supported:

```sh
docker compose up --build
```

## Build and test

```sh
make build
make check
```

Run `make help` for the other development commands.

## Documentation

- [Documentation map](docs/README.md)
- [Local development](docs/operations/local-development.md)
- [Architecture](docs/architecture/overview.md)
- [Contributing](CONTRIBUTING.md)
- [Security](SECURITY.md)

## License

The application source is licensed under [Apache-2.0](LICENSE). Third-party
collectible names and artwork are not covered by that license; see the
[artwork policy](docs/catalog/artwork-provenance.md).
