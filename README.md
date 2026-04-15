# sugapack

A BuildKit frontend that wraps [railpack](https://github.com/railwayapp/railpack) with remote git source support. Everything runs on the builder — zero local context transfer.

## How it works

Sugapack is a BuildKit gRPC frontend that accepts a small JSON config as input (not the full source):

1. **Fetch source** — clones the repo via `llb.Git()` with optional token auth for private repos
2. **Generate plan** — runs railpack's plan generation (embedded as a Go library) on the fetched source
3. **Execute plan** — converts the plan to LLB using railpack's `build_llb` package, substituting the git source for local context

## Usage

### With Depot

```bash
echo '{"repo":"https://github.com/user/repo.git","ref":"abc123"}' | \
  depot build -f - \
    --build-arg BUILDKIT_SYNTAX=ghcr.io/nitrictech/sugapack:latest \
    --save --platform linux/amd64 \
    /dev/null
```

### With buildctl

```bash
buildctl build \
  --frontend gateway.v0 \
  --opt source=ghcr.io/nitrictech/sugapack:latest \
  --local dockerfile=. \
  --opt filename=config.json \
  --output type=image,name=my-app:latest
```

### Private repos

Pass a git auth token as a BuildKit secret:

```bash
echo '{"repo":"https://github.com/user/private-repo.git","ref":"main","authSecret":"GIT_AUTH_TOKEN"}' | \
  depot build -f - \
    --build-arg BUILDKIT_SYNTAX=ghcr.io/nitrictech/sugapack:latest \
    --secret id=GIT_AUTH_TOKEN \
    --save --platform linux/amd64 \
    /dev/null
```

## Config format

The "Dockerfile" input is a JSON config:

```json
{
  "repo": "https://github.com/user/repo.git",
  "ref": "abc123def",
  "context": "apps/web",
  "authSecret": "GIT_AUTH_TOKEN",
  "railpack": {
    "buildCmd": "npm run build",
    "startCmd": "npm start",
    "envs": {
      "NODE_ENV": "production"
    }
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `repo` | yes | Git repository URL (HTTPS) |
| `ref` | no | Commit SHA, branch, or tag (default: `main`) |
| `context` | no | Subdirectory within the repo to use as build context |
| `authSecret` | no | BuildKit secret ID containing a git auth token |
| `railpack.buildCmd` | no | Override the build command |
| `railpack.startCmd` | no | Override the start command |
| `railpack.envs` | no | Additional environment variables for plan generation |

## Local development

Requires Docker and [buildctl](https://github.com/moby/buildkit).

```bash
# Run tests
make test

# Start local infra (buildkitd + registry) and run a full build
make dry-run

# Custom config
make dry-run TEST_CONFIG=myapp.json

# Private repo
GIT_AUTH_TOKEN=ghp_xxx make dry-run-private

# Tear down
make clean
```
