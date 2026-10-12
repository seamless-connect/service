# Seamless Connect service

**Open Infrastructure for Domain Operations and Agent Bootstrapping**

Seamless Connect is an open-source, cross-provider developer CLI for discrete, authorized domain operations and agent bootstrapping. It is maintained as a core Internet utility under nonprofit administration.

This repository contains supporting service infrastructure for the Seamless Connect CLI, including the observability stack below. The service is intended to coordinate authorized operations while providers retain control of execution and policy.

Early industry participation includes Name.com (Identity Digital), Cloudflare, and DigiCert.

The initial focus is DNS configuration through Domain Connect. DNSSEC, Primary / Secondary DNS, Domain Registration, and Agent Bootstrapping describe planned or developing workflows; availability depends on the operation and participating provider integrations. Industry participation does not imply implemented integrations.

[Use Cases & Protocols](https://seamlessconnect.org/use-cases/) · [Canonical project description](https://github.com/seamless-connect/.github/blob/main/PROJECT-DESCRIPTION.md)

## Observability stack

| Service | URL | What you see |
|---|---|---|
| Grafana | http://localhost:3000 | Main UI — dashboards and explore for Prometheus/Loki/Tempo/Pyroscope (pre-provisioned datasources + dashboards) |
| Prometheus | http://localhost:9090 | Metrics query UI |
| Pyroscope | http://localhost:4040 | Profiling UI |
| Tempo | http://localhost:3200 | Trace query API (use Grafana's Explore → Tempo for the UI) |
| Loki | http://localhost:3100 | Log query API (use Grafana for the UI) |
