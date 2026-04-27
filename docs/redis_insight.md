# Viewing Redis Data

If using Docker Compose, start RedisInsight with:

```bash
docker compose --profile tools up -d
```

Then open http://localhost:5540 and connect to `redis://default@redis:6379`.

Alternatively, run RedisInsight manually:

```bash
docker run -d --name redisinsight -p 5540:5540 -v redisinsight:/data --network authproxy redis/redisinsight:latest
```

Add a connection to redis. Connect to the redis server using the following URI:

```
redis://default@redis-server:6379
```

![redis-insight-add-db.jpg](docs/images/redis-insight-add-db.jpg)
