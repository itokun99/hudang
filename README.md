# hudang

**hudang** *(Sundanese, "hoo-DANG": "wake up")* — a tiny scheduler that wakes your GitHub Actions workflows when GitHub's own `schedule` trigger won't.

Born from a real failure: GitHub's scheduler silently stopped firing `schedule` events for a repository while `workflow_dispatch` kept working perfectly ([community #185212](https://github.com/orgs/community/discussions/185212), [#195072](https://github.com/orgs/community/discussions/195072), [#202034](https://github.com/orgs/community/discussions/202034)). hudang runs anywhere always-on — a VPS, a home server, a container — and dispatches your workflows on time through the GitHub API.

## How it works

```
hudang (cron)  --POST /repos/{owner}/{repo}/actions/workflows/{workflow}/dispatches-->  GitHub Actions
```

GitHub Actions still does the actual work (build, render, commit); hudang only provides a reliable clock. It is deliberately dispatch-only: no git credentials, no duplicated logic.

## Quick start

1. Create a **fine-grained PAT** with access to the target repositories and permission **Actions: Read and write** (a classic token with the `workflow` scope also works).
2. Write a config (see `config.example.yaml`):

   ```yaml
   jobs:
     - name: profile-readme
       repo: itokun99/itokun99
       workflow: update-readme.yml
       ref: main
       schedule: "7,22,37,52 * * * *"
   ```

3. Run it:

   ```sh
   go build ./cmd/hudang
   HUDANG_GITHUB_TOKEN=github_pat_... ./hudang --config hudang.yaml
   ```

Preview without calling the API: `./hudang --once --dry-run --config hudang.yaml`

## Configuration reference

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `token_env` | no | `HUDANG_GITHUB_TOKEN` | Environment variable holding the GitHub token |
| `api_base` | no | `https://api.github.com` | API base URL (GitHub Enterprise Server) |
| `jobs[].name` | yes | — | Unique job name |
| `jobs[].repo` | yes | — | `owner/name` |
| `jobs[].workflow` | yes | — | Workflow file name (`ci.yml`) or numeric ID |
| `jobs[].ref` | no | repo default branch | Git ref to run the workflow on |
| `jobs[].schedule` | yes | — | Standard 5-field cron, UTC (`7,22,37,52 * * * *`) |
| `jobs[].enabled` | no | `true` | Set to `false` to pause a job |
| `jobs[].inputs` | no | — | `workflow_dispatch` inputs, as key/value pairs |

The config is strict: unknown fields are rejected so typos fail fast.

## Modes

- **Daemon** (default): schedules every enabled job internally and runs until stopped. Use `deploy/hudang.service`.
- **Timer**: `hudang --once` dispatches every enabled job immediately and exits — pair it with systemd timers (`deploy/hudang-once.timer` / `hudang-once.service`) or crontab. In this mode the `schedule` field is not used.

## Deploy on a VPS (systemd)

```sh
go build -o hudang ./cmd/hudang
sudo install -m 0755 hudang /usr/local/bin/hudang
sudo mkdir -p /etc/hudang
sudo install -m 0644 config.example.yaml /etc/hudang/config.yaml   # then edit it
sudo install -m 0600 /dev/null /etc/hudang/hudang.env              # then add: HUDANG_GITHUB_TOKEN=...
sudo install -m 0644 deploy/hudang.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hudang
journalctl -u hudang -f
```

## Docker

```sh
docker build -t hudang .
docker run -d --restart unless-stopped \
  -e HUDANG_GITHUB_TOKEN=github_pat_... \
  -v "$PWD/hudang.yaml:/etc/hudang/config.yaml:ro" \
  hudang
```

## Flags

| Flag | Description |
| --- | --- |
| `--config PATH` | Config file (default `hudang.yaml`, or `$HUDANG_CONFIG`) |
| `--once` | Dispatch all enabled jobs once and exit |
| `--job NAME` | With `--once`, run only this job |
| `--dry-run` | Log dispatches without calling the API |
| `--log-level LEVEL` | `debug`, `info` (default), `warn`, `error` |
| `--version` | Print version |

## Security notes

- The token only needs **Actions: Read and write** on the target repositories; keep it in a mode-600 env file or a secret manager, never in the repo.
- `--dry-run` makes no API calls at all.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| `HTTP 401` / `HTTP 403` | Token missing, expired, or lacking `Actions: write` |
| `HTTP 404` | Wrong repo, or the workflow does not exist on that ref |
| `HTTP 422` | `ref` does not exist, or the workflow has no `workflow_dispatch:` trigger |
| Nothing runs | Check `journalctl -u hudang`; at startup each job logs its next run time |

## Development

```sh
gofmt -l . && go vet ./... && go test ./...
```

## License

[MIT](LICENSE)
