# drydock

> Inspect `helm_release` value changes in a Terraform plan before you set sail.

`drydock` is a small CLI that pulls the `helm_release` resources out of a Terraform plan and shows you a clean, human-readable diff of the **effective Helm values** — the merged result of every `values` YAML doc, `set`, `set_list`, and `set_sensitive` block, in the same order Helm would apply them.

If you've ever stared at a `terraform plan` and tried to figure out *which key in which values file is actually changing*, this is for you.

## Why

The `terraform-provider-helm` plan output is hard to read:

- `values` is a list of stringified YAML blobs.
- `set` / `set_sensitive` / `set_list` are flat dotted paths layered on top.
- Terraform shows the raw provider state, not the merged Helm view.

`drydock` does the merge itself and prints a per-release diff that mirrors what Helm will actually see.

## Install

### Homebrew (macOS / Linux)

```sh
brew install sethryder/tap/drydock
```

To upgrade later:

```sh
brew update && brew upgrade drydock
```

### `go install`

```sh
go install github.com/sethryder/drydock@latest
```

### Pre-built binaries

Download the appropriate archive for your OS/arch from the [latest release](https://github.com/sethryder/drydock/releases/latest), extract it, and put `drydock` on your `$PATH`.

### From source

```sh
git clone https://github.com/sethryder/drydock
cd drydock
go build -o drydock .
```

Building from source requires Go 1.22+. The `drydock plan` subcommand and `.tfplan` input shell out to `terraform`, so that needs to be on your `$PATH` for those workflows regardless of how you installed.

## Usage

```
drydock <plan.tfplan>           # binary plan, shells out to `terraform show -json`
drydock <plan.json>             # already-rendered JSON plan
drydock -                       # read JSON plan from stdin
drydock plan [-- tf-args...]    # run `terraform plan` and diff in one step
```

Flags:

| Flag | Description |
| --- | --- |
| `--release <addr>` | Only show the diff for one resource (e.g. `helm_release.airflow`). |
| `--no-color` | Disable ANSI color output. |
| `--chdir <dir>` | (`plan` subcommand only) Run terraform from this directory. |

### Examples

Diff a binary plan file:

```sh
terraform plan -out=tf.plan
drydock tf.plan
```

Pipe in JSON:

```sh
terraform show -json tf.plan | drydock -
```

Run `terraform plan` and diff in one step, passing extra args through:

```sh
drydock plan -- -var-file=prod.tfvars -target=helm_release.airflow
```

Filter to a single release:

```sh
drydock --release helm_release.airflow tf.plan
```

### Sample output

```
helm_release.airflow  [update]  chart=airflow
─────────────────────────────────────────────
  ~ image.tag: "2.8.1" → "2.9.0"
  + workers.replicas: 5
  - dags.gitSync.branch: "main"
  ~ workers.resources.requests.memory: "2Gi" → "4Gi"
```

Adds are green `+`, removes are red `-`, modifications are yellow `~`. Nested keys use dotted paths; list elements use `[i]` indexing when lengths match.

## How it works

For each `helm_release` resource change in the plan, `drydock`:

1. Decodes every string in `values` as YAML.
2. Deep-merges them in list order (later wins, matching Helm).
3. Applies `set` → `set_list` → `set_sensitive` blocks on top, coercing stringified scalars (`"true"`, `"42"`) back to their native types so you don't see spurious noise.
4. Diffs the resulting `before` and `after` maps key by key.

Resources whose only action is `no-op` are skipped.

## Limitations

- Helm's `set` syntax for array indices (`a[0].b`) is mostly normalized away by the provider; `drydock` treats `set` paths as plain dotted paths and does not parse `[i]` segments.
- When two list values differ in length, the whole list is shown as replaced rather than diffed element-wise.
- Only `helm_release` (and `helm_release_*` variants) are inspected — everything else in the plan is ignored.

## Contributing

Issues and PRs welcome. Run the existing tests with `go test ./...` and please include a testdata fixture for any new plan-shape edge case you're handling.

## License

MIT — see [LICENSE](LICENSE).
