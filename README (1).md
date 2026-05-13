# Affordmed – Scalable REST API Backend

A production-grade, high-throughput REST API for an eCommerce / SaaS platform, built to demonstrate:

- **Go (1.22)** — backend API services, middleware, transaction handling
- **Next.js 14** — SSR frontend consuming the REST API
- **PostgreSQL 15** — relational data storage with ACID transactions
- **Redis 7** — caching (cache-aside pattern) and rate limiting
- **Docker + Kubernetes (AKS)** — containerised deployment with autoscaling
- **CI/CD** — GitHub Actions pipeline: lint → test → build → deploy
- **Automated testing** — unit, integration, and E2E test suites with >80% coverage

---

## Project Structure

```
affordmed-api/
├── cmd/server/           # Entrypoint (main.go)
├── internal/
│   ├── auth/             # JWT service
│   ├── cache/            # Redis product & session caching
│   ├── handlers/         # HTTP handlers (auth, products, orders, inventory, health)
│   ├── middleware/        # JWT auth, CORS, rate limiter, role guard
│   ├── models/           # Request/response types
│   └── repository/       # PostgreSQL data layer
├── pkg/database/         # DB connection + migrations
├── tests/
│   ├── unit/             # Handler unit tests (mocked deps)
│   ├── integration/      # Real DB + Redis tests
│   └── e2e/              # Full API flow tests
├── deployments/
│   ├── docker/           # Dockerfile + docker-compose.yml
│   ├── k8s/              # Kubernetes manifests (deployment, HPA, ingress)
│   └── ci/               # GitHub Actions workflow
└── frontend/             # Next.js 14 frontend
    └── src/
        ├── lib/api.ts    # Typed API client
        └── app/products/ # Products page (SSR + search/filter)
```

---

## Quick Start (Local)

### Prerequisites
- Go 1.22+
- Docker & Docker Compose
- Node.js 20+ (for frontend)

### 1. Clone and configure
```bash
git clone https://github.com/yourusername/affordmed-api.git
cd affordmed-api
cp .env.example .env
# Edit .env with your values
```

### 2. Start dependencies (PostgreSQL + Redis)
```bash
docker-compose -f deployments/docker/docker-compose.yml up postgres redis -d
```

### 3. Run the API
```bash
go run ./cmd/server
# Server starts at http://localhost:8080
```

### 4. Run the frontend
```bash
cd frontend
npm install
npm run dev
# Frontend at http://localhost:3000
```

---

## Running Tests

### Unit tests (no external deps)
```bash
go test ./tests/unit/... -v
```

### Integration + E2E (requires DB + Redis)
```bash
# Start services
docker-compose -f deployments/docker/docker-compose.yml up postgres redis -d

# Run all tests with coverage
INTEGRATION_TESTS=true \
TEST_DB_URL="postgres://postgres:postgres@localhost:5432/affordmed_test?sslmode=disable" \
TEST_REDIS_URL="redis://localhost:6379/1" \
go test ./... -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out
```

---

## API Reference

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/api/v1/auth/register` | — | Register new user |
| POST | `/api/v1/auth/login` | — | Login, receive JWT |
| POST | `/api/v1/auth/refresh` | — | Refresh access token |
| GET  | `/api/v1/products` | JWT | List products (paginated, cached) |
| GET  | `/api/v1/products/:id` | JWT | Get product by ID |
| POST | `/api/v1/products` | Admin | Create product |
| PUT  | `/api/v1/products/:id` | Admin | Update product |
| DELETE | `/api/v1/products/:id` | Admin | Soft-delete product |
| POST | `/api/v1/orders` | JWT | Create order (transactional stock deduction) |
| GET  | `/api/v1/orders` | JWT | List user's orders |
| GET  | `/api/v1/orders/:id` | JWT | Get order (ownership check) |
| PUT  | `/api/v1/inventory/:id` | Admin | Update stock + invalidate cache |
| GET  | `/api/v1/health` | — | Health check (K8s liveness/readiness) |

---

## Deploy to Azure AKS

### 1. Build and push image
```bash
docker build -f deployments/docker/Dockerfile -t youracr.azurecr.io/affordmed-api:latest .
docker push youracr.azurecr.io/affordmed-api:latest
```

### 2. Apply Kubernetes manifests
```bash
# Update secrets in deployments/k8s/deployment.yaml first
kubectl apply -f deployments/k8s/deployment.yaml
kubectl get pods -n affordmed
```

### 3. CI/CD (automatic)
Push to `main` → GitHub Actions runs tests → builds Docker image → deploys to AKS.

Required GitHub secrets: `ACR_USERNAME`, `ACR_PASSWORD`, `AZURE_CREDENTIALS`, `AKS_RESOURCE_GROUP`, `AKS_CLUSTER_NAME`

---

## Architecture Highlights

- **Cache-aside pattern**: Product lists cached in Redis with 5-min TTL; writes invalidate affected keys
- **Transactional orders**: Stock check + deduction + order creation in a single PostgreSQL transaction; rolls back on any failure
- **JWT auth with refresh tokens**: Short-lived access tokens + Redis-backed refresh tokens; blacklisting supported
- **Rate limiting**: Redis token bucket, 60 req/min per IP
- **Graceful shutdown**: Handles SIGTERM cleanly, drains in-flight requests with 30s timeout
- **HPA autoscaling**: Scales 2→10 pods on CPU >65% or memory >80%

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Go over Node.js | Lower latency, native concurrency, smaller container image |
| Cache-aside over write-through | Simplicity; avoids stale cache on complex update logic |
| Soft deletes on products | Preserves order history integrity |
| Parameterised SQL | Prevents SQL injection without ORM overhead |
| Scratch Docker image | Minimal attack surface; image < 15MB |
