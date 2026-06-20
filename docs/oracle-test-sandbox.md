# Oracle test sandbox — cheat sheet

A throwaway Oracle for testing ASS's database paths. Image: `gvenzl/oracle-free`
= Oracle Database **Free 23ai** — no account or license needed for the community
image.

## Container lifecycle

```bash
# Create + start (first run pulls ~2 GB, boots in ~3 min)
podman run -d --name oracle-free -p 1521:1521 -e ORACLE_PASSWORD=ass_test \
  docker.io/gvenzl/oracle-free:slim

# Is it ready? (look for "DATABASE IS READY TO USE!")
podman logs oracle-free | grep "DATABASE IS READY"

podman stop oracle-free     # pause (keeps data)
podman start oracle-free    # resume (ready in seconds, no re-pull)
podman rm -f oracle-free    # destroy completely (data gone)
podman ps                   # is it running?
```

Data lives inside the container: `stop`/`start` keeps your tables; `rm` wipes
them. To survive even a `rm`, add a volume:
`-v oracle-data:/opt/oracle/oradata`.

> `docker` works identically — substitute `docker` for `podman` throughout.

## Connection details

| | |
|---|---|
| Host / port | `localhost` / `1521` |
| Service (PDB) | `FREEPDB1` |
| User / password | `system` / `ass_test` |
| **ASS DSN** | `oracle://system:ass_test@localhost:1521/FREEPDB1` |

## Connect from ASS (SAS)

```sas
libname ora oracle "oracle://system:ass_test@localhost:1521/FREEPDB1";

data ora.mytable;            /* creates/replaces the table */
  input k $ v;
  datalines;
a 1
b 2
;
run;

proc print data=ora.mytable; run;                  /* read it back */
proc append base=ora.mytable data=work.more; run;  /* in-place INSERT */

libname ora clear;
```

## Poke at it directly with sqlplus

```bash
# One-off query from your shell:
podman exec -i oracle-free sqlplus -S system/ass_test@localhost:1521/FREEPDB1 <<'EOF'
SELECT table_name FROM user_tables;
EXIT;
EOF

# Interactive session:
podman exec -it oracle-free sqlplus system/ass_test@localhost:1521/FREEPDB1
```

Handy SQL once you're in:

```sql
SELECT table_name FROM user_tables;        -- list your tables
DESC "mytable";                            -- columns + types
SELECT * FROM "mytable";                   -- see rows
DROP TABLE "mytable";                      -- clean up one table
SET LINESIZE 200 PAGESIZE 50               -- make output readable
```

## ⚠️ Gotchas that will bite you

1. **ASS writes lowercase quoted identifiers.** Oracle upper-cases unquoted
   names, so a table ASS created as `mytable` is **only** reachable as
   `"mytable"` (with quotes) in sqlplus. `SELECT * FROM mytable;` →
   `ORA-00942: table or view does not exist`. Always quote: `SELECT * FROM "mytable";`.
2. **Empty string = NULL in Oracle.** A non-missing but empty character value
   (`""`) comes back as a SAS *missing*, not an empty string. Oracle-specific;
   don't be surprised in round-trip tests.
3. **`DATE` carries a time component.** Oracle `DATE` is really date+seconds. ASS
   maps SAS dates to it fine, but if you compare against a SQL `DATE` literal,
   mind the time part.
4. **`DROP TABLE IF EXISTS` is 23ai-only.** ASS's replace path (`Store`) uses it;
   the Free image is 23ai so it's fine. Older Oracle would choke — not a concern
   for this sandbox.

## Run the gated Go integration test against it

```bash
ASS_ORACLE_DSN="oracle://system:ass_test@localhost:1521/FREEPDB1" \
  go test ./dbio/ -run TestOracleIntegration -v
```

Same pattern for the other engines:

| Engine | Env var | Test | Build |
|--------|---------|------|-------|
| Postgres | `ASS_PG_DSN` | `TestPostgresIntegration` | default |
| SQL Server | `ASS_MSSQL_DSN` | `TestSQLServerIntegration` | default |
| Oracle | `ASS_ORACLE_DSN` | `TestOracleIntegration` | default |
| DB2 | `ASS_DB2_DSN` | `TestDB2Integration` | `-tags db2` |

See [`db2-test-sandbox.md`](db2-test-sandbox.md) for the DB2 equivalent of this
page and [`podman-cheatsheet.md`](podman-cheatsheet.md) for general container ops.
