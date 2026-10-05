# service
Open service for automating domain setup across DNS providers and applications.

## Observability stack

| Service | URL | What you see |
|---|---|---|
| Grafana | http://localhost:3000 | Main UI — dashboards and explore for Prometheus/Loki/Tempo/Pyroscope (pre-provisioned datasources + dashboards) |
| Prometheus | http://localhost:9090 | Metrics query UI |
| Pyroscope | http://localhost:4040 | Profiling UI |
| Tempo | http://localhost:3200 | Trace query API (use Grafana's Explore → Tempo for the UI) |
| Loki | http://localhost:3100 | Log query API (use Grafana for the UI) |
