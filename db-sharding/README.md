# DB Sharding Prototype

A small Go HTTP API that splits user data across **3 PostgreSQL shards**, each in its own Docker container. The app picks a shard for each user from the user's `id`. 

## How routing works

- **Shard key:** `id` (the user ID)
- **Strategy:** modulo sharding, `shard = id % 3`

| User id | `id % 3` | Shard | Host port |
|---|---|---|---|
| 3, 6, 9, … | 0 | `postgres-shard-0` | 5433 |
| 1, 4, 7, … | 1 | `postgres-shard-1` | 5434 |
| 2, 5, 8, … | 2 | `postgres-shard-2` | 5435 |

Writes and reads for a user always go to the same shard, so a lookup by `id` touches only one database.


### 1. Start the shards

```bash
docker compose up -d
docker compose ps        
```

Each container runs `db/schema.sql` the first time its volume is created:


### 2. Run the API

```bash
go run .
```

### some curl commands 

```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"id": 10, "name": "Alice", "email": "alice@example.com"}'
```

```bash
curl http://localhost:8080/users/10
```


### Resetting after a schema change

`db/schema.sql` only runs when a volume is **first created**. After you edit it, wipe the volumes so it runs again:

```bash
docker compose down -v
docker compose up -d
```

Then restart `go run .`, because its connections point to the old containers.


## Limitations

This is a learning prototype, not production code

- **Adding a shard breaks routing.** Changing `% 3` to `% 4` sends about 75% of existing ids to a different shard, where they aren't found. Consistent hashing fixes this.
- **No cross-shard queries.** Only lookups by `id` work. Listing all users or searching by email would have to query every shard and merge the results 
- **One `pgx.Conn` per shard.** It isn't safe for concurrent requests. 