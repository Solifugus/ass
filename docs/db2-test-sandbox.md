# DB2 test sandbox — cheat sheet

A throwaway IBM **Db2 (LUW)** for testing ASS's database paths. Image:
`icr.io/db2_community/db2` = Db2 Community Edition — free (capped at 4 cores /
16 GB), no account needed.

> **Two things make Db2 fussier than the Oracle sandbox**, both covered below:
> it needs **rootful** podman, and the Go driver needs IBM's native CLI driver
> (`-tags db2`). The *server* is easy; the *driver* is the work.

## Container lifecycle (must be rootful — use `sudo`)

```bash
# Create + start (first run pulls ~3 GB, initializes in ~3-8 min)
sudo podman run -d --name db2 --privileged=true -p 50000:50000 \
  -e LICENSE=accept -e DB2INST1_PASSWORD=ass_test -e DBNAME=testdb \
  icr.io/db2_community/db2

# Is it ready? (this connects cleanly — not SQL1032N/SQL1035N — when done)
sudo podman exec db2 su - db2inst1 -c "db2 connect to testdb"
# or watch the log for "Setup has completed."
sudo podman logs db2 | tail

sudo podman stop db2     # pause (keeps data)
sudo podman start db2    # resume (ready in ~1 min, no re-pull)
sudo podman rm -f db2    # destroy completely (data gone)
sudo podman ps           # is it running?
```

**Why rootful?** Db2's `db2start` runs setuid-root binaries. Rootless podman
mounts its storage `nosuid`, which blocks them — you get `SQL1641N ... prevented
from executing with root privileges by file system mount settings`, even with
`--privileged`. Rootful podman (`sudo`) has no `nosuid`, so it works. (`docker`
works too — its daemon is already root.)

> **sudo note (this machine):** `sudo-rs` requires interactive auth and the
> timestamp expires, so a *background* poll of `sudo podman ...` will hang
> waiting for a password. Run sudo commands in the foreground.

Once the server is up it speaks TCP on `localhost:50000`, so **everything after
container start needs no sudo** — ASS, the Go test, and any client connect over
the port.

## Connection details

| | |
|---|---|
| Host / port | `localhost` / `50000` |
| Database | `testdb` |
| User / password | `db2inst1` / `ass_test` |
| **ASS connection string** | `HOSTNAME=localhost;PORT=50000;DATABASE=testdb;UID=db2inst1;PWD=ass_test` |

The connection string is the IBM CLI form (`KEY=value;...`), **not** a URL like
the other engines.

## Building ASS with DB2 (one-time driver setup)

DB2 is behind the `db2` build tag because its driver links IBM's native CLI
driver. A plain `go build ./...` ignores it; to compile DB2 in:

```bash
# 1. Download the CLI driver into the module cache (one time):
go run github.com/ibmdb/go_ibm_db/installer/setup.go

# 2. Point the toolchain + loader at it (every shell that builds -tags db2):
export IBM_DB_HOME="$(go env GOMODCACHE)/github.com/ibmdb/clidriver"
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib -ldb2"
export LD_LIBRARY_PATH="$HOME/db2libs:$IBM_DB_HOME/lib:$LD_LIBRARY_PATH"

# 3. Build / run with the tag:
go build -tags db2 ./...
go run -tags db2 ./cmd/ass run program.sas
```

**`libxml2.so.2` gotcha:** `libdb2.so` needs the *old* libxml2 ABI
(`libxml2.so.2`). Recent Ubuntu ships only `libxml2.so.16`, so on this machine a
copy lives in `~/db2libs/` (lifted from the Db2 container's own overlay) and is
prepended to `LD_LIBRARY_PATH` above. If you ever recreate it:

```bash
mkdir -p ~/db2libs
src=$(sudo find /var/lib/containers -name 'libxml2.so.2.9*' 2>/dev/null | head -1)
sudo cp "$src" ~/db2libs/libxml2.so.2   # or grab from any RHEL-ish libxml2 2.9.x
```

## Connect from ASS (SAS)

```sas
libname d db2 "HOSTNAME=localhost;PORT=50000;DATABASE=testdb;UID=db2inst1;PWD=ass_test";

data d.mytable;              /* creates/replaces the table */
  input k $ v;
  datalines;
a 1
b 2
;
run;

proc print data=d.mytable; run;                  /* read it back */
proc append base=d.mytable data=work.more; run;  /* in-place INSERT */

libname d clear;
```

Run it with the `db2`-tagged binary and the env from above:
`go run -tags db2 ./cmd/ass run program.sas`.

## Poke at it directly with the db2 CLP

```bash
# One-off query from your shell:
sudo podman exec -i db2 su - db2inst1 -c "db2 connect to testdb; db2 'SELECT tabname FROM syscat.tables WHERE owner = ''DB2INST1'''"

# Interactive shell as the instance owner, then the db2 prompt:
sudo podman exec -it db2 su - db2inst1
  db2 connect to testdb
  db2 "SELECT * FROM \"mytable\""
  db2 terminate
```

Handy SQL once connected:

```sql
SELECT tabname FROM syscat.tables WHERE owner='DB2INST1';   -- list your tables
SELECT colname, typename FROM syscat.columns WHERE tabname='mytable';  -- columns
SELECT * FROM "mytable";                                    -- see rows
DROP TABLE "mytable";                                       -- clean up one table
```

## ⚠️ Gotchas that will bite you

1. **Rootful only.** See above — rootless podman fails with `SQL1641N`. Always
   `sudo podman` for the container.
2. **CLI connection string, not a URL.** `HOSTNAME=...;PORT=...;DATABASE=...;UID=...;PWD=...`.
3. **ASS writes lowercase quoted identifiers.** Db2 upper-cases unquoted names,
   so a table ASS created as `mytable` is reachable as `"mytable"` (quoted) — in
   `syscat.tables` it shows up under `TABNAME='mytable'`. Quote it in raw SQL.
4. **No `DROP TABLE IF EXISTS`.** Db2 lacks it; ASS's replace path issues a plain
   `DROP TABLE` and swallows the "undefined name" error (`SQL0204N` / SQLSTATE
   `42704`). Not a concern through ASS; only matters if you hand-write DDL.
5. **Slow first boot.** 3-8 min to initialize. `start` after a `stop` is much
   faster. `SQL1035N` right after boot just means activation hasn't finished —
   wait and retry.
6. **`go mod tidy` drops the driver.** A tag-less tidy ignores `//go:build db2`
   and removes `go_ibm_db` from `go.mod` — there's a comment there warning you;
   restore the line or use `go mod tidy -e`.

## Run the gated Go integration test against it

```bash
# (with the IBM_DB_HOME/CGO_*/LD_LIBRARY_PATH env from "Building ASS with DB2" set)
ASS_DB2_DSN="HOSTNAME=localhost;PORT=50000;DATABASE=testdb;UID=db2inst1;PWD=ass_test" \
  go test -tags db2 ./dbio/ -run TestDB2Integration -v
```

Same pattern for the other engines:

| Engine | Env var | Test | Build |
|--------|---------|------|-------|
| Postgres | `ASS_PG_DSN` | `TestPostgresIntegration` | default |
| SQL Server | `ASS_MSSQL_DSN` | `TestSQLServerIntegration` | default |
| Oracle | `ASS_ORACLE_DSN` | `TestOracleIntegration` | default |
| DB2 | `ASS_DB2_DSN` | `TestDB2Integration` | `-tags db2` |

See [`oracle-test-sandbox.md`](oracle-test-sandbox.md) for the Oracle equivalent
and [`podman-cheatsheet.md`](podman-cheatsheet.md) for general container ops.
