# Podman cheat sheet

General-purpose container ops for spinning up throwaway services to test ASS
against (databases, etc.). Podman is a drop-in for Docker — **every command here
works with `docker` too**, just swap the word.

The DB-specific sandboxes have their own pages:
[`oracle-test-sandbox.md`](oracle-test-sandbox.md),
[`db2-test-sandbox.md`](db2-test-sandbox.md). This page is the generic toolbox.

## Test sandbox credentials (at a glance)

The local throwaway containers all use the disposable password **`ass_test`** —
fine to hardcode because they're local, data-less sandboxes you `rm` when done.

| Engine | Host:port | User / password | Database / service | ASS connection string |
|--------|-----------|-----------------|--------------------|-----------------------|
| Oracle | `localhost:1521` | `system` / `ass_test` | service `FREEPDB1` | `oracle://system:ass_test@localhost:1521/FREEPDB1` |
| DB2 | `localhost:50000` | `db2inst1` / `ass_test` | db `testdb` | `HOSTNAME=localhost;PORT=50000;DATABASE=testdb;UID=db2inst1;PWD=ass_test` |

The matching `-e` env vars at `podman run` time:

```bash
# Oracle (gvenzl/oracle-free):       -e ORACLE_PASSWORD=ass_test
# DB2 (icr.io/db2_community/db2):     -e DB2INST1_PASSWORD=ass_test -e DBNAME=testdb -e LICENSE=accept
```

Postgres and SQL Server have no local sandbox page here — point their integration
tests at your own server via `ASS_PG_DSN` / `ASS_MSSQL_DSN` (DSN formats in
[`databases.md`](databases.md)); use that server's own credentials, and keep real
passwords out of committed files. Full per-engine details: the
[Oracle](oracle-test-sandbox.md) and [DB2](db2-test-sandbox.md) sheets.

## Rootless vs rootful — read this first

By default `podman` runs **rootless** (as your user, no `sudo`). That's safer and
usually what you want. But rootless has real limits that bite when running heavy
server images:

- **`nosuid` storage.** Rootless mounts its container storage `nosuid`, so images
  with setuid-root startup binaries fail to start (this is exactly why **Db2
  needs rootful** — `SQL1641N`). Oracle/Postgres/SQL Server run fine rootless.
- **Privileged ports.** Rootless can't bind host ports < 1024 (use `-p 8080:80`,
  not `-p 80:80`).
- **Separate everything.** Rootless and rootful have **separate image stores,
  containers, and networks.** An image you pulled rootless isn't visible to
  `sudo podman` (it re-pulls), and vice-versa. `sudo podman ps` and `podman ps`
  show different lists.

Rule of thumb: try rootless first; reach for `sudo podman` only when a server
needs setuid/kernel privileges (Db2). Whichever you pick, **stay consistent** —
manage a given container with the same mode you created it.

```bash
podman info | grep -i rootless     # rootless: true/false
```

> **sudo note (this machine):** `sudo-rs` requires interactive auth and the
> credential times out. Run `sudo podman ...` in the foreground — a backgrounded
> sudo command will hang waiting for a password it can't read.

## Lifecycle

```bash
podman run -d --name svc -p HOST:CONTAINER image:tag   # create + start detached
podman run -d --name svc -e KEY=val ... image          # with env vars
podman ps                     # running containers
podman ps -a                  # include stopped
podman stop svc               # graceful stop (keeps the container + its data)
podman start svc              # restart a stopped container (fast, no re-pull)
podman restart svc
podman rm svc                 # remove a stopped container (data gone)
podman rm -f svc              # force-remove even if running
```

`stop`/`start` preserve the container's filesystem (your tables survive). `rm`
destroys it. To survive a `rm`, mount a named volume (below).

## Logs, exec, inspect

```bash
podman logs svc               # all logs
podman logs -f svc            # follow (tail -f style)
podman logs svc | grep READY  # is it up? (grep the image's ready marker)

podman exec -it svc bash      # interactive shell inside
podman exec -i svc some-cmd <<'EOF'   # pipe a script/query in
...
EOF

podman inspect svc            # full JSON (config, mounts, IP, env)
podman port svc               # show published port mappings
podman stats --no-stream      # CPU/mem snapshot
```

## Images

```bash
podman pull image:tag         # fetch without running
podman images                 # list local images
podman rmi image:tag          # delete an image
podman image prune            # delete dangling images (reclaim space)
podman system prune -a        # delete ALL unused images/containers/networks ⚠️
podman system df              # disk usage by images/containers/volumes
```

> Big DB images are 2-3 GB each. `podman system df` then `podman image prune`
> when disk gets tight. Remember rootless and rootful have separate stores —
> prune both if you've used both.

## Networking & ports

```bash
podman run -d -p 5432:5432 ...   # host:5432 -> container:5432
```

From the **host**, reach the service at `localhost:<host-port>` — this is how
ASS, Go tests, and CLI clients connect (no sudo needed even for rootful
containers, since it's just TCP). Containers reach each other by name only on a
shared network:

```bash
podman network create testnet
podman run -d --name db --network testnet ...
podman run -d --name app --network testnet ...   # app reaches "db:5432"
```

## Persistent data (volumes)

```bash
podman run -d -v mydata:/var/lib/postgresql/data ...   # named volume
podman volume ls
podman volume rm mydata
```

A named volume outlives `podman rm` — use it when you want test data to persist
across container recreation; skip it for truly throwaway sandboxes.

## Quick teardown of everything

```bash
podman rm -f $(podman ps -aq)            # remove all containers (rootless set)
sudo podman rm -f $(sudo podman ps -aq)  # ...and the rootful set
```
