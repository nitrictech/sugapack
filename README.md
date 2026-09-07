# sugapack

A BuildKit frontend that wraps [railpack](https://github.com/railwayapp/railpack) with remote git source support. Everything runs on the builder — zero local context transfer.

## How it works

Sugapack is a BuildKit gRPC frontend that accepts a small JSON config as input (not the full source):

1. **Fetch source** — clones the repo via `llb.Git()` with optional token auth for private repos
2. **Generate plan** — runs railpack's plan generation (embedded as a Go library) on the fetched source
3. **Execute plan** — converts the plan to LLB using railpack's `build_llb` package, substituting the git source for local context

## The vendored railpack version

Framework support lives in railpack, so the version in `go.mod` is what decides
whether an app builds. It is reported in the plan step's logs and baked into
every image as `RAILPACK_VERSION`.

Dependabot opens a weekly PR for railpack on its own (see
[`.github/dependabot.yml`](.github/dependabot.yml)) so detection fixes are not
buried in a grouped dependency bump. Framework detection itself is railpack's to
test; what we check here is our own handling of the plan it produces.

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
  "version": 1,
  "repo": "https://github.com/user/repo.git",
  "ref": "abc123def",
  "context": "apps/web",
  "authSecret": "GIT_AUTH_TOKEN",
  "railpack": {
    "buildCmd": "npm run build",
    "startCmd": "npm start",
    "buildVariables": { "VITE_API_URL": "https://api.example.com" },
    "variables": { "GIT_SHA": "abc123def" },
    "secrets": ["DATABASE_URL"]
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `version` | no | Schema version. Omitted means `0`, the legacy schema. |
| `repo` | yes | Git repository URL (HTTPS) |
| `ref` | no | Commit SHA, branch, or tag (default: `main`) |
| `context` | no | Subdirectory within the repo to use as build context |
| `authSecret` | no | BuildKit secret ID containing a git auth token |
| `railpack.buildCmd` | no | Override the build command |
| `railpack.startCmd` | no | Override the start command |

### Variables and secrets (`version: 1`)

| Field | Value comes from | Reaches | In the image | In step cache keys |
|-------|------------------|---------|--------------|--------------------|
| `railpack.buildVariables` | this config | plan generation and every build step | no | yes |
| `railpack.variables` | this config | the final image's environment | yes | no |
| `railpack.secrets` | `--secret id=NAME` | every build step | no | no |

`buildVariables` also carries railpack's own knobs — `RAILPACK_PACKAGES` and
friends — since it is what feeds plan generation.

`secrets` lists names only; values never appear in this config or in the image.
Each name needs a matching `--secret id=NAME,env=NAME` or the build fails with
`secret NAME: not found`. BuildKit keeps secrets out of its cache keys, so pass
`--build-arg secrets-hash=<hash of the values>` too, or a changed secret will
reuse the layer that baked the old one.

A name may appear in both `buildVariables` and `variables` when a value is
needed at build time and at runtime. A name cannot be both a secret and either
kind of variable.

### The legacy schema (`version: 0`)

Version 0 has a single `railpack.envs` map, which conflates the three. Its
values steer plan generation, while its *names* become plan secrets — so the
caller supplies the value a second time through `--secret id=NAME`, and that is
the one the build steps see. It reaches neither the image nor `deploy`.

It is kept so the frontend image can roll forward ahead of its callers. Mixing
fields across versions is rejected rather than guessed at.

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
