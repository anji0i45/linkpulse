# LinkPulse — Simple Hinglish Guide

Ye file project ko samajhne ke liye hai. Code padhne se pehle ye padh lo.

---

## 1. LinkPulse ek line me kya hai?

**LinkPulse ek URL shortener + click analytics project hai.**

User long URL daalta hai, short link milta hai. Jab log short link open karte hain,
LinkPulse un clicks ka data dikhata hai — kitne clicks, kis din, kaha se aaye.

Ye koi naya app nahi hai. Ye tumhare basic URL shortener ka **advanced version** hai.

---

## 2. Abhi kya bana hua hai

Local MVP ready hai:

- Long URL se short link banana
- Short link open karne par original website par redirect
- Har click ko queue me daalna
- Clicks ko local JSON file me save karna
- Dashboard par:
  - total links
  - total clicks
  - top link
  - per-link analytics
  - clicks by day
  - top referrers
- GraphQL analytics API
- Tests
- Zero paid services; no cloud accounts

**Abhi AWS version nahi bana.** Abhi sab kuch local computer par chal raha hai.
Resume me isliye `(In Progress)` likha hai.

---

## 3. Kaise chalao

PowerShell open karo:

```powershell
cd C:\Users\Admin\linkpulse
go run .
```

Browser me kholo:

```text
http://localhost:8080
```

Band karne ke liye `Ctrl+C`.

Test chalane ke liye:

```powershell
go test ./...
```

Data file yahan banegi:

```text
C:\Users\Admin\linkpulse\data\linkpulse.json
```

Reset karna ho to server band karke file delete kar do:

```powershell
Remove-Item .\data\linkpulse.json
```

---

## 4. Normal user ka experience

1. Website kholta hai.
2. Long URL paste karta hai.
3. **Shorten** dabata hai.
4. Short link copy karta hai.
5. WhatsApp / Instagram / LinkedIn par share karta hai.
6. Log link open karte hain.
7. Dashboard me clicks ka data dekhta hai.

Bas. AWS, Lambda, Elasticsearch user ko dikhta hi nahi. Wo sab peeche hota hai.

---

## 5. Basic vs LinkPulse

| Basic URL shortener | LinkPulse |
|---|---|
| Long URL → short link | Long URL → short link |
| Short link → redirect | Short link → redirect |
| Maybe total clicks | Clicks + unique visitors + day-wise data |
| Simple database | Local JSON, later DynamoDB + MongoDB |
| No queue | Click events queue me jaate hain |
| No analytics dashboard | Full analytics dashboard |
| Local only | Later AWS serverless |

**Yaad rakho:** URL shortener product ka excuse hai. Asli skill backend engineering hai.

---

## 6. Request flow

### Link create

```text
Browser
  -> POST /api/links
  -> Go HTTP handler
  -> URL validate
  -> 7-character short code generate
  -> JSON store me save
  -> cache me daalo
  -> short URL return
```

### Redirect

```text
User short link open karta hai
  -> GET /abc123
  -> cache check
  -> cache miss? store se link lo
  -> click event queue me daalo
  -> 302 redirect original URL par
```

Click save hone ka wait redirect nahi karta. Isliye redirect fast rehta hai.

### Analytics

```text
Queue worker
  -> click event store me save karta hai
  -> dashboard GET /api/analytics/abc123
  -> total clicks, unique visitors, days, referrers
```

---

## 7. Folder structure

```text
linkpulse/
├── main.go                  # sab kuch connect karta hai
├── internal/
│   ├── api/
│   │   ├── api.go           # Gin HTTP API + redirect + analytics endpoints
│   │   ├── graphql.go       # GraphQL schema + resolvers
│   │   └── web/index.html   # dashboard
│   ├── cache/cache.go       # in-memory TTL cache
│   ├── queue/queue.go       # in-process click queue
│   └── store/
│       ├── store.go         # data types + Store interface
│       └── file.go          # JSON file database
├── data/                    # local data file
├── README.md
└── LEARN_HINGLISH.md
```

### Important: `Store` interface

`internal/store/store.go` me ek interface hai:

```go
type Store interface {
    CreateLink(...)
    GetLink(...)
    RecordClick(...)
    ListLinks(...)
    GetAnalytics(...)
}
```

Abhi `FileStore` isko implement karta hai. Baad me `DynamoDBStore` bana sakte ho.
HTTP handlers ko badalna nahi padega. Isko **abstraction** kehte hain.

---

## 8. Concepts — simple language

### URL shortener

Long URL ka chhota alias.

```text
https://example.com/very/long/path -> http://localhost:8080/Ab3xK9p
```

### API

Do software ke beech baat karne ka tarika. Browser `POST /api/links` bhejta hai,
Go JSON reply karta hai.

### REST API

HTTP methods ka use:

- `POST` = create
- `GET` = read
- `DELETE` = remove
- `PUT/PATCH` = update

### Cache

Fast memory me data rakhna taaki har baar database na kholna pade.

LinkPulse me popular short links memory cache me rehte hain. Cache miss hone par
JSON store se link aata hai.

### Queue

Kaam ki line. Redirect handler click event queue me daal deta hai, worker baad me
save karta hai. Isse user ko wait nahi karna padta.

### Event-driven

System events par react karta hai. Yahan event hai: **“kisi ne link click kiya.”**

### Async

Kaam turant nahi hota, background me hota hai. Analytics save hona async hai;
redirect turant hota hai.

### Persistence

Data save rehna. Yahan JSON file me save hota hai. Server restart hone par bhi
links aur clicks bache rehte hain.

### Serverless

Server ka management nahi karna padta. AWS request aane par code chalata hai.
Traffic badhe to automatically scale karta hai.

Serverless ka matlab “server nahi hai” nahi hai. Matlab “tum server manage nahi
karte” hai.

### AWS Lambda

AWS ka serverless compute. Tum Go function deploy karte ho. Request aayi, function
chala, response gaya, band.

### API Gateway

Public HTTP entry point. Browser `POST /links` bhejta hai, API Gateway Lambda ko
call karta hai.

### DynamoDB

AWS ka serverless NoSQL database. Link records ke liye good:

```text
PK = LINK#abc123
long_url = https://...
created_at = ...
```

### SQS

AWS ki message queue. Click events SQS me jaate hain. Consumer Lambda unhe process
karta hai. Failure par retry aur dead-letter queue milti hai.

### MongoDB

Document database. Raw click events ke liye flexible hai:

```json
{
  "code": "abc123",
  "clicked_at": "2026-11-23T10:00:00Z",
  "referrer": "google.com",
  "user_agent": "..."
}
```

### Elasticsearch / OpenSearch

Search aur analytics engine. Clicks ko index karta hai taaki fast search, filter
aur aggregation ho sake.

### GraphQL

Dashboard ke liye query API. Client exact data maangta hai. Ye local MVP me
`/graphql` par implemented hai:

```graphql
{
  analytics(code: "abc123") {
    totalClicks
    clicksByDay { day clicks }
  }
}
```

### gRPC

Internal service-to-service API. Fast binary protocol, protobuf use karta hai.
Public browser API ke liye REST/GraphQL zyada common hai.

### Terraform

Infrastructure as Code. AWS console me click-click karne ke bajaye `.tf` files se
Lambda, DynamoDB, SQS, IAM sab create karte hain.

### Docker

App ko container me pack karta hai taaki har machine par same chale. Local version
me zarurat nahi, AWS version me useful hai.

---

## 9. Local version vs AWS version

| Feature | Abhi local | AWS version |
|---|---|---|
| HTTP server | Go + Gin | API Gateway + Lambda |
| Link storage | JSON file | DynamoDB |
| Cache | In-memory map | Redis / Upstash |
| Queue | Go channel | SQS |
| Click storage | JSON file | MongoDB Atlas free tier |
| Search/analytics | In-memory aggregation | OpenSearch/Elasticsearch |
| Dashboard API | REST + GraphQL | REST + GraphQL |
| Internal API | Same Go functions | gRPC |
| Infra | Manual | Terraform |
| Cost | ₹0 | Free tier only, with billing alerts |

---

## 10. Free setup plan — important

Sab kuch free me possible hai, lekin **“free forever” guarantee nahi hoti**.
AWS free tier time-limited hota hai aur limits cross karne par charge lag sakta hai.

### Phase 1 — abhi done, ₹0

- Go installed
- Local JSON database
- In-memory cache
- In-process queue
- Local dashboard
- No account needed

### Phase 2 — AWS free tier (jab ready ho)

1. AWS account banao.
2. Billing alarm lagao — $1 ya $5.
3. Sirf ye services use karo:
   - AWS Lambda
   - API Gateway HTTP API
   - DynamoDB on-demand
   - SQS
   - CloudWatch basic logs
4. **In cheezon se bacho:**
   - NAT Gateway (mehnga)
   - Always-on OpenSearch
   - Always-on ElastiCache
   - Provisioned Lambda concurrency
5. MongoDB Atlas free M0 cluster use karo.
6. Redis ke liye Upstash free tier ya local Redis use karo.
7. Elasticsearch/OpenSearch local Docker me chalao, ya free trial check karo.
8. Terraform se infra banao.
9. GitHub Actions free minutes me CI chalao.

**Rule:** deploy karne ke baad AWS console me cost explorer aur billing alarm check
karo. Free tier limit cross mat karo.

---

## 11. 7-day build plan

### Day 1 — done

Local Go API + JSON store + dashboard.

### Day 2 — DynamoDB

`DynamoDBStore` banao jo `Store` interface implement kare. LocalStack/DynamoDB
Local se test karo.

### Day 3 — SQS

`SQSPublisher` banao. Click event SQS me bhejo. Consumer Lambda se process karo.

### Day 4 — Lambda + API Gateway

Go HTTP server ko Lambda handler me wrap karo. API Gateway routes banao.

### Day 5 — MongoDB + Elasticsearch

Click events MongoDB me save karo. Elasticsearch me index karo. Analytics query
Elasticsearch se lao.

### Day 6 — gRPC

GraphQL dashboard API local MVP me already done hai. Ab internal gRPC
`LinkService` banao.

### Day 7 — Terraform + load test

Terraform se deploy karo. k6 se load test karo. Real p95 latency measure karo.
README me architecture diagram aur numbers add karo.

---

## 12. Interview me kaise batao

Agar interviewer poochhe:

> “Apne project ke baare me batao.”

Tum safe answer ye de sakte ho:

> “Main pehle ek basic Go URL shortener banaya tha. Ab usko LinkPulse me extend
> kar raha hoon. Local version me Go + Gin REST API, GraphQL analytics API,
> JSON persistence, in-memory cache aur async click queue hai. Dashboard clicks
> by day aur referrers dikhata hai. Next phase me same interfaces ko DynamoDB,
> SQS, Lambda, MongoDB Atlas aur Elasticsearch se replace kar raha hoon.”

Agar interviewer poochhe:

> “DynamoDB kyun?”

Tum bolo:

> “Link lookup simple key-value access hai. Short code partition key ke liye fit
> hai. DynamoDB serverless hai, on-demand scaling hai, aur Lambda ke saath
> low-latency access deta hai.”

Agar poochhe:

> “SQS kyun?”

Tum bolo:

> “Redirect path fast rakhna tha. Click analytics ko synchronous rakhna galat
> hota, kyunki analytics down ho to redirect fail ho jata. Isliye click event
> queue me daalte hain aur consumer asynchronously process karta hai.”

Agar poochhe:

> “Cache kaise kaam karta hai?”

Tum bolo:

> “Read-through/cache-aside pattern. Pehle cache check, miss par database, phir
> cache me TTL ke saath write. Link immutable hai isliye local MVP me
> invalidation ki zarurat nahi, but production me update/delete par cache delete
> karna hoga.”

---

## 13. Resume ke liye sach

Abhi resume me project `(In Progress)` hai. Ye sahi hai.

- Local MVP ready hai — tum ise chala sakte ho.
- AWS part ready nahi hai — usko “done” mat bolo.
- Interview me sirf wahi bolo jo tumne actually kiya.
- Jab AWS deploy ho jaye, tab `(In Progress)` hatao aur real numbers add karo.

**Fake project se better hai simple project jo tum explain kar sako.**

---

## 14. Next commands

```powershell
cd C:\Users\Admin\linkpulse
go test ./...
go run .
```

Browser:

```text
http://localhost:8080
```

Project khatam karne ke baad:

```text
LinkPulse — completed
├── local MVP
├── AWS Lambda + API Gateway
├── DynamoDB + SQS
├── MongoDB + Elasticsearch
├── GraphQL + gRPC
├── Terraform
└── load test numbers
```

Tab resume line `(In Progress)` se `completed` ho sakti hai.
