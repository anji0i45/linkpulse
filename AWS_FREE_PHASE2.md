# LinkPulse Phase 2 — Optional Free AWS Plan

> **STOP: No account has been created. No credentials are stored here.**
> This file is only an optional future plan. Do not create an AWS account or add
> any payment/account details unless you personally choose to. The current
> project is 100% local and free.
>
> Current status: **Phase 1 local MVP is complete and running.**
> Phase 2 is not deployed yet. Do not remove `(In Progress)` from the resume
> until the AWS version is actually working.

This document is the free-tier plan for turning the local MVP into the
serverless version described on the resume.

---

## Target architecture

```text
Browser
  |
  v
API Gateway (HTTP API)
  |
  v
AWS Lambda (Go)
  |------------------> DynamoDB (short links)
  |------------------> Redis / Upstash (cache)
  |
  v
SQS queue
  |
  v
Consumer Lambda
  |------------------> MongoDB Atlas free tier (raw clicks)
  |------------------> OpenSearch / Elasticsearch (search + analytics)
  |
  v
GraphQL API + gRPC internal API
```

Terraform creates the AWS pieces. Docker is used only for local development and
optional packaging.

---

## What is already done locally

| Local piece | File / status |
|---|---|
| REST API | `internal/api/api.go` |
| Dashboard | `internal/api/web/index.html` |
| Link storage | `internal/store/file.go` |
| Cache abstraction | `internal/cache/cache.go` |
| Queue abstraction | `internal/queue/queue.go` |
| Tests | `internal/**/*_test.go` |
| Run locally | `go run .` |

The `Store` and `Publisher` interfaces are the important part. The AWS version
will implement the same interfaces.

---

## Free-tier services to use

| Need | Free option | Notes |
|---|---|---|
| Compute | AWS Lambda | Free-tier requests; avoid provisioned concurrency |
| HTTP API | API Gateway HTTP API | Cheaper than REST API for simple routes |
| Link DB | DynamoDB on-demand | Free-tier storage/reads/writes; set billing alert |
| Queue | SQS | Free-tier requests |
| Click DB | MongoDB Atlas M0 | Free shared cluster |
| Cache | Upstash Redis free tier or local Redis | Check current limits |
| Search | Local OpenSearch/Elasticsearch | Do not leave a paid AWS OpenSearch domain running |
| Infra | Terraform | Open source |
| CI | GitHub Actions | Free minutes for public repos |

**Avoid:** NAT Gateway, always-on ElastiCache, always-on OpenSearch domain,
provisioned Lambda concurrency.

---

## Step-by-step

### 1. Protect the account

- Create an AWS account.
- Enable MFA.
- Set a billing alarm at **$1–$5**.
- Use only one region.
- Never commit AWS keys to Git.

### 2. Create the free data layer

- DynamoDB table `linkpulse-links`, partition key `code` (string).
- SQS standard queue `linkpulse-clicks`.
- Optional dead-letter queue `linkpulse-clicks-dlq`.
- MongoDB Atlas M0 cluster and database `linkpulse`.
- Upstash Redis database or local Redis for the first test.

### 3. Add the Go adapters

Create these packages without changing the HTTP handlers:

```text
internal/store/dynamodb.go       # implements store.Store
internal/queue/sqs.go            # implements queue.Publisher
internal/analytics/mongo.go      # saves raw click events
internal/search/opensearch.go    # indexes click events
cmd/lambda/main.go               # AWS Lambda entry point
```

DynamoDB item shape:

```text
PK       = LINK#<code>
code     = <code>
long_url = https://...
created_at = ...
```

SQS click event:

```json
{
  "code": "abc123",
  "clicked_at": "2026-11-23T10:00:00Z",
  "referrer": "google.com",
  "user_agent": "...",
  "ip_hash": "..."
}
```

### 4. Wrap the HTTP API for Lambda

- Add `github.com/aws/aws-lambda-go/lambda`.
- Use `github.com/aws/aws-lambda-go/lambdahandler` or an adapter for `net/http`.
- Keep the same routes and handlers.
- API Gateway HTTP API routes:
  - `POST /api/links`
  - `GET /api/links`
  - `GET /api/analytics/{code}`
  - `GET /{code}`

### 5. Add Terraform

Terraform resources:

```text
aws_lambda_function
aws_apigatewayv2_api
aws_apigatewayv2_route
aws_dynamodb_table
aws_sqs_queue
aws_iam_role
aws_iam_policy
aws_cloudwatch_log_group
```

Use GitHub Actions OIDC instead of long-lived AWS keys if possible.

### 6. Load test

Use k6 locally or against the deployed endpoint:

```text
GET /{code}        -> redirect + queue publish
POST /api/links    -> DynamoDB write
GET /api/analytics -> read analytics
```

Record real p95 latency and requests/second. Only then add numbers to the
resume.

---

## Cost guardrails

- Billing alarm: **$1** and **$5**.
- Check AWS Cost Explorer weekly.
- Delete unused resources after the demo.
- Do not enable NAT Gateway.
- Do not create a paid OpenSearch domain just for the resume.
- Keep the dashboard and MongoDB free tier small.
- Test with a small number of requests first.

---

## What to say in an interview

> “I built the local Go MVP first: REST API, JSON persistence, cache, async
> queue, and dashboard. I’m now replacing the local edges with AWS managed
> services using the same interfaces: DynamoDB, SQS, Lambda and API Gateway.
> For analytics I’m using MongoDB Atlas and OpenSearch; Terraform manages the
> infrastructure.”

Do not say “deployed on AWS” until it is actually deployed.
