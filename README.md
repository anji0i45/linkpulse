# LinkPulse

A local, zero-cost URL shortener with click analytics, built with Go.

This is the **Phase 1 local MVP** of the LinkPulse project. It runs with nothing
but Go — no AWS account, no Docker, no database server, no paid service.

> Resume note: the project is marked **In Progress** until the AWS/serverless
> version is actually deployed. Keep that label until it is true.

## What works right now

- Create a short link from a long URL
- Redirect through the short code
- Track every click asynchronously through an in-process event queue
- Show total clicks and unique visitors
- Show clicks by day and top referrers
- Persist links and clicks in a local JSON file
- Cache hot links in memory for fast redirects
- Dashboard at `http://localhost:8080`
- No external dependencies

## Quick start

```powershell
cd C:\Users\Admin\linkpulse
go run .
```

Open:

```text
http://localhost:8080
```

Paste a URL, create a short link, open it a few times, then click **Analytics**
on the link.

Stop the server with `Ctrl+C`.

## Tests

```powershell
go test ./...
```

## API

Create a link:

```powershell
curl.exe -X POST http://localhost:8080/api/links ^
  -H "Content-Type: application/json" ^
  -d "{\"long_url\":\"https://example.com/a/long/path\"}"
```

List links:

```powershell
curl.exe http://localhost:8080/api/links
```

View analytics:

```powershell
curl.exe http://localhost:8080/api/analytics/CODE
```

Redirect:

```text
http://localhost:8080/CODE
```

Health:

```powershell
curl.exe http://localhost:8080/healthz
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `ADDR` | `:8080` | HTTP listen address |
| `LINKPULSE_DATA` | `data/linkpulse.json` | Local JSON data file |
| `PUBLIC_BASE_URL` | request host | Base URL used in returned short links |

Example:

```powershell
$env:PUBLIC_BASE_URL = "http://localhost:8080"
go run .
```

## Project layout

```text
linkpulse/
├── main.go                    # starts store, cache, queue and HTTP server
├── internal/
│   ├── api/
│   │   ├── api.go             # REST handlers + redirect + analytics
│   │   └── web/index.html     # dashboard
│   ├── cache/
│   │   └── cache.go           # in-memory TTL cache
│   ├── queue/
│   │   └── queue.go           # in-process click event queue
│   └── store/
│       ├── store.go           # data types and Store interface
│       ├── file.go            # JSON file implementation
│       └── file_test.go
├── data/                      # created automatically
├── README.md
├── LEARN_HINGLISH.md          # simple explanation of every concept
└── AWS_FREE_PHASE2.md         # free-tier deployment plan
```

## Why this is a good resume project

The local MVP proves the core backend flow:

- Go HTTP APIs
- URL validation
- Redirects
- Persistence
- Caching
- Async click processing
- Analytics aggregation
- Tests

The next phase replaces the local edges with managed AWS services:

| Local | AWS/free-tier version |
|---|---|
| JSON file | DynamoDB |
| In-memory cache | Redis / ElastiCache or Upstash free tier |
| In-process queue | SQS |
| Local click events | MongoDB Atlas free tier |
| Local analytics query | OpenSearch / Elasticsearch |
| `go run .` | Lambda + API Gateway |
| Manual setup | Terraform |

That keeps the same interfaces and makes the project a real cloud portfolio
piece, not just a tutorial.

## Known limitations (local MVP)

- One process only; the JSON file is not a multi-instance database.
- The click queue is in memory, so events can be lost if the process crashes.
- No login, rate limiting, or link expiry yet.
- AWS, GraphQL, gRPC, and Terraform are Phase 2, not completed yet.

These are intentional. The goal is to understand the flow first, then replace
each local piece with a managed service.

## Free-tier rule

Nothing in this repository requires a paid service. If you deploy the AWS phase:

- use only free-tier eligible services
- set an AWS billing alert
- do not leave always-on OpenSearch/ElastiCache/NAT Gateway resources running
- prefer `Lambda + API Gateway HTTP API + DynamoDB on-demand + SQS`

Read `LEARN_HINGLISH.md` before deploying so you understand which part costs
money and how to avoid it.
