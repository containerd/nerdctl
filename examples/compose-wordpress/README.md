# Demo: wordpress + mariadb

Usage:
- Substitute "examplepass" in [`docker-compose.yaml`](./docker-compose.yaml) to your own password.
- Run `nerdctl compose up`.
- Open http://localhost:8080, and make sure Wordpress is working. If you see "Error establishing a database connection", wait for a minute.

## eStargz version

eStargz version enables lazy-pulling. See [`../docs/stargz.md`](../docs/stargz.md).

Usage: `nerdctl --snapshotter=stargz compose -f docker-compose.stargz.yaml up`
